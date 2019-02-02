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

type RuleBuilderInstall struct {
	From, To string
}

type RuleBuilder struct {
	commands []*RuleBuilderCommand
	installs []RuleBuilderInstall
	restat   bool
}

func (r *RuleBuilder) Restat() *RuleBuilder {
	r.restat = true
	return r
}

func (r *RuleBuilder) Install(from, to string) {
	r.installs = append(r.installs, RuleBuilderInstall{from, to})
}

func (r *RuleBuilder) Command() *RuleBuilderCommand {
	command := &RuleBuilderCommand{}
	r.commands = append(r.commands, command)
	return command
}

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

func (r *RuleBuilder) Outputs() []string {
	outputs := r.outputSet()

	var outputList []string
	for output := range outputs {
		outputList = append(outputList, output)
	}
	sort.Strings(outputList)
	return outputList
}

func (r *RuleBuilder) Installs() []RuleBuilderInstall {
	return append([]RuleBuilderInstall(nil), r.installs...)
}

func (r *RuleBuilder) Tools() []string {
	var tools []string
	for _, c := range r.commands {
		tools = append(tools, c.tools...)
	}
	return tools
}

func (r *RuleBuilder) Commands() []string {
	var commands []string
	for _, c := range r.commands {
		commands = append(commands, string(c.buf))
	}
	return commands
}

type BuilderContext interface {
	PathContext
	Rule(PackageContext, string, blueprint.RuleParams, ...string) blueprint.Rule
	Build(PackageContext, BuildParams)
}

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

type RuleBuilderCommand struct {
	buf     []byte
	inputs  []string
	outputs []string
	tools   []string
}

func (c *RuleBuilderCommand) Text(text string) *RuleBuilderCommand {
	if len(c.buf) > 0 {
		c.buf = append(c.buf, ' ')
	}
	c.buf = append(c.buf, text...)
	return c
}

func (c *RuleBuilderCommand) Textf(format string, a ...interface{}) *RuleBuilderCommand {
	return c.Text(fmt.Sprintf(format, a...))
}

func (c *RuleBuilderCommand) Flag(flag string) *RuleBuilderCommand {
	return c.Text(flag)
}

func (c *RuleBuilderCommand) FlagWithArg(flag, arg string) *RuleBuilderCommand {
	return c.Text(flag + arg)
}

func (c *RuleBuilderCommand) FlagWithList(flag string, list []string, sep string) *RuleBuilderCommand {
	return c.Text(flag + strings.Join(list, sep))
}

func (c *RuleBuilderCommand) Tool(path string) *RuleBuilderCommand {
	c.tools = append(c.tools, path)
	return c.Text(path)
}

func (c *RuleBuilderCommand) Input(path string) *RuleBuilderCommand {
	c.inputs = append(c.inputs, path)
	return c.Text(path)
}

func (c *RuleBuilderCommand) Implicit(path string) *RuleBuilderCommand {
	c.inputs = append(c.inputs, path)
	return c
}

func (c *RuleBuilderCommand) Implicits(paths []string) *RuleBuilderCommand {
	c.inputs = append(c.inputs, paths...)
	return c
}

func (c *RuleBuilderCommand) Output(path string) *RuleBuilderCommand {
	c.outputs = append(c.outputs, path)
	return c.Text(path)
}

func (c *RuleBuilderCommand) ImplicitOutput(path string) *RuleBuilderCommand {
	c.outputs = append(c.outputs, path)
	return c
}

func (c *RuleBuilderCommand) FlagWithInput(flag, path string) *RuleBuilderCommand {
	c.inputs = append(c.inputs, path)
	return c.Text(flag + path)
}

func (c *RuleBuilderCommand) FlagWithInputList(flag string, paths []string, sep string) *RuleBuilderCommand {
	c.inputs = append(c.inputs, paths...)
	return c.FlagWithList(flag, paths, sep)
}

func (c *RuleBuilderCommand) FlagWithOutput(flag, path string) *RuleBuilderCommand {
	c.outputs = append(c.outputs, path)
	return c.Text(flag + path)
}

type unknownRulePath struct {
	path string
}

var _ Path = (*unknownRulePath)(nil)

func (p *unknownRulePath) String() string { return p.path }
func (p *unknownRulePath) Ext() string    { return filepath.Ext(p.path) }
func (p *unknownRulePath) Base() string   { return filepath.Base(p.path) }
func (p *unknownRulePath) Rel() string    { return p.path }
