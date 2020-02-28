// Copyright 2018 Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package android

import (
	"fmt"
	"sort"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"

	"android/soong/shared"
)

// RuleBuilder provides an alternative to ModuleContext.Rule and ModuleContext.Build to add a command line to the build
// graph.
type RuleBuilder struct {
	commands       []*RuleBuilderCommand
	installs       RuleBuilderInstalls
	temporariesSet map[WritablePath]bool
	restat         bool
	sbox           bool
	highmem        bool
	remoteable     RemoteRuleSupports
	sboxOutDir     WritablePath
	missingDeps    []string
}

// NewRuleBuilder returns a newly created RuleBuilder.
func NewRuleBuilder() *RuleBuilder {
	return &RuleBuilder{
		temporariesSet: make(map[WritablePath]bool),
	}
}

// RuleBuilderInstall is a tuple of install from and to locations.
type RuleBuilderInstall struct {
	From Path
	To   string
}

type RuleBuilderInstalls []RuleBuilderInstall

// String returns the RuleBuilderInstalls in the form used by $(call copy-many-files) in Make, a space separated
// list of from:to tuples.
func (installs RuleBuilderInstalls) String() string {
	sb := strings.Builder{}
	for i, install := range installs {
		if i != 0 {
			sb.WriteRune(' ')
		}
		sb.WriteString(install.From.String())
		sb.WriteRune(':')
		sb.WriteString(install.To)
	}
	return sb.String()
}

// MissingDeps adds modules to the list of missing dependencies.  If MissingDeps
// is called with a non-empty input, any call to Build will result in a rule
// that will print an error listing the missing dependencies and fail.
// MissingDeps should only be called if Config.AllowMissingDependencies() is
// true.
func (r *RuleBuilder) MissingDeps(missingDeps []string) {
	r.missingDeps = append(r.missingDeps, missingDeps...)
}

// Restat marks the rule as a restat rule, which will be passed to ModuleContext.Rule in BuildParams.Restat.
//
// Restat is not compatible with Sbox()
func (r *RuleBuilder) Restat() *RuleBuilder {
	if r.sbox {
		panic("Restat() is not compatible with Sbox()")
	}
	r.restat = true
	return r
}

// HighMem marks the rule as a high memory rule, which will limit how many run in parallel with other high memory
// rules.
func (r *RuleBuilder) HighMem() *RuleBuilder {
	r.highmem = true
	return r
}

// Remoteable marks the rule as supporting remote execution.
func (r *RuleBuilder) Remoteable(supports RemoteRuleSupports) *RuleBuilder {
	r.remoteable = supports
	return r
}

// Sbox marks the rule as needing to be wrapped by sbox. The WritablePath should point to the output
// directory that sbox will wipe. It should not be written to by any other rule. sbox will ensure
// that all outputs have been written, and will discard any output files that were not specified.
//
// Sbox is not compatible with Restat()
func (r *RuleBuilder) Sbox(outputDir WritablePath) *RuleBuilder {
	if r.sbox {
		panic("Sbox() may not be called more than once")
	}
	if len(r.commands) > 0 {
		panic("Sbox() may not be called after Command()")
	}
	if r.restat {
		panic("Sbox() is not compatible with Restat()")
	}
	r.sbox = true
	r.sboxOutDir = outputDir
	return r
}

// Install associates an output of the rule with an install location, which can be retrieved later using
// RuleBuilder.Installs.
func (r *RuleBuilder) Install(from Path, to string) {
	r.installs = append(r.installs, RuleBuilderInstall{from, to})
}

// Command returns a new RuleBuilderCommand for the rule.  The commands will be ordered in the rule by when they were
// created by this method.  That can be mutated through their methods in any order, as long as the mutations do not
// race with any call to Build.
func (r *RuleBuilder) Command() *RuleBuilderCommand {
	command := &RuleBuilderCommand{
		sbox:       r.sbox,
		sboxOutDir: r.sboxOutDir,
	}
	r.commands = append(r.commands, command)
	return command
}

// Temporary marks an output of a command as an intermediate file that will be used as an input to another command
// in the same rule, and should not be listed in Outputs.
func (r *RuleBuilder) Temporary(path WritablePath) {
	r.temporariesSet[path] = true
}

