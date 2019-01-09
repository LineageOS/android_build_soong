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

package dexpreopt

import (
	"fmt"
	"sort"
	"strings"
)

type Install struct {
	From, To string
}

type Rule struct {
	commands []*Command
	installs []Install
}

func (r *Rule) Install(from, to string) {
	r.installs = append(r.installs, Install{from, to})
}

func (r *Rule) Command() *Command {
	command := &Command{}
	r.commands = append(r.commands, command)
	return command
}

func (r *Rule) Inputs() []string {
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

func (r *Rule) outputSet() map[string]bool {
	outputs := make(map[string]bool)
	for _, c := range r.commands {
		for _, output := range c.outputs {
			outputs[output] = true
		}
	}
	return outputs
}

func (r *Rule) Outputs() []string {
	outputs := r.outputSet()

	var outputList []string
	for output := range outputs {
		outputList = append(outputList, output)
	}
	sort.Strings(outputList)
	return outputList
}

func (r *Rule) Installs() []Install {
	return append([]Install(nil), r.installs...)
}

func (r *Rule) Tools() []string {
	var tools []string
	for _, c := range r.commands {
		tools = append(tools, c.tools...)
	}
	return tools
}

func (r *Rule) Commands() []string {
	var commands []string
	for _, c := range r.commands {
		commands = append(commands, string(c.buf))
	}
	return commands
}

type Command struct {
	buf     []byte
	inputs  []string
	outputs []string
	tools   []string
}

func (c *Command) Text(text string) *Command {
	if len(c.buf) > 0 {
		c.buf = append(c.buf, ' ')
	}
	c.buf = append(c.buf, text...)
	return c
}

func (c *Command) Textf(format string, a ...interface{}) *Command {
	return c.Text(fmt.Sprintf(format, a...))
}

func (c *Command) Flag(flag string) *Command {
	return c.Text(flag)
}

func (c *Command) FlagWithArg(flag, arg string) *Command {
	return c.Text(flag + arg)
}

func (c *Command) FlagWithList(flag string, list []string, sep string) *Command {
	return c.Text(flag + strings.Join(list, sep))
}

func (c *Command) Tool(path string) *Command {
	c.tools = append(c.tools, path)
	return c.Text(path)
}

func (c *Command) Input(path string) *Command {
	c.inputs = append(c.inputs, path)
	return c.Text(path)
}

func (c *Command) Implicit(path string) *Command {
	c.inputs = append(c.inputs, path)
	return c
}

func (c *Command) Implicits(paths []string) *Command {
	c.inputs = append(c.inputs, paths...)
	return c
}

func (c *Command) Output(path string) *Command {
	c.outputs = append(c.outputs, path)
	return c.Text(path)
}

func (c *Command) ImplicitOutput(path string) *Command {
	c.outputs = append(c.outputs, path)
	return c
}

func (c *Command) FlagWithInput(flag, path string) *Command {
	c.inputs = append(c.inputs, path)
	return c.Text(flag + path)
}

func (c *Command) FlagWithInputList(flag string, paths []string, sep string) *Command {
	c.inputs = append(c.inputs, paths...)
	return c.FlagWithList(flag, paths, sep)
}

func (c *Command) FlagWithOutput(flag, path string) *Command {
	c.outputs = append(c.outputs, path)
	return c.Text(flag + path)
}
