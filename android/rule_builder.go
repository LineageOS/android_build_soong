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
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"

	"android/soong/cmd/sbox/sbox_proto"
	"android/soong/shared"
)

const sboxSandboxBaseDir = "__SBOX_SANDBOX_DIR__"
const sboxOutSubDir = "out"
const sboxToolsSubDir = "tools"
const sboxOutDir = sboxSandboxBaseDir + "/" + sboxOutSubDir

// RuleBuilder provides an alternative to ModuleContext.Rule and ModuleContext.Build to add a command line to the build
// graph.
type RuleBuilder struct {
	pctx PackageContext
	ctx  BuilderContext

	commands         []*RuleBuilderCommand
	installs         RuleBuilderInstalls
	temporariesSet   map[WritablePath]bool
	restat           bool
	sbox             bool
	highmem          bool
	remoteable       RemoteRuleSupports
	outDir           WritablePath
	sboxTools        bool
	sboxManifestPath WritablePath
	missingDeps      []string
}

// NewRuleBuilder returns a newly created RuleBuilder.
func NewRuleBuilder(pctx PackageContext, ctx BuilderContext) *RuleBuilder {
	return &RuleBuilder{
		pctx:           pctx,
		ctx:            ctx,
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

// Sbox marks the rule as needing to be wrapped by sbox. The outputDir should point to the output
// directory that sbox will wipe. It should not be written to by any other rule. manifestPath should
// point to a location where sbox's manifest will be written and must be outside outputDir. sbox
// will ensure that all outputs have been written, and will discard any output files that were not
// specified.
//
// Sbox is not compatible with Restat()
func (r *RuleBuilder) Sbox(outputDir WritablePath, manifestPath WritablePath) *RuleBuilder {
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
	r.outDir = outputDir
	r.sboxManifestPath = manifestPath
	return r
}

// SandboxTools enables tool sandboxing for the rule by copying any referenced tools into the
// sandbox.
func (r *RuleBuilder) SandboxTools() *RuleBuilder {
	if !r.sbox {
		panic("SandboxTools() must be called after Sbox()")
	}
	if len(r.commands) > 0 {
		panic("SandboxTools() may not be called after Command()")
	}
	r.sboxTools = true
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
		rule: r,
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
// input paths, such as RuleBuilderCommand.Input, RuleBuilderCommand.Implicit, or
// RuleBuilderCommand.FlagWithInput.  Inputs to a command that are also outputs of another command
// in the same RuleBuilder are filtered out.  The list is sorted and duplicates removed.
func (r *RuleBuilder) Inputs() Paths {
	outputs := r.outputSet()
	depFiles := r.depFileSet()

	inputs := make(map[string]Path)
	for _, c := range r.commands {
		for _, input := range append(c.inputs, c.implicits...) {
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

func (r *RuleBuilder) symlinkOutputSet() map[string]WritablePath {
	symlinkOutputs := make(map[string]WritablePath)
	for _, c := range r.commands {
		for _, symlinkOutput := range c.symlinkOutputs {
			symlinkOutputs[symlinkOutput.String()] = symlinkOutput
		}
	}
	return symlinkOutputs
}

// SymlinkOutputs returns the list of paths that the executor (Ninja) would
// verify, after build edge completion, that:
//
// 1) Created output symlinks match the list of paths in this list exactly (no more, no fewer)
// 2) Created output files are *not* declared in this list.
//
// These symlink outputs are expected to be a subset of outputs or implicit
// outputs, or they would fail validation at build param construction time
// later, to support other non-rule-builder approaches for constructing
// statements.
func (r *RuleBuilder) SymlinkOutputs() WritablePaths {
	symlinkOutputs := r.symlinkOutputSet()

	var symlinkOutputList WritablePaths
	for _, symlinkOutput := range symlinkOutputs {
		symlinkOutputList = append(symlinkOutputList, symlinkOutput)
	}

	sort.Slice(symlinkOutputList, func(i, j int) bool {
		return symlinkOutputList[i].String() < symlinkOutputList[j].String()
	})

	return symlinkOutputList
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

// RspFile returns the path to the rspfile that was passed to the RuleBuilderCommand.FlagWithRspFileInputList method.
func (r *RuleBuilder) RspFile() WritablePath {
	var rspFile WritablePath
	for _, c := range r.commands {
		if c.rspFile != nil {
			if rspFile != nil {
				panic("Multiple commands in a rule may not have rsp file inputs")
			}
			rspFile = c.rspFile
		}
	}

	return rspFile
}

// Commands returns a slice containing the built command line for each call to RuleBuilder.Command.
func (r *RuleBuilder) Commands() []string {
	var commands []string
	for _, c := range r.commands {
		commands = append(commands, c.String())
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

func (r *RuleBuilder) depFileMergerCmd(depFiles WritablePaths) *RuleBuilderCommand {
	return r.Command().
		BuiltTool("dep_fixer").
		Inputs(depFiles.Paths())
}

// Build adds the built command line to the build graph, with dependencies on Inputs and Tools, and output files for
// Outputs.
func (r *RuleBuilder) Build(name string, desc string) {
	name = ninjaNameEscape(name)

	if len(r.missingDeps) > 0 {
		r.ctx.Build(pctx, BuildParams{
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
			r.depFileMergerCmd(depFiles)

			if r.sbox {
				// Check for Rel() errors, as all depfiles should be in the output dir.  Errors
				// will be reported to the ctx.
				for _, path := range depFiles[1:] {
					Rel(r.ctx, r.outDir.String(), path.String())
				}
			}
		}
	}

	tools := r.Tools()
	commands := r.Commands()
	outputs := r.Outputs()
	inputs := r.Inputs()
	rspFileInputs := r.RspFileInputs()
	rspFilePath := r.RspFile()

	if len(commands) == 0 {
		return
	}
	if len(outputs) == 0 {
		panic("No outputs specified from any Commands")
	}

	commandString := strings.Join(commands, " && ")

	if r.sbox {
		// If running the command inside sbox, write the rule data out to an sbox
		// manifest.textproto.
		manifest := sbox_proto.Manifest{}
		command := sbox_proto.Command{}
		manifest.Commands = append(manifest.Commands, &command)
		command.Command = proto.String(commandString)

		if depFile != nil {
			manifest.OutputDepfile = proto.String(depFile.String())
		}

		// If sandboxing tools is enabled, add copy rules to the manifest to copy each tool
		// into the sbox directory.
		if r.sboxTools {
			for _, tool := range tools {
				command.CopyBefore = append(command.CopyBefore, &sbox_proto.Copy{
					From: proto.String(tool.String()),
					To:   proto.String(sboxPathForToolRel(r.ctx, tool)),
				})
			}
			for _, c := range r.commands {
				for _, tool := range c.packagedTools {
					command.CopyBefore = append(command.CopyBefore, &sbox_proto.Copy{
						From:       proto.String(tool.srcPath.String()),
						To:         proto.String(sboxPathForPackagedToolRel(tool)),
						Executable: proto.Bool(tool.executable),
					})
					tools = append(tools, tool.srcPath)
				}
			}
		}

		// Add copy rules to the manifest to copy each output file from the sbox directory.
		// to the output directory after running the commands.
		sboxOutputs := make([]string, len(outputs))
		for i, output := range outputs {
			rel := Rel(r.ctx, r.outDir.String(), output.String())
			sboxOutputs[i] = filepath.Join(sboxOutDir, rel)
			command.CopyAfter = append(command.CopyAfter, &sbox_proto.Copy{
				From: proto.String(filepath.Join(sboxOutSubDir, rel)),
				To:   proto.String(output.String()),
			})
		}

		// Add a hash of the list of input files to the manifest so that the textproto file
		// changes when the list of input files changes and causes the sbox rule that
		// depends on it to rerun.
		command.InputHash = proto.String(hashSrcFiles(inputs))

		// Verify that the manifest textproto is not inside the sbox output directory, otherwise
		// it will get deleted when the sbox rule clears its output directory.
		_, manifestInOutDir := MaybeRel(r.ctx, r.outDir.String(), r.sboxManifestPath.String())
		if manifestInOutDir {
			ReportPathErrorf(r.ctx, "sbox rule %q manifestPath %q must not be in outputDir %q",
				name, r.sboxManifestPath.String(), r.outDir.String())
		}

		// Create a rule to write the manifest as a the textproto.
		WriteFileRule(r.ctx, r.sboxManifestPath, proptools.NinjaEscape(proto.MarshalTextString(&manifest)))

		// Generate a new string to use as the command line of the sbox rule.  This uses
		// a RuleBuilderCommand as a convenience method of building the command line, then
		// converts it to a string to replace commandString.
		sboxCmd := &RuleBuilderCommand{
			rule: &RuleBuilder{
				ctx: r.ctx,
			},
		}
		sboxCmd.Text("rm -rf").Output(r.outDir)
		sboxCmd.Text("&&")
		sboxCmd.BuiltTool("sbox").
			Flag("--sandbox-path").Text(shared.TempDirForOutDir(PathForOutput(r.ctx).String())).
			Flag("--manifest").Input(r.sboxManifestPath)

		// Replace the command string, and add the sbox tool and manifest textproto to the
		// dependencies of the final sbox rule.
		commandString = sboxCmd.buf.String()
		tools = append(tools, sboxCmd.tools...)
		inputs = append(inputs, sboxCmd.inputs...)
	} else {
		// If not using sbox the rule will run the command directly, put the hash of the
		// list of input files in a comment at the end of the command line to ensure ninja
		// reruns the rule when the list of input files changes.
		commandString += " # hash of input list: " + hashSrcFiles(inputs)
	}

	// Ninja doesn't like multiple outputs when depfiles are enabled, move all but the first output to
	// ImplicitOutputs.  RuleBuilder doesn't use "$out", so the distinction between Outputs and
	// ImplicitOutputs doesn't matter.
	output := outputs[0]
	implicitOutputs := outputs[1:]

	var rspFile, rspFileContent string
	if rspFilePath != nil {
		rspFile = rspFilePath.String()
		rspFileContent = "$in"
	}

	var pool blueprint.Pool
	if r.ctx.Config().UseGoma() && r.remoteable.Goma {
		// When USE_GOMA=true is set and the rule is supported by goma, allow jobs to run outside the local pool.
	} else if r.ctx.Config().UseRBE() && r.remoteable.RBE {
		// When USE_RBE=true is set and the rule is supported by RBE, use the remotePool.
		pool = remotePool
	} else if r.highmem {
		pool = highmemPool
	} else if r.ctx.Config().UseRemoteBuild() {
		pool = localPool
	}

	r.ctx.Build(r.pctx, BuildParams{
		Rule: r.ctx.Rule(pctx, name, blueprint.RuleParams{
			Command:        proptools.NinjaEscape(commandString),
			CommandDeps:    proptools.NinjaEscapeList(tools.Strings()),
			Restat:         r.restat,
			Rspfile:        proptools.NinjaEscape(rspFile),
			RspfileContent: rspFileContent,
			Pool:           pool,
		}),
		Inputs:          rspFileInputs,
		Implicits:       inputs,
		Output:          output,
		ImplicitOutputs: implicitOutputs,
		SymlinkOutputs:  r.SymlinkOutputs(),
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
	rule *RuleBuilder

	buf            strings.Builder
	inputs         Paths
	implicits      Paths
	orderOnlys     Paths
	outputs        WritablePaths
	symlinkOutputs WritablePaths
	depFiles       WritablePaths
	tools          Paths
	packagedTools  []PackagingSpec
	rspFileInputs  Paths
	rspFile        WritablePath
}

func (c *RuleBuilderCommand) addInput(path Path) string {
	if c.rule.sbox {
		if rel, isRel, _ := maybeRelErr(c.rule.outDir.String(), path.String()); isRel {
			return filepath.Join(sboxOutDir, rel)
		}
	}
	c.inputs = append(c.inputs, path)
	return path.String()
}

func (c *RuleBuilderCommand) addImplicit(path Path) string {
	if c.rule.sbox {
		if rel, isRel, _ := maybeRelErr(c.rule.outDir.String(), path.String()); isRel {
			return filepath.Join(sboxOutDir, rel)
		}
	}
	c.implicits = append(c.implicits, path)
	return path.String()
}

func (c *RuleBuilderCommand) addOrderOnly(path Path) {
	c.orderOnlys = append(c.orderOnlys, path)
}

// PathForOutput takes an output path and returns the appropriate path to use on the command
// line.  If sbox was enabled via a call to RuleBuilder.Sbox(), it returns a path with the
// placeholder prefix used for outputs in sbox.  If sbox is not enabled it returns the
// original path.
func (c *RuleBuilderCommand) PathForOutput(path WritablePath) string {
	if c.rule.sbox {
		// Errors will be handled in RuleBuilder.Build where we have a context to report them
		rel, _, _ := maybeRelErr(c.rule.outDir.String(), path.String())
		return filepath.Join(sboxOutDir, rel)
	}
	return path.String()
}

// SboxPathForTool takes a path to a tool, which may be an output file or a source file, and returns
// the corresponding path for the tool in the sbox sandbox.  It assumes that sandboxing and tool
// sandboxing are enabled.
func SboxPathForTool(ctx BuilderContext, path Path) string {
	return filepath.Join(sboxSandboxBaseDir, sboxPathForToolRel(ctx, path))
}

func sboxPathForToolRel(ctx BuilderContext, path Path) string {
	// Errors will be handled in RuleBuilder.Build where we have a context to report them
	relOut, isRelOut, _ := maybeRelErr(PathForOutput(ctx, "host", ctx.Config().PrebuiltOS()).String(), path.String())
	if isRelOut {
		// The tool is in the output directory, it will be copied to __SBOX_OUT_DIR__/tools/out
		return filepath.Join(sboxToolsSubDir, "out", relOut)
	}
	// The tool is in the source directory, it will be copied to __SBOX_OUT_DIR__/tools/src
	return filepath.Join(sboxToolsSubDir, "src", path.String())
}

// SboxPathForPackagedTool takes a PackageSpec for a tool and returns the corresponding path for the
// tool after copying it into the sandbox.  This can be used  on the RuleBuilder command line to
// reference the tool.
func SboxPathForPackagedTool(spec PackagingSpec) string {
	return filepath.Join(sboxSandboxBaseDir, sboxPathForPackagedToolRel(spec))
}

func sboxPathForPackagedToolRel(spec PackagingSpec) string {
	return filepath.Join(sboxToolsSubDir, "out", spec.relPathInPackage)
}

// PathForTool takes a path to a tool, which may be an output file or a source file, and returns
// the corresponding path for the tool in the sbox sandbox if sbox is enabled, or the original path
// if it is not.  This can be used  on the RuleBuilder command line to reference the tool.
func (c *RuleBuilderCommand) PathForTool(path Path) string {
	if c.rule.sbox && c.rule.sboxTools {
		return filepath.Join(sboxSandboxBaseDir, sboxPathForToolRel(c.rule.ctx, path))
	}
	return path.String()
}

// PackagedTool adds the specified tool path to the command line.  It can only be used with tool
// sandboxing enabled by SandboxTools(), and will copy the tool into the sandbox.
func (c *RuleBuilderCommand) PackagedTool(spec PackagingSpec) *RuleBuilderCommand {
	if !c.rule.sboxTools {
		panic("PackagedTool() requires SandboxTools()")
	}

	c.packagedTools = append(c.packagedTools, spec)
	c.Text(sboxPathForPackagedToolRel(spec))
	return c
}

// ImplicitPackagedTool copies the specified tool into the sandbox without modifying the command
// line.  It can only be used with tool sandboxing enabled by SandboxTools().
func (c *RuleBuilderCommand) ImplicitPackagedTool(spec PackagingSpec) *RuleBuilderCommand {
	if !c.rule.sboxTools {
		panic("ImplicitPackagedTool() requires SandboxTools()")
	}

	c.packagedTools = append(c.packagedTools, spec)
	return c
}

// ImplicitPackagedTools copies the specified tools into the sandbox without modifying the command
// line.  It can only be used with tool sandboxing enabled by SandboxTools().
func (c *RuleBuilderCommand) ImplicitPackagedTools(specs []PackagingSpec) *RuleBuilderCommand {
	if !c.rule.sboxTools {
		panic("ImplicitPackagedTools() requires SandboxTools()")
	}

	c.packagedTools = append(c.packagedTools, specs...)
	return c
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
	return c.Text(c.PathForTool(path))
}

// Tool adds the specified tool path to the dependencies returned by RuleBuilder.Tools.
func (c *RuleBuilderCommand) ImplicitTool(path Path) *RuleBuilderCommand {
	c.tools = append(c.tools, path)
	return c
}

// Tool adds the specified tool path to the dependencies returned by RuleBuilder.Tools.
func (c *RuleBuilderCommand) ImplicitTools(paths Paths) *RuleBuilderCommand {
	c.tools = append(c.tools, paths...)
	return c
}

// BuiltTool adds the specified tool path that was built using a host Soong module to the command line.  The path will
// be also added to the dependencies returned by RuleBuilder.Tools.
//
// It is equivalent to:
//  cmd.Tool(ctx.Config().HostToolPath(ctx, tool))
func (c *RuleBuilderCommand) BuiltTool(tool string) *RuleBuilderCommand {
	return c.Tool(c.rule.ctx.Config().HostToolPath(c.rule.ctx, tool))
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
	c.addImplicit(path)
	return c
}

// Implicits adds the specified input paths to the dependencies returned by RuleBuilder.Inputs without modifying the
// command line.
func (c *RuleBuilderCommand) Implicits(paths Paths) *RuleBuilderCommand {
	for _, path := range paths {
		c.addImplicit(path)
	}
	return c
}

// GetImplicits returns the command's implicit inputs.
func (c *RuleBuilderCommand) GetImplicits() Paths {
	return c.implicits
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
	return c.Text(c.PathForOutput(path))
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
	if !c.rule.sbox {
		panic("OutputDir only valid with Sbox")
	}
	return c.Text(sboxOutDir)
}

// DepFile adds the specified depfile path to the paths returned by RuleBuilder.DepFiles and adds it to the command
// line, and causes RuleBuilder.Build file to set the depfile flag for ninja.  If multiple depfiles are added to
// commands in a single RuleBuilder then RuleBuilder.Build will add an extra command to merge the depfiles together.
func (c *RuleBuilderCommand) DepFile(path WritablePath) *RuleBuilderCommand {
	c.depFiles = append(c.depFiles, path)
	return c.Text(c.PathForOutput(path))
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

// ImplicitSymlinkOutput declares the specified path as an implicit output that
// will be a symlink instead of a regular file. Does not modify the command
// line.
func (c *RuleBuilderCommand) ImplicitSymlinkOutput(path WritablePath) *RuleBuilderCommand {
	c.symlinkOutputs = append(c.symlinkOutputs, path)
	return c.ImplicitOutput(path)
}

// ImplicitSymlinkOutputs declares the specified paths as implicit outputs that
// will be a symlinks instead of regular files. Does not modify the command
// line.
func (c *RuleBuilderCommand) ImplicitSymlinkOutputs(paths WritablePaths) *RuleBuilderCommand {
	for _, path := range paths {
		c.ImplicitSymlinkOutput(path)
	}
	return c
}

// SymlinkOutput declares the specified path as an output that will be a symlink
// instead of a regular file. Modifies the command line.
func (c *RuleBuilderCommand) SymlinkOutput(path WritablePath) *RuleBuilderCommand {
	c.symlinkOutputs = append(c.symlinkOutputs, path)
	return c.Output(path)
}

// SymlinkOutputsl declares the specified paths as outputs that will be symlinks
// instead of regular files. Modifies the command line.
func (c *RuleBuilderCommand) SymlinkOutputs(paths WritablePaths) *RuleBuilderCommand {
	for _, path := range paths {
		c.SymlinkOutput(path)
	}
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
	return c.Text(flag + c.PathForOutput(path))
}

// FlagWithDepFile adds the specified flag and depfile path to the command line, with no separator between them.  The path
// will also be added to the outputs returned by RuleBuilder.Outputs.
func (c *RuleBuilderCommand) FlagWithDepFile(flag string, path WritablePath) *RuleBuilderCommand {
	c.depFiles = append(c.depFiles, path)
	return c.Text(flag + c.PathForOutput(path))
}

// FlagWithRspFileInputList adds the specified flag and path to an rspfile to the command line, with no separator
// between them.  The paths will be written to the rspfile.  If sbox is enabled, the rspfile must
// be outside the sbox directory.
func (c *RuleBuilderCommand) FlagWithRspFileInputList(flag string, rspFile WritablePath, paths Paths) *RuleBuilderCommand {
	if c.rspFileInputs != nil {
		panic("FlagWithRspFileInputList cannot be called if rsp file inputs have already been provided")
	}

	// Use an empty slice if paths is nil, the non-nil slice is used as an indicator that the rsp file must be
	// generated.
	if paths == nil {
		paths = Paths{}
	}

	c.rspFileInputs = paths
	c.rspFile = rspFile

	if c.rule.sbox {
		if _, isRel, _ := maybeRelErr(c.rule.outDir.String(), rspFile.String()); isRel {
			panic(fmt.Errorf("FlagWithRspFileInputList rspfile %q must not be inside out dir %q",
				rspFile.String(), c.rule.outDir.String()))
		}
	}

	c.FlagWithArg(flag, rspFile.String())
	return c
}

// String returns the command line.
func (c *RuleBuilderCommand) String() string {
	return c.buf.String()
}

// RuleBuilderSboxProtoForTests takes the BuildParams for the manifest passed to RuleBuilder.Sbox()
// and returns sbox testproto generated by the RuleBuilder.
func RuleBuilderSboxProtoForTests(t *testing.T, params TestingBuildParams) *sbox_proto.Manifest {
	t.Helper()
	content := ContentFromFileRuleForTests(t, params)
	manifest := sbox_proto.Manifest{}
	err := proto.UnmarshalText(content, &manifest)
	if err != nil {
		t.Fatalf("failed to unmarshal manifest: %s", err.Error())
	}
	return &manifest
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

// hashSrcFiles returns a hash of the list of source files.  It is used to ensure the command line
// or the sbox textproto manifest change even if the input files are not listed on the command line.
func hashSrcFiles(srcFiles Paths) string {
	h := sha256.New()
	srcFileList := strings.Join(srcFiles.Strings(), "\n")
	h.Write([]byte(srcFileList))
	return fmt.Sprintf("%x", h.Sum(nil))
}

// BuilderContextForTesting returns a BuilderContext for the given config that can be used for tests
// that need to call methods that take a BuilderContext.
func BuilderContextForTesting(config Config) BuilderContext {
	pathCtx := PathContextForTesting(config)
	return builderContextForTests{
		PathContext: pathCtx,
	}
}

type builderContextForTests struct {
	PathContext
}

func (builderContextForTests) Rule(PackageContext, string, blueprint.RuleParams, ...string) blueprint.Rule {
	return nil
}
func (builderContextForTests) Build(PackageContext, BuildParams) {}