// DeleteTemporaryFiles adds a command to the rule that deletes any outputs that have been marked using Temporary
// when the rule runs.  DeleteTemporaryFiles should be called after all calls to Temporary.
func (r *RuleBuilder) DeleteTemporaryFiles() {
	var temporariesList WritablePaths

	for intermediate := range r.temporariesSet {
		temporariesList = append(temporariesList, intermediate)
	}

	sort.Slice(temporariesList, func(i, j int) bool {
		return temporariesList[i].String() < temporariesList[j].String()
	})

	r.Command().Text("rm").Flag("-f").Outputs(temporariesList)
}

// Inputs returns the list of paths that were passed to the RuleBuilderCommand methods that take
// input paths, such as RuleBuilderCommand.Input, RuleBuilderComand.Implicit, or
// RuleBuilderCommand.FlagWithInput.  Inputs to a command that are also outputs of another command
// in the same RuleBuilder are filtered out.  The list is sorted and duplicates removed.
func (r *RuleBuilder) Inputs() Paths {
	outputs := r.outputSet()
	depFiles := r.depFileSet()

	inputs := make(map[string]Path)
	for _, c := range r.commands {
		for _, input := range c.inputs {
			inputStr := input.String()
			if _, isOutput := outputs[inputStr]; !isOutput {
				if _, isDepFile := depFiles[inputStr]; !isDepFile {
					inputs[input.String()] = input
				}
			}
		}
	}

	var inputList Paths
	for _, input := range inputs {
		inputList = append(inputList, input)
	}

	sort.Slice(inputList, func(i, j int) bool {
		return inputList[i].String() < inputList[j].String()
	})

	return inputList
}

// OrderOnlys returns the list of paths that were passed to the RuleBuilderCommand.OrderOnly or
// RuleBuilderCommand.OrderOnlys.  The list is sorted and duplicates removed.
func (r *RuleBuilder) OrderOnlys() Paths {
	orderOnlys := make(map[string]Path)
	for _, c := range r.commands {
		for _, orderOnly := range c.orderOnlys {
			orderOnlys[orderOnly.String()] = orderOnly
		}
	}

	var orderOnlyList Paths
	for _, orderOnly := range orderOnlys {
		orderOnlyList = append(orderOnlyList, orderOnly)
	}

	sort.Slice(orderOnlyList, func(i, j int) bool {
		return orderOnlyList[i].String() < orderOnlyList[j].String()
	})

	return orderOnlyList
}

func (r *RuleBuilder) outputSet() map[string]WritablePath {
	outputs := make(map[string]WritablePath)
	for _, c := range r.commands {
		for _, output := range c.outputs {
			outputs[output.String()] = output
		}
	}
	return outputs
}

// Outputs returns the list of paths that were passed to the RuleBuilderCommand methods that take
// output paths, such as RuleBuilderCommand.Output, RuleBuilderCommand.ImplicitOutput, or
// RuleBuilderCommand.FlagWithInput.  The list is sorted and duplicates removed.
func (r *RuleBuilder) Outputs() WritablePaths {
	outputs := r.outputSet()

	var outputList WritablePaths
	for _, output := range outputs {
		if !r.temporariesSet[output] {
			outputList = append(outputList, output)
		}
	}

	sort.Slice(outputList, func(i, j int) bool {
		return outputList[i].String() < outputList[j].String()
	})

	return outputList
}

func (r *RuleBuilder) depFileSet() map[string]WritablePath {
	depFiles := make(map[string]WritablePath)
	for _, c := range r.commands {
		for _, depFile := range c.depFiles {
			depFiles[depFile.String()] = depFile
		}
	}
	return depFiles
}

// DepFiles returns the list of paths that were passed to the RuleBuilderCommand methods that take depfile paths, such
// as RuleBuilderCommand.DepFile or RuleBuilderCommand.FlagWithDepFile.
func (r *RuleBuilder) DepFiles() WritablePaths {
	var depFiles WritablePaths

	for _, c := range r.commands {
		for _, depFile := range c.depFiles {
			depFiles = append(depFiles, depFile)
		}
	}

	return depFiles
}

