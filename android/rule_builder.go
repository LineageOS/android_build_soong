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
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

// RuleBuilder provides an alternative to ModuleContext.Rule and ModuleContext.Build to add a command line to the build
// graph.
type RuleBuilder struct {
	commands       []*RuleBuilderCommand
	installs       []RuleBuilderInstall
	temporariesSet map[string]bool
	restat         bool
	missingDeps    []string
}

// NewRuleBuilder returns a newly created RuleBuilder.
func NewRuleBuilder() *RuleBuilder {
	return &RuleBuilder{
		temporariesSet: make(map[string]bool),
	}
}

// RuleBuilderInstall is a tuple of install from and to locations.
type RuleBuilderInstall struct {
	From, To string
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

// Install associates an output of the rule with an install location, which can be retrieved later using
// RuleBuilder.Installs.
func (r *RuleBuilder) Install(from, to string) {
	r.installs = append(r.installs, RuleBuilderInstall{from, to})
}

// Command returns a new RuleBuilderCommand for the rule.  The commands will be ordered in the rule by when they were
// created by this method.  That can be mutated through their methods in any order, as long as the mutations do not
// race with any call to Build.
func (r *RuleBuilder) Command() *RuleBuilderCommand {
	command := &RuleBuilderCommand{}
	r.commands = append(r.commands, command)
	return command
}

// Temporary marks an output of a command as an intermediate file that will be used as an input to another command
// in the same rule, and should not be listed in Outputs.
func (r *RuleBuilder) Temporary(path string) {
	r.temporariesSet[path] = true
}

// DeleteTemporaryFiles adds a command to the rule that deletes any outputs that have been marked using Temporary
// when the rule runs.  DeleteTemporaryFiles should be called after all calls to Temporary.
func (r *RuleBuilder) DeleteTemporaryFiles() {
	var temporariesList []string

	for intermediate := range r.temporariesSet {
		temporariesList = append(temporariesList, intermediate)
	}
	sort.Strings(temporariesList)

	r.Command().Text("rm").Flag("-f").Outputs(temporariesList)
}

// Inputs returns the list of paths that were passed to the RuleBuilderCommand methods that take input paths, such
// as RuleBuilderCommand.Input, RuleBuilderComand.Implicit, or RuleBuilderCommand.FlagWithInput.  Inputs to a command
// that are also outputs of another command in the same RuleBuilder are filtered out.
func (r *RuleBuilder) Inputs() []string {
	outputs := r.outputSet()

	inputs := make(map[string]bool)
	for _, c := range r.commands {
		for _, input := range c.inputs {
			if !outputs[input] {
				inputs[input] = true
			}
		}
	}

	var inputList []string
	for input := range inputs {
		inputList = append(inputList, input)
	}
	sort.Strings(inputList)

	return inputList
}

func (r *RuleBuilder) outputSet() map[string]bool {
	outputs := make(map[string]bool)
	for _, c := range r.commands {
		for _, output := range c.outputs {
			outputs[output] = true
		}
	}
	return outputs
}

// Outputs returns the list of paths that were passed to the RuleBuilderCommand methods that take output paths, such
// as RuleBuilderCommand.Output, RuleBuilderCommand.ImplicitOutput, or RuleBuilderCommand.FlagWithInput.
func (r *RuleBuilder) Outputs() []string {
	outputs := r.outputSet()

	var outputList []string
	for output := range outputs {
		if !r.temporariesSet[output] {
			outputList = append(outputList, output)
		}
	}
	sort.Strings(outputList)
	return outputList
}

// Installs returns the list of tuples passed to Install.
func (r *RuleBuilder) Installs() []RuleBuilderInstall {
	return append([]RuleBuilderInstall(nil), r.installs...)
}

func (r *RuleBuilder) toolsSet() map[string]bool {
	tools := make(map[string]bool)
	for _, c := range r.commands {
		for _, tool := range c.tools {
			tools[tool] = true
		}
	}

	return tools
}

// Tools returns the list of paths that were passed to the RuleBuilderCommand.Tool method.
func (r *RuleBuilder) Tools() []string {
	toolsSet := r.toolsSet()

	var toolsList []string
	for tool := range toolsSet {
		toolsList = append(toolsList, tool)
	}
	sort.Strings(toolsList)
	return toolsList
}

// Commands returns a slice containing a the built command line for each call to RuleBuilder.Command.
func (r *RuleBuilder) Commands() []string {
	var commands []string
	for _, c := range r.commands {
		commands = append(commands, string(c.buf))
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

// Build adds the built command line to the build graph, with dependencies on Inputs and Tools, and output files for
// Outputs.
func (r *RuleBuilder) Build(pctx PackageContext, ctx BuilderContext, name string, desc string) {
	// TODO: convert RuleBuilder arguments and storage to Paths
	mctx, _ := ctx.(ModuleContext)
	var inputs Paths
	for _, input := range r.Inputs() {
		// Module output paths
		if mctx != nil {
			rel, isRel := MaybeRel(ctx, PathForModuleOut(mctx).String(), input)
			if isRel {
				inputs = append(inputs, PathForModuleOut(mctx, rel))
				continue
			}
		}

		// Other output paths
		rel, isRel := MaybeRel(ctx, PathForOutput(ctx).String(), input)
		if isRel {
			inputs = append(inputs, PathForOutput(ctx, rel))
			continue
		}

		// TODO: remove this once boot image is moved to where PathForOutput can find it.
		inputs = append(inputs, &unknownRulePath{input})
	}

	var outputs WritablePaths
	for _, output := range r.Outputs() {
		if mctx != nil {
			rel := Rel(ctx, PathForModuleOut(mctx).String(), output)
			outputs = append(outputs, PathForModuleOut(mctx, rel))
		} else {
			rel := Rel(ctx, PathForOutput(ctx).String(), output)
			outputs = append(outputs, PathForOutput(ctx, rel))
		}
	}

	if len(r.missingDeps) > 0 {
		ctx.Build(pctx, BuildParams{
			Rule:        ErrorRule,
			Outputs:     outputs,
			Description: desc,
			Args: map[string]string{
				"error": "missing dependencies: " + strings.Join(r.missingDeps, ", "),
			},
		})
		return
	}

	if len(r.Commands()) > 0 {
		ctx.Build(pctx, BuildParams{
			Rule: ctx.Rule(pctx, name, blueprint.RuleParams{
				Command:     strings.Join(proptools.NinjaEscape(r.Commands()), " && "),
				CommandDeps: r.Tools(),
			}),
			Implicits:   inputs,
			Outputs:     outputs,
			Description: desc,
		})
	}
}

// RuleBuilderCommand is a builder for a command in a command line.  It can be mutated by its methods to add to the
// command and track dependencies.  The methods mutate the RuleBuilderCommand in place, as well as return the
// RuleBuilderCommand, so they can be used chained or unchained.  All methods that add text implicitly add a single
// space as a separator from the previous method.
type RuleBuilderCommand struct {
	buf     []byte
	inputs  []string
	outputs []string
	tools   []string
}

// Text adds the specified raw text to the command line.  The text should not contain input or output paths or the
// rule will not have them listed in its dependencies or outputs.
func (c *RuleBuilderCommand) Text(text string) *RuleBuilderCommand {
	if len(c.buf) > 0 {
		c.buf = append(c.buf, ' ')
	}
	c.buf = append(c.buf, text...)
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

// FlagWithArg adds the specified flag and argument text to the command line, with no separator between them.  The flag
// and argument should not contain input or output paths or the rule will not have them listed in its dependencies or
// outputs.
func (c *RuleBuilderCommand) FlagWithArg(flag, arg string) *RuleBuilderCommand {
	return c.Text(flag + arg)
}

// FlagWithArg adds the specified flag and list of arguments to the command line, with the arguments joined by sep
// and no separator between the flag and arguments.  The flag and arguments should not contain input or output paths or
// the rule will not have them listed in its dependencies or outputs.
func (c *RuleBuilderCommand) FlagWithList(flag string, list []string, sep string) *RuleBuilderCommand {
	return c.Text(flag + strings.Join(list, sep))
}

// Tool adds the specified tool path to the command line.  The path will be also added to the dependencies returned by
// RuleBuilder.Tools.
func (c *RuleBuilderCommand) Tool(path string) *RuleBuilderCommand {
	c.tools = append(c.tools, path)
	return c.Text(path)
}

// Input adds the specified input path to the command line.  The path will also be added to the dependencies returned by
// RuleBuilder.Inputs.
func (c *RuleBuilderCommand) Input(path string) *RuleBuilderCommand {
	c.inputs = append(c.inputs, path)
	return c.Text(path)
}

// Inputs adds the specified input paths to the command line, separated by spaces.  The paths will also be added to the
// dependencies returned by RuleBuilder.Inputs.
func (c *RuleBuilderCommand) Inputs(paths []string) *RuleBuilderCommand {
	for _, path := range paths {
		c.Input(path)
	}
	return c
}

// Implicit adds the specified input path to the dependencies returned by RuleBuilder.Inputs without modifying the
// command line.
func (c *RuleBuilderCommand) Implicit(path string) *RuleBuilderCommand {
	c.inputs = append(c.inputs, path)
	return c
}

// Implicits adds the specified input paths to the dependencies returned by RuleBuilder.Inputs without modifying the
// command line.
func (c *RuleBuilderCommand) Implicits(paths []string) *RuleBuilderCommand {
	c.inputs = append(c.inputs, paths...)
	return c
}

// Output adds the specified output path to the command line.  The path will also be added to the outputs returned by
// RuleBuilder.Outputs.
func (c *RuleBuilderCommand) Output(path string) *RuleBuilderCommand {
	c.outputs = append(c.outputs, path)
	return c.Text(path)
}

// Outputs adds the specified output paths to the command line, separated by spaces.  The paths will also be added to
// the outputs returned by RuleBuilder.Outputs.
func (c *RuleBuilderCommand) Outputs(paths []string) *RuleBuilderCommand {
	for _, path := range paths {
		c.Output(path)
	}
	return c
}

// ImplicitOutput adds the specified output path to the dependencies returned by RuleBuilder.Outputs without modifying
// the command line.
func (c *RuleBuilderCommand) ImplicitOutput(path string) *RuleBuilderCommand {
	c.outputs = append(c.outputs, path)
	return c
}

// ImplicitOutputs adds the specified output paths to the dependencies returned by RuleBuilder.Outputs without modifying
// the command line.
func (c *RuleBuilderCommand) ImplicitOutputs(paths []string) *RuleBuilderCommand {
	c.outputs = append(c.outputs, paths...)
	return c
}

// FlagWithInput adds the specified flag and input path to the command line, with no separator between them.  The path
// will also be added to the dependencies returned by RuleBuilder.Inputs.
func (c *RuleBuilderCommand) FlagWithInput(flag, path string) *RuleBuilderCommand {
	c.inputs = append(c.inputs, path)
	return c.Text(flag + path)
}

// FlagWithInputList adds the specified flag and input paths to the command line, with the inputs joined by sep
// and no separator between the flag and inputs.  The input paths will also be added to the dependencies returned by
// RuleBuilder.Inputs.
func (c *RuleBuilderCommand) FlagWithInputList(flag string, paths []string, sep string) *RuleBuilderCommand {
	c.inputs = append(c.inputs, paths...)
	return c.FlagWithList(flag, paths, sep)
}

// FlagForEachInput adds the specified flag joined with each input path to the command line.  The input paths will also
// be added to the dependencies returned by RuleBuilder.Inputs.  The result is identical to calling FlagWithInput for
// each input path.
func (c *RuleBuilderCommand) FlagForEachInput(flag string, paths []string) *RuleBuilderCommand {
	for _, path := range paths {
		c.FlagWithInput(flag, path)
	}
	return c
}

// FlagWithOutput adds the specified flag and output path to the command line, with no separator between them.  The path
// will also be added to the outputs returned by RuleBuilder.Outputs.
func (c *RuleBuilderCommand) FlagWithOutput(flag, path string) *RuleBuilderCommand {
	c.outputs = append(c.outputs, path)
	return c.Text(flag + path)
}

// String returns the command line.
func (c *RuleBuilderCommand) String() string {
	return string(c.buf)
}

type unknownRulePath struct {
	path string
}

var _ Path = (*unknownRulePath)(nil)

func (p *unknownRulePath) String() string { return p.path }
func (p *unknownRulePath) Ext() string    { return filepath.Ext(p.path) }
func (p *unknownRulePath) Base() string   { return filepath.Base(p.path) }
func (p *unknownRulePath) Rel() string    { return p.path }
