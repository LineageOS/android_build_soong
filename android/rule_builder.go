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

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	"android/soong/cmd/sbox/sbox_proto"
	"android/soong/remoteexec"
	"android/soong/response"
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
	rbeParams        *remoteexec.REParams
	outDir           WritablePath
	sboxOutSubDir    string
	sboxTools        bool
	sboxInputs       bool
	sboxManifestPath WritablePath
	missingDeps      []string
}

// NewRuleBuilder returns a newly created RuleBuilder.
func NewRuleBuilder(pctx PackageContext, ctx BuilderContext) *RuleBuilder {
	return &RuleBuilder{
		pctx:           pctx,
		ctx:            ctx,
		temporariesSet: make(map[WritablePath]bool),
		sboxOutSubDir:  sboxOutSubDir,
	}
}

// SetSboxOutDirDirAsEmpty sets the out subdirectory to an empty string
// This is useful for sandboxing actions that change the execution root to a path in out/ (e.g mixed builds)
// For such actions, SetSboxOutDirDirAsEmpty ensures that the path does not become $SBOX_SANDBOX_DIR/out/out/bazel/output/execroot/__main__/...
func (rb *RuleBuilder) SetSboxOutDirDirAsEmpty() *RuleBuilder {
	rb.sboxOutSubDir = ""
	return rb
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
func (r *RuleBuilder) Restat() *RuleBuilder {
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

// Rewrapper marks the rule as running inside rewrapper using the given params in order to support
// running on RBE.  During RuleBuilder.Build the params will be combined with the inputs, outputs
// and tools known to RuleBuilder to prepend an appropriate rewrapper command line to the rule's
// command line.
func (r *RuleBuilder) Rewrapper(params *remoteexec.REParams) *RuleBuilder {
	if !r.sboxInputs {
		panic(fmt.Errorf("RuleBuilder.Rewrapper must be called after RuleBuilder.SandboxInputs"))
	}
	r.rbeParams = params
	return r
}

// Sbox marks the rule as needing to be wrapped by sbox. The outputDir should point to the output
// directory that sbox will wipe. It should not be written to by any other rule. manifestPath should
// point to a location where sbox's manifest will be written and must be outside outputDir. sbox
// will ensure that all outputs have been written, and will discard any output files that were not
// specified.
func (r *RuleBuilder) Sbox(outputDir WritablePath, manifestPath WritablePath) *RuleBuilder {
	if r.sbox {
		panic("Sbox() may not be called more than once")
	}
	if len(r.commands) > 0 {
		panic("Sbox() may not be called after Command()")
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

// SandboxInputs enables input sandboxing for the rule by copying any referenced inputs into the
// sandbox.  It also implies SandboxTools().
//
// Sandboxing inputs requires RuleBuilder to be aware of all references to input paths.  Paths
// that are passed to RuleBuilder outside of the methods that expect inputs, for example
// FlagWithArg, must use RuleBuilderCommand.PathForInput to translate the path to one that matches
// the sandbox layout.
func (r *RuleBuilder) SandboxInputs() *RuleBuilder {
	if !r.sbox {
		panic("SandboxInputs() must be called after Sbox()")
	}
	if len(r.commands) > 0 {
		panic("SandboxInputs() may not be called after Command()")
	}
	r.sboxTools = true
	r.sboxInputs = true
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

// Validations returns the list of paths that were passed to RuleBuilderCommand.Validation or
// RuleBuilderCommand.Validations.  The list is sorted and duplicates removed.
func (r *RuleBuilder) Validations() Paths {
	validations := make(map[string]Path)
	for _, c := range r.commands {
		for _, validation := range c.validations {
			validations[validation.String()] = validation
		}
	}

	var validationList Paths
	for _, validation := range validations {
		validationList = append(validationList, validation)
	}

	sort.Slice(validationList, func(i, j int) bool {
		return validationList[i].String() < validationList[j].String()
	})

	return validationList
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
		for _, rspFile := range c.rspFiles {
			rspFileInputs = append(rspFileInputs, rspFile.paths...)
		}
	}

	return rspFileInputs
}

func (r *RuleBuilder) rspFiles() []rspFileAndPaths {
	var rspFiles []rspFileAndPaths
	for _, c := range r.commands {
		rspFiles = append(rspFiles, c.rspFiles...)
	}

	return rspFiles
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
		builtToolWithoutDeps("dep_fixer").
		Inputs(depFiles.Paths())
}

// BuildWithNinjaVars adds the built command line to the build graph, with dependencies on Inputs and Tools, and output files for
// Outputs. This function will not escape Ninja variables, so it may be used to write sandbox manifests using Ninja variables.
func (r *RuleBuilder) BuildWithUnescapedNinjaVars(name string, desc string) {
	r.build(name, desc, false)
}

// Build adds the built command line to the build graph, with dependencies on Inputs and Tools, and output files for
// Outputs.
func (r *RuleBuilder) Build(name string, desc string) {
	r.build(name, desc, true)
}

func (r *RuleBuilder) build(name string, desc string, ninjaEscapeCommandString bool) {
	name = ninjaNameEscape(name)

	if len(r.missingDeps) > 0 {
		r.ctx.Build(r.pctx, BuildParams{
			Rule:        ErrorRule,
			Outputs:     r.Outputs(),
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
	rspFiles := r.rspFiles()

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

		// If sandboxing inputs is enabled, add copy rules to the manifest to copy each input
		// into the sbox directory.
		if r.sboxInputs {
			for _, input := range inputs {
				command.CopyBefore = append(command.CopyBefore, &sbox_proto.Copy{
					From: proto.String(input.String()),
					To:   proto.String(r.sboxPathForInputRel(input)),
				})
			}

			// If using rsp files copy them and their contents into the sbox directory with
			// the appropriate path mappings.
			for _, rspFile := range rspFiles {
				command.RspFiles = append(command.RspFiles, &sbox_proto.RspFile{
					File: proto.String(rspFile.file.String()),
					// These have to match the logic in sboxPathForInputRel
					PathMappings: []*sbox_proto.PathMapping{
						{
							From: proto.String(r.outDir.String()),
							To:   proto.String(sboxOutSubDir),
						},
						{
							From: proto.String(PathForOutput(r.ctx).String()),
							To:   proto.String(sboxOutSubDir),
						},
					},
				})
			}

			command.Chdir = proto.Bool(true)
		}

		// Add copy rules to the manifest to copy each output file from the sbox directory.
		// to the output directory after running the commands.
		for _, output := range outputs {
			rel := Rel(r.ctx, r.outDir.String(), output.String())
			command.CopyAfter = append(command.CopyAfter, &sbox_proto.Copy{
				From: proto.String(filepath.Join(r.sboxOutSubDir, rel)),
				To:   proto.String(output.String()),
			})
		}

		// Outputs that were marked Temporary will not be checked that they are in the output
		// directory by the loop above, check them here.
		for path := range r.temporariesSet {
			Rel(r.ctx, r.outDir.String(), path.String())
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

		// Create a rule to write the manifest as textproto.
		pbText, err := prototext.Marshal(&manifest)
		if err != nil {
			ReportPathErrorf(r.ctx, "sbox manifest failed to marshal: %q", err)
		}
		if ninjaEscapeCommandString {
			WriteFileRule(r.ctx, r.sboxManifestPath, string(pbText))
		} else {
			// We need  to have a rule to write files that is
			// defined on the RuleBuilder's pctx in order to
			// write Ninja variables in the string.
			// The WriteFileRule function above rule can only write
			// raw strings because it is defined on the android
			// package's pctx, and it can't access variables defined
			// in another context.
			r.ctx.Build(r.pctx, BuildParams{
				Rule: r.ctx.Rule(r.pctx, "unescapedWriteFile", blueprint.RuleParams{
					Command:        `rm -rf ${out} && cat ${out}.rsp > ${out}`,
					Rspfile:        "${out}.rsp",
					RspfileContent: "${content}",
					Description:    "write file",
				}, "content"),
				Output:      r.sboxManifestPath,
				Description: "write sbox manifest " + r.sboxManifestPath.Base(),
				Args: map[string]string{
					"content": string(pbText),
				},
			})
		}

		// Generate a new string to use as the command line of the sbox rule.  This uses
		// a RuleBuilderCommand as a convenience method of building the command line, then
		// converts it to a string to replace commandString.
		sboxCmd := &RuleBuilderCommand{
			rule: &RuleBuilder{
				ctx: r.ctx,
			},
		}
		sboxCmd.builtToolWithoutDeps("sbox").
			FlagWithArg("--sandbox-path ", shared.TempDirForOutDir(PathForOutput(r.ctx).String())).
			FlagWithArg("--output-dir ", r.outDir.String()).
			FlagWithInput("--manifest ", r.sboxManifestPath)

		if r.restat {
			sboxCmd.Flag("--write-if-changed")
		}

		// Replace the command string, and add the sbox tool and manifest textproto to the
		// dependencies of the final sbox rule.
		commandString = sboxCmd.buf.String()
		tools = append(tools, sboxCmd.tools...)
		inputs = append(inputs, sboxCmd.inputs...)

		if r.rbeParams != nil {
			// RBE needs a list of input files to copy to the remote builder.  For inputs already
			// listed in an rsp file, pass the rsp file directly to rewrapper.  For the rest,
			// create a new rsp file to pass to rewrapper.
			var remoteRspFiles Paths
			var remoteInputs Paths

			remoteInputs = append(remoteInputs, inputs...)
			remoteInputs = append(remoteInputs, tools...)

			for _, rspFile := range rspFiles {
				remoteInputs = append(remoteInputs, rspFile.file)
				remoteRspFiles = append(remoteRspFiles, rspFile.file)
			}

			if len(remoteInputs) > 0 {
				inputsListFile := r.sboxManifestPath.ReplaceExtension(r.ctx, "rbe_inputs.list")
				writeRspFileRule(r.ctx, inputsListFile, remoteInputs)
				remoteRspFiles = append(remoteRspFiles, inputsListFile)
				// Add the new rsp file as an extra input to the rule.
				inputs = append(inputs, inputsListFile)
			}

			r.rbeParams.OutputFiles = outputs.Strings()
			r.rbeParams.RSPFiles = remoteRspFiles.Strings()
			rewrapperCommand := r.rbeParams.NoVarTemplate(r.ctx.Config().RBEWrapper())
			commandString = rewrapperCommand + " bash -c '" + strings.ReplaceAll(commandString, `'`, `'\''`) + "'"
		}
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
	var rspFileInputs Paths
	if len(rspFiles) > 0 {
		// The first rsp files uses Ninja's rsp file support for the rule
		rspFile = rspFiles[0].file.String()
		// Use "$in" for rspFileContent to avoid duplicating the list of files in the dependency
		// list and in the contents of the rsp file.  Inputs to the rule that are not in the
		// rsp file will be listed in Implicits instead of Inputs so they don't show up in "$in".
		rspFileContent = "$in"
		rspFileInputs = append(rspFileInputs, rspFiles[0].paths...)

		for _, rspFile := range rspFiles[1:] {
			// Any additional rsp files need an extra rule to write the file.
			writeRspFileRule(r.ctx, rspFile.file, rspFile.paths)
			// The main rule needs to depend on the inputs listed in the extra rsp file.
			inputs = append(inputs, rspFile.paths...)
			// The main rule needs to depend on the extra rsp file.
			inputs = append(inputs, rspFile.file)
		}
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

	if ninjaEscapeCommandString {
		commandString = proptools.NinjaEscape(commandString)
	}

	r.ctx.Build(r.pctx, BuildParams{
		Rule: r.ctx.Rule(r.pctx, name, blueprint.RuleParams{
			Command:        commandString,
			CommandDeps:    proptools.NinjaEscapeList(tools.Strings()),
			Restat:         r.restat,
			Rspfile:        proptools.NinjaEscape(rspFile),
			RspfileContent: rspFileContent,
			Pool:           pool,
		}),
		Inputs:          rspFileInputs,
		Implicits:       inputs,
		OrderOnly:       r.OrderOnlys(),
		Validations:     r.Validations(),
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
	validations    Paths
	outputs        WritablePaths
	symlinkOutputs WritablePaths
	depFiles       WritablePaths
	tools          Paths
	packagedTools  []PackagingSpec
	rspFiles       []rspFileAndPaths
}

type rspFileAndPaths struct {
	file  WritablePath
	paths Paths
}

func checkPathNotNil(path Path) {
	if path == nil {
		panic("rule_builder paths cannot be nil")
	}
}

func (c *RuleBuilderCommand) addInput(path Path) string {
	checkPathNotNil(path)
	c.inputs = append(c.inputs, path)
	return c.PathForInput(path)
}

func (c *RuleBuilderCommand) addImplicit(path Path) {
	checkPathNotNil(path)
	c.implicits = append(c.implicits, path)
}

func (c *RuleBuilderCommand) addOrderOnly(path Path) {
	checkPathNotNil(path)
	c.orderOnlys = append(c.orderOnlys, path)
}

// PathForInput takes an input path and returns the appropriate path to use on the command line.  If
// sbox was enabled via a call to RuleBuilder.Sbox() and the path was an output path it returns a
// path with the placeholder prefix used for outputs in sbox.  If sbox is not enabled it returns the
// original path.
func (c *RuleBuilderCommand) PathForInput(path Path) string {
	if c.rule.sbox {
		rel, inSandbox := c.rule._sboxPathForInputRel(path)
		if inSandbox {
			rel = filepath.Join(sboxSandboxBaseDir, rel)
		}
		return rel
	}
	return path.String()
}

// PathsForInputs takes a list of input paths and returns the appropriate paths to use on the
// command line.  If sbox was enabled via a call to RuleBuilder.Sbox() a path was an output path, it
// returns the path with the placeholder prefix used for outputs in sbox.  If sbox is not enabled it
// returns the original paths.
func (c *RuleBuilderCommand) PathsForInputs(paths Paths) []string {
	ret := make([]string, len(paths))
	for i, path := range paths {
		ret[i] = c.PathForInput(path)
	}
	return ret
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

func sboxPathForToolRel(ctx BuilderContext, path Path) string {
	// Errors will be handled in RuleBuilder.Build where we have a context to report them
	toolDir := pathForInstall(ctx, ctx.Config().BuildOS, ctx.Config().BuildArch, "")
	relOutSoong, isRelOutSoong, _ := maybeRelErr(toolDir.String(), path.String())
	if isRelOutSoong {
		// The tool is in the Soong output directory, it will be copied to __SBOX_OUT_DIR__/tools/out
		return filepath.Join(sboxToolsSubDir, "out", relOutSoong)
	}
	// The tool is in the source directory, it will be copied to __SBOX_OUT_DIR__/tools/src
	return filepath.Join(sboxToolsSubDir, "src", path.String())
}

func (r *RuleBuilder) _sboxPathForInputRel(path Path) (rel string, inSandbox bool) {
	// Errors will be handled in RuleBuilder.Build where we have a context to report them
	rel, isRelSboxOut, _ := maybeRelErr(r.outDir.String(), path.String())
	if isRelSboxOut {
		return filepath.Join(sboxOutSubDir, rel), true
	}
	if r.sboxInputs {
		// When sandboxing inputs all inputs have to be copied into the sandbox.  Input files that
		// are outputs of other rules could be an arbitrary absolute path if OUT_DIR is set, so they
		// will be copied to relative paths under __SBOX_OUT_DIR__/out.
		rel, isRelOut, _ := maybeRelErr(PathForOutput(r.ctx).String(), path.String())
		if isRelOut {
			return filepath.Join(sboxOutSubDir, rel), true
		}
	}
	return path.String(), false
}

func (r *RuleBuilder) sboxPathForInputRel(path Path) string {
	rel, _ := r._sboxPathForInputRel(path)
	return rel
}

func (r *RuleBuilder) sboxPathsForInputsRel(paths Paths) []string {
	ret := make([]string, len(paths))
	for i, path := range paths {
		ret[i] = r.sboxPathForInputRel(path)
	}
	return ret
}

func sboxPathForPackagedToolRel(spec PackagingSpec) string {
	return filepath.Join(sboxToolsSubDir, "out", spec.relPathInPackage)
}

// PathForPackagedTool takes a PackageSpec for a tool and returns the corresponding path for the
// tool after copying it into the sandbox.  This can be used  on the RuleBuilder command line to
// reference the tool.
func (c *RuleBuilderCommand) PathForPackagedTool(spec PackagingSpec) string {
	if !c.rule.sboxTools {
		panic("PathForPackagedTool() requires SandboxTools()")
	}

	return filepath.Join(sboxSandboxBaseDir, sboxPathForPackagedToolRel(spec))
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

// PathsForTools takes a list of paths to tools, which may be output files or source files, and
// returns the corresponding paths for the tools in the sbox sandbox if sbox is enabled, or the
// original paths if it is not.  This can be used  on the RuleBuilder command line to reference the tool.
func (c *RuleBuilderCommand) PathsForTools(paths Paths) []string {
	if c.rule.sbox && c.rule.sboxTools {
		var ret []string
		for _, path := range paths {
			ret = append(ret, filepath.Join(sboxSandboxBaseDir, sboxPathForToolRel(c.rule.ctx, path)))
		}
		return ret
	}
	return paths.Strings()
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
	checkPathNotNil(path)
	c.tools = append(c.tools, path)
	return c.Text(c.PathForTool(path))
}

// Tool adds the specified tool path to the dependencies returned by RuleBuilder.Tools.
func (c *RuleBuilderCommand) ImplicitTool(path Path) *RuleBuilderCommand {
	checkPathNotNil(path)
	c.tools = append(c.tools, path)
	return c
}

// Tool adds the specified tool path to the dependencies returned by RuleBuilder.Tools.
func (c *RuleBuilderCommand) ImplicitTools(paths Paths) *RuleBuilderCommand {
	for _, path := range paths {
		c.ImplicitTool(path)
	}
	return c
}

// BuiltTool adds the specified tool path that was built using a host Soong module to the command line.  The path will
// be also added to the dependencies returned by RuleBuilder.Tools.
//
// It is equivalent to:
//
//	cmd.Tool(ctx.Config().HostToolPath(ctx, tool))
func (c *RuleBuilderCommand) BuiltTool(tool string) *RuleBuilderCommand {
	if c.rule.ctx.Config().UseHostMusl() {
		// If the host is using musl, assume that the tool was built against musl libc and include
		// libc_musl.so in the sandbox.
		// TODO(ccross): if we supported adding new dependencies during GenerateAndroidBuildActions
		// this could be a dependency + TransitivePackagingSpecs.
		c.ImplicitTool(c.rule.ctx.Config().HostJNIToolPath(c.rule.ctx, "libc_musl"))
	}
	return c.builtToolWithoutDeps(tool)
}

// builtToolWithoutDeps is similar to BuiltTool, but doesn't add any dependencies.  It is used
// internally by RuleBuilder for helper tools that are known to be compiled statically.
func (c *RuleBuilderCommand) builtToolWithoutDeps(tool string) *RuleBuilderCommand {
	return c.Tool(c.rule.ctx.Config().HostToolPath(c.rule.ctx, tool))
}

// PrebuiltBuildTool adds the specified tool path from prebuils/build-tools.  The path will be also added to the
// dependencies returned by RuleBuilder.Tools.
//
// It is equivalent to:
//
//	cmd.Tool(ctx.Config().PrebuiltBuildTool(ctx, tool))
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

// Validation adds the specified input path to the validation dependencies by
// RuleBuilder.Validations without modifying the command line.
func (c *RuleBuilderCommand) Validation(path Path) *RuleBuilderCommand {
	checkPathNotNil(path)
	c.validations = append(c.validations, path)
	return c
}

// Validations adds the specified input paths to the validation dependencies by
// RuleBuilder.Validations without modifying the command line.
func (c *RuleBuilderCommand) Validations(paths Paths) *RuleBuilderCommand {
	for _, path := range paths {
		c.Validation(path)
	}
	return c
}

// Output adds the specified output path to the command line.  The path will also be added to the outputs returned by
// RuleBuilder.Outputs.
func (c *RuleBuilderCommand) Output(path WritablePath) *RuleBuilderCommand {
	checkPathNotNil(path)
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
	checkPathNotNil(path)
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
	checkPathNotNil(path)
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
	checkPathNotNil(path)
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

// FlagWithRspFileInputList adds the specified flag and path to an rspfile to the command line, with
// no separator between them.  The paths will be written to the rspfile.  If sbox is enabled, the
// rspfile must be outside the sbox directory.  The first use of FlagWithRspFileInputList in any
// RuleBuilderCommand of a RuleBuilder will use Ninja's rsp file support for the rule, additional
// uses will result in an auxiliary rules to write the rspFile contents.
func (c *RuleBuilderCommand) FlagWithRspFileInputList(flag string, rspFile WritablePath, paths Paths) *RuleBuilderCommand {
	// Use an empty slice if paths is nil, the non-nil slice is used as an indicator that the rsp file must be
	// generated.
	if paths == nil {
		paths = Paths{}
	}

	c.rspFiles = append(c.rspFiles, rspFileAndPaths{rspFile, paths})

	if c.rule.sbox {
		if _, isRel, _ := maybeRelErr(c.rule.outDir.String(), rspFile.String()); isRel {
			panic(fmt.Errorf("FlagWithRspFileInputList rspfile %q must not be inside out dir %q",
				rspFile.String(), c.rule.outDir.String()))
		}
	}

	c.FlagWithArg(flag, c.PathForInput(rspFile))
	return c
}

// String returns the command line.
func (c *RuleBuilderCommand) String() string {
	return c.buf.String()
}

// RuleBuilderSboxProtoForTests takes the BuildParams for the manifest passed to RuleBuilder.Sbox()
// and returns sbox testproto generated by the RuleBuilder.
func RuleBuilderSboxProtoForTests(t *testing.T, ctx *TestContext, params TestingBuildParams) *sbox_proto.Manifest {
	t.Helper()
	content := ContentFromFileRuleForTests(t, ctx, params)
	manifest := sbox_proto.Manifest{}
	err := prototext.Unmarshal([]byte(content), &manifest)
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

func writeRspFileRule(ctx BuilderContext, rspFile WritablePath, paths Paths) {
	buf := &strings.Builder{}
	err := response.WriteRspFile(buf, paths.Strings())
	if err != nil {
		// There should never be I/O errors writing to a bytes.Buffer.
		panic(err)
	}
	WriteFileRule(ctx, rspFile, buf.String())
}