// Installs returns the list of tuples passed to Install.
func (r *RuleBuilder) Installs() RuleBuilderInstalls {
	return append(RuleBuilderInstalls(nil), r.installs...)
}

func (r *RuleBuilder) toolsSet() map[string]Path {
	tools := make(map[string]Path)
	for _, c := range r.commands {
		for _, tool := range c.tools {
			tools[tool.String()] = tool
		}
	}

	return tools
}

// Tools returns the list of paths that were passed to the RuleBuilderCommand.Tool method.  The
// list is sorted and duplicates removed.
func (r *RuleBuilder) Tools() Paths {
	toolsSet := r.toolsSet()

	var toolsList Paths
	for _, tool := range toolsSet {
		toolsList = append(toolsList, tool)
	}

	sort.Slice(toolsList, func(i, j int) bool {
		return toolsList[i].String() < toolsList[j].String()
	})

	return toolsList
}

// RspFileInputs returns the list of paths that were passed to the RuleBuilderCommand.FlagWithRspFileInputList method.
func (r *RuleBuilder) RspFileInputs() Paths {
	var rspFileInputs Paths
	for _, c := range r.commands {
		if c.rspFileInputs != nil {
			if rspFileInputs != nil {
				panic("Multiple commands in a rule may not have rsp file inputs")
			}
			rspFileInputs = c.rspFileInputs
		}
	}

	return rspFileInputs
}

// Commands returns a slice containing the built command line for each call to RuleBuilder.Command.
func (r *RuleBuilder) Commands() []string {
	var commands []string
	for _, c := range r.commands {
		commands = append(commands, c.String())
	}
	return commands
}

// NinjaEscapedCommands returns a slice containin the built command line after ninja escaping for each call to
// RuleBuilder.Command.
func (r *RuleBuilder) NinjaEscapedCommands() []string {
	var commands []string
	for _, c := range r.commands {
		commands = append(commands, c.NinjaEscapedString())
	}
	return commands
}

// BuilderContext is a subset of ModuleContext and SingletonContext.
type BuilderContext interface {
	PathContext
	Rule(PackageContext, string, blueprint.RuleParams, ...string) blueprint.Rule
	Build(PackageContext, BuildParams)
}

var _ BuilderContext = ModuleContext(nil)
var _ BuilderContext = SingletonContext(nil)

func (r *RuleBuilder) depFileMergerCmd(ctx PathContext, depFiles WritablePaths) *RuleBuilderCommand {
	return r.Command().
		BuiltTool(ctx, "dep_fixer").
		Inputs(depFiles.Paths())
}

// Build adds the built command line to the build graph, with dependencies on Inputs and Tools, and output files for
// Outputs.
func (r *RuleBuilder) Build(pctx PackageContext, ctx BuilderContext, name string, desc string) {
	name = ninjaNameEscape(name)

	if len(r.missingDeps) > 0 {
		ctx.Build(pctx, BuildParams{
			Rule:        ErrorRule,
			Outputs:     r.Outputs(),
			OrderOnly:   r.OrderOnlys(),
			Description: desc,
			Args: map[string]string{
				"error": "missing dependencies: " + strings.Join(r.missingDeps, ", "),
			},
		})
		return
	}

	var depFile WritablePath
	var depFormat blueprint.Deps
	if depFiles := r.DepFiles(); len(depFiles) > 0 {
		depFile = depFiles[0]
		depFormat = blueprint.DepsGCC
		if len(depFiles) > 1 {
			// Add a command locally that merges all depfiles together into the first depfile.
			r.depFileMergerCmd(ctx, depFiles)

			if r.sbox {
				// Check for Rel() errors, as all depfiles should be in the output dir
				for _, path := range depFiles[1:] {
					Rel(ctx, r.sboxOutDir.String(), path.String())
				}
			}
		}
	}

	tools := r.Tools()
	commands := r.NinjaEscapedCommands()
	outputs := r.Outputs()

	if len(commands) == 0 {
		return
	}
	if len(outputs) == 0 {
		panic("No outputs specified from any Commands")
	}

	commandString := strings.Join(commands, " && ")

	if r.sbox {
		sboxOutputs := make([]string, len(outputs))
		for i, output := range outputs {
			sboxOutputs[i] = "__SBOX_OUT_DIR__/" + Rel(ctx, r.sboxOutDir.String(), output.String())
		}

		commandString = proptools.ShellEscape(commandString)
		if !strings.HasPrefix(commandString, `'`) {
			commandString = `'` + commandString + `'`
		}

		sboxCmd := &RuleBuilderCommand{}
		sboxCmd.BuiltTool(ctx, "sbox").
			Flag("-c").Text(commandString).
			Flag("--sandbox-path").Text(shared.TempDirForOutDir(PathForOutput(ctx).String())).
			Flag("--output-root").Text(r.sboxOutDir.String())

		if depFile != nil {
			sboxCmd.Flag("--depfile-out").Text(depFile.String())
		}

		sboxCmd.Flags(sboxOutputs)

		commandString = sboxCmd.buf.String()
		tools = append(tools, sboxCmd.tools...)
	}

	// Ninja doesn't like multiple outputs when depfiles are enabled, move all but the first output to
	// ImplicitOutputs.  RuleBuilder only uses "$out" for the rsp file location, so the distinction between Outputs and
	// ImplicitOutputs doesn't matter.
	output := outputs[0]
	implicitOutputs := outputs[1:]

	var rspFile, rspFileContent string
	rspFileInputs := r.RspFileInputs()
	if rspFileInputs != nil {
		rspFile = "$out.rsp"
		rspFileContent = "$in"
	}

	var pool blueprint.Pool
	if ctx.Config().UseGoma() && r.remoteable.Goma {
		// When USE_GOMA=true is set and the rule is supported by goma, allow jobs to run outside the local pool.
	} else if ctx.Config().UseRBE() && r.remoteable.RBE {
		// When USE_RBE=true is set and the rule is supported by RBE, allow jobs to run outside the local pool.
	} else if r.highmem {
		pool = highmemPool
	} else if ctx.Config().UseRemoteBuild() {
		pool = localPool
	}

	ctx.Build(pctx, BuildParams{
		Rule: ctx.Rule(pctx, name, blueprint.RuleParams{
			Command:        commandString,
			CommandDeps:    tools.Strings(),
			Restat:         r.restat,
			Rspfile:        rspFile,
			RspfileContent: rspFileContent,
			Pool:           pool,
		}),
		Inputs:          rspFileInputs,
		Implicits:       r.Inputs(),
		Output:          output,
		ImplicitOutputs: implicitOutputs,
		Depfile:         depFile,
		Deps:            depFormat,
		Description:     desc,
	})
}

// RuleBuilderCommand is a builder for a command in a command line.  It can be mutated by its methods to add to the
// command and track dependencies.  The methods mutate the RuleBuilderCommand in place, as well as return the
// RuleBuilderCommand, so they can be used chained or unchained.  All methods that add text implicitly add a single
// space as a separator from the previous method.
type RuleBuilderCommand struct {
	buf           strings.Builder
	inputs        Paths
	orderOnlys    Paths
	outputs       WritablePaths
	depFiles      WritablePaths
	tools         Paths
	rspFileInputs Paths

	// spans [start,end) of the command that should not be ninja escaped
	unescapedSpans [][2]int

	sbox       bool
	sboxOutDir WritablePath
}

func (c *RuleBuilderCommand) addInput(path Path) string {
	if c.sbox {
		if rel, isRel, _ := maybeRelErr(c.sboxOutDir.String(), path.String()); isRel {
			return "__SBOX_OUT_DIR__/" + rel
		}
	}
	c.inputs = append(c.inputs, path)
	return path.String()
}

func (c *RuleBuilderCommand) addOrderOnly(path Path) {
	c.orderOnlys = append(c.orderOnlys, path)
}

func (c *RuleBuilderCommand) outputStr(path Path) string {
	if c.sbox {
		// Errors will be handled in RuleBuilder.Build where we have a context to report them
		rel, _, _ := maybeRelErr(c.sboxOutDir.String(), path.String())
		return "__SBOX_OUT_DIR__/" + rel
	}
	return path.String()
}

// Text adds the specified raw text to the command line.  The text should not contain input or output paths or the
// rule will not have them listed in its dependencies or outputs.
func (c *RuleBuilderCommand) Text(text string) *RuleBuilderCommand {
	if c.buf.Len() > 0 {
		c.buf.WriteByte(' ')
	}
	c.buf.WriteString(text)
	return c
}

// Textf adds the specified formatted text to the command line.  The text should not contain input or output paths or
// the rule will not have them listed in its dependencies or outputs.
func (c *RuleBuilderCommand) Textf(format string, a ...interface{}) *RuleBuilderCommand {
	return c.Text(fmt.Sprintf(format, a...))
}

// Flag adds the specified raw text to the command line.  The text should not contain input or output paths or the
// rule will not have them listed in its dependencies or outputs.
func (c *RuleBuilderCommand) Flag(flag string) *RuleBuilderCommand {
	return c.Text(flag)
}

// OptionalFlag adds the specified raw text to the command line if it is not nil.  The text should not contain input or
// output paths or the rule will not have them listed in its dependencies or outputs.
func (c *RuleBuilderCommand) OptionalFlag(flag *string) *RuleBuilderCommand {
	if flag != nil {
		c.Text(*flag)
	}

	return c
}

// Flags adds the specified raw text to the command line.  The text should not contain input or output paths or the
// rule will not have them listed in its dependencies or outputs.
func (c *RuleBuilderCommand) Flags(flags []string) *RuleBuilderCommand {
	for _, flag := range flags {
		c.Text(flag)
	}
	return c
}

// FlagWithArg adds the specified flag and argument text to the command line, with no separator between them.  The flag
// and argument should not contain input or output paths or the rule will not have them listed in its dependencies or
// outputs.
func (c *RuleBuilderCommand) FlagWithArg(flag, arg string) *RuleBuilderCommand {
	return c.Text(flag + arg)
}

// FlagForEachArg adds the specified flag joined with each argument to the command line.  The result is identical to
// calling FlagWithArg for argument.
func (c *RuleBuilderCommand) FlagForEachArg(flag string, args []string) *RuleBuilderCommand {
	for _, arg := range args {
		c.FlagWithArg(flag, arg)
	}
	return c
}

// FlagWithList adds the specified flag and list of arguments to the command line, with the arguments joined by sep
// and no separator between the flag and arguments.  The flag and arguments should not contain input or output paths or
// the rule will not have them listed in its dependencies or outputs.
func (c *RuleBuilderCommand) FlagWithList(flag string, list []string, sep string) *RuleBuilderCommand {
	return c.Text(flag + strings.Join(list, sep))
}

// Tool adds the specified tool path to the command line.  The path will be also added to the dependencies returned by
// RuleBuilder.Tools.
func (c *RuleBuilderCommand) Tool(path Path) *RuleBuilderCommand {
	c.tools = append(c.tools, path)
	return c.Text(path.String())
}

// BuiltTool adds the specified tool path that was built using a host Soong module to the command line.  The path will
// be also added to the dependencies returned by RuleBuilder.Tools.
//
// It is equivalent to:
//  cmd.Tool(ctx.Config().HostToolPath(ctx, tool))
func (c *RuleBuilderCommand) BuiltTool(ctx PathContext, tool string) *RuleBuilderCommand {
	return c.Tool(ctx.Config().HostToolPath(ctx, tool))
}

// PrebuiltBuildTool adds the specified tool path from prebuils/build-tools.  The path will be also added to the
// dependencies returned by RuleBuilder.Tools.
//
// It is equivalent to:
//  cmd.Tool(ctx.Config().PrebuiltBuildTool(ctx, tool))
func (c *RuleBuilderCommand) PrebuiltBuildTool(ctx PathContext, tool string) *RuleBuilderCommand {
	return c.Tool(ctx.Config().PrebuiltBuildTool(ctx, tool))
}

// Input adds the specified input path to the command line.  The path will also be added to the dependencies returned by
// RuleBuilder.Inputs.
func (c *RuleBuilderCommand) Input(path Path) *RuleBuilderCommand {
	return c.Text(c.addInput(path))
}

// Inputs adds the specified input paths to the command line, separated by spaces.  The paths will also be added to the
// dependencies returned by RuleBuilder.Inputs.
func (c *RuleBuilderCommand) Inputs(paths Paths) *RuleBuilderCommand {
	for _, path := range paths {
		c.Input(path)
	}
	return c
}

// Implicit adds the specified input path to the dependencies returned by RuleBuilder.Inputs without modifying the
// command line.
func (c *RuleBuilderCommand) Implicit(path Path) *RuleBuilderCommand {
	c.addInput(path)
	return c
}

// Implicits adds the specified input paths to the dependencies returned by RuleBuilder.Inputs without modifying the
// command line.
func (c *RuleBuilderCommand) Implicits(paths Paths) *RuleBuilderCommand {
	for _, path := range paths {
		c.addInput(path)
	}
	return c
}

// OrderOnly adds the specified input path to the dependencies returned by RuleBuilder.OrderOnlys
// without modifying the command line.
func (c *RuleBuilderCommand) OrderOnly(path Path) *RuleBuilderCommand {
	c.addOrderOnly(path)
	return c
}

// OrderOnlys adds the specified input paths to the dependencies returned by RuleBuilder.OrderOnlys
// without modifying the command line.
func (c *RuleBuilderCommand) OrderOnlys(paths Paths) *RuleBuilderCommand {
	for _, path := range paths {
		c.addOrderOnly(path)
	}
	return c
}

// Output adds the specified output path to the command line.  The path will also be added to the outputs returned by
// RuleBuilder.Outputs.
func (c *RuleBuilderCommand) Output(path WritablePath) *RuleBuilderCommand {
	c.outputs = append(c.outputs, path)
	return c.Text(c.outputStr(path))
}

// Outputs adds the specified output paths to the command line, separated by spaces.  The paths will also be added to
// the outputs returned by RuleBuilder.Outputs.
func (c *RuleBuilderCommand) Outputs(paths WritablePaths) *RuleBuilderCommand {
	for _, path := range paths {
		c.Output(path)
	}
	return c
}

// OutputDir adds the output directory to the command line. This is only available when used with RuleBuilder.Sbox,
// and will be the temporary output directory managed by sbox, not the final one.
func (c *RuleBuilderCommand) OutputDir() *RuleBuilderCommand {
	if !c.sbox {
		panic("OutputDir only valid with Sbox")
	}
	return c.Text("__SBOX_OUT_DIR__")
}

// DepFile adds the specified depfile path to the paths returned by RuleBuilder.DepFiles and adds it to the command
// line, and causes RuleBuilder.Build file to set the depfile flag for ninja.  If multiple depfiles are added to
// commands in a single RuleBuilder then RuleBuilder.Build will add an extra command to merge the depfiles together.
func (c *RuleBuilderCommand) DepFile(path WritablePath) *RuleBuilderCommand {
	c.depFiles = append(c.depFiles, path)
	return c.Text(c.outputStr(path))
}

// ImplicitOutput adds the specified output path to the dependencies returned by RuleBuilder.Outputs without modifying
// the command line.
func (c *RuleBuilderCommand) ImplicitOutput(path WritablePath) *RuleBuilderCommand {
	c.outputs = append(c.outputs, path)
	return c
}

// ImplicitOutputs adds the specified output paths to the dependencies returned by RuleBuilder.Outputs without modifying
// the command line.
func (c *RuleBuilderCommand) ImplicitOutputs(paths WritablePaths) *RuleBuilderCommand {
	c.outputs = append(c.outputs, paths...)
	return c
}

// ImplicitDepFile adds the specified depfile path to the paths returned by RuleBuilder.DepFiles without modifying
// the command line, and causes RuleBuilder.Build file to set the depfile flag for ninja.  If multiple depfiles
// are added to commands in a single RuleBuilder then RuleBuilder.Build will add an extra command to merge the
// depfiles together.
func (c *RuleBuilderCommand) ImplicitDepFile(path WritablePath) *RuleBuilderCommand {
	c.depFiles = append(c.depFiles, path)
	return c
}

// FlagWithInput adds the specified flag and input path to the command line, with no separator between them.  The path
// will also be added to the dependencies returned by RuleBuilder.Inputs.
func (c *RuleBuilderCommand) FlagWithInput(flag string, path Path) *RuleBuilderCommand {
	return c.Text(flag + c.addInput(path))
}

// FlagWithInputList adds the specified flag and input paths to the command line, with the inputs joined by sep
// and no separator between the flag and inputs.  The input paths will also be added to the dependencies returned by
// RuleBuilder.Inputs.
func (c *RuleBuilderCommand) FlagWithInputList(flag string, paths Paths, sep string) *RuleBuilderCommand {
	strs := make([]string, len(paths))
	for i, path := range paths {
		strs[i] = c.addInput(path)
	}
	return c.FlagWithList(flag, strs, sep)
}

// FlagForEachInput adds the specified flag joined with each input path to the command line.  The input paths will also
// be added to the dependencies returned by RuleBuilder.Inputs.  The result is identical to calling FlagWithInput for
// each input path.
func (c *RuleBuilderCommand) FlagForEachInput(flag string, paths Paths) *RuleBuilderCommand {
	for _, path := range paths {
		c.FlagWithInput(flag, path)
	}
	return c
}

// FlagWithOutput adds the specified flag and output path to the command line, with no separator between them.  The path
// will also be added to the outputs returned by RuleBuilder.Outputs.
func (c *RuleBuilderCommand) FlagWithOutput(flag string, path WritablePath) *RuleBuilderCommand {
	c.outputs = append(c.outputs, path)
	return c.Text(flag + c.outputStr(path))
}

// FlagWithDepFile adds the specified flag and depfile path to the command line, with no separator between them.  The path
// will also be added to the outputs returned by RuleBuilder.Outputs.
func (c *RuleBuilderCommand) FlagWithDepFile(flag string, path WritablePath) *RuleBuilderCommand {
	c.depFiles = append(c.depFiles, path)
	return c.Text(flag + c.outputStr(path))
}

// FlagWithRspFileInputList adds the specified flag and path to an rspfile to the command line, with no separator
// between them.  The paths will be written to the rspfile.
func (c *RuleBuilderCommand) FlagWithRspFileInputList(flag string, paths Paths) *RuleBuilderCommand {
	if c.rspFileInputs != nil {
		panic("FlagWithRspFileInputList cannot be called if rsp file inputs have already been provided")
	}

	// Use an empty slice if paths is nil, the non-nil slice is used as an indicator that the rsp file must be
	// generated.
	if paths == nil {
		paths = Paths{}
	}

	c.rspFileInputs = paths

	rspFile := "$out.rsp"
	c.FlagWithArg(flag, rspFile)
	c.unescapedSpans = append(c.unescapedSpans, [2]int{c.buf.Len() - len(rspFile), c.buf.Len()})
	return c
}

// String returns the command line.
func (c *RuleBuilderCommand) String() string {
	return c.buf.String()
}

// String returns the command line.
func (c *RuleBuilderCommand) NinjaEscapedString() string {
	return ninjaEscapeExceptForSpans(c.String(), c.unescapedSpans)
}

func ninjaEscapeExceptForSpans(s string, spans [][2]int) string {
	if len(spans) == 0 {
		return proptools.NinjaEscape(s)
	}

	sb := strings.Builder{}
	sb.Grow(len(s) * 11 / 10)

	i := 0
	for _, span := range spans {
		sb.WriteString(proptools.NinjaEscape(s[i:span[0]]))
		sb.WriteString(s[span[0]:span[1]])
		i = span[1]
	}
	sb.WriteString(proptools.NinjaEscape(s[i:]))

	return sb.String()
}

func ninjaNameEscape(s string) string {
	b := []byte(s)
	escaped := false
	for i, c := range b {
		valid := (c >= 'a' && c <= 'z') ||
			(c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') ||
			(c == '_') ||
			(c == '-') ||
			(c == '.')
		if !valid {
			b[i] = '_'
			escaped = true
		}
	}
	if escaped {
		s = string(b)
	}
	return s
}
