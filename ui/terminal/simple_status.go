// Copyright 2019 Google Inc. All rights reserved.
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

package terminal

import (
	"fmt"
	"io"

	"android/soong/ui/status"
)

type simpleStatusOutput struct {
	writer      io.Writer
	formatter   formatter
	keepANSI    bool
	outputLevel status.MsgLevel
}

// NewSimpleStatusOutput returns a StatusOutput that represents the
// current build status similarly to Ninja's built-in terminal
// output.
func NewSimpleStatusOutput(w io.Writer, formatter formatter, keepANSI bool, quietBuild bool) status.StatusOutput {
	level := status.StatusLvl
	if quietBuild {
		level = status.PrintLvl
	}
	return &simpleStatusOutput{
		writer:      w,
		formatter:   formatter,
		keepANSI:    keepANSI,
		outputLevel: level,
	}
}

func (s *simpleStatusOutput) Message(level status.MsgLevel, message string) {
	if level >= s.outputLevel {
		output := s.formatter.message(level, message)
		if !s.keepANSI {
			output = string(stripAnsiEscapes([]byte(output)))
		}
		fmt.Fprintln(s.writer, output)
	}
}

func (s *simpleStatusOutput) StartAction(_ *status.Action, _ status.Counts) {
}

func (s *simpleStatusOutput) FinishAction(result status.ActionResult, counts status.Counts) {
	if s.outputLevel > status.StatusLvl {
		return
	}
	str := result.Description
	if str == "" {
		str = result.Command
	}

	progress := s.formatter.progress(counts) + str

	output := s.formatter.result(result)
	if !s.keepANSI {
		output = string(stripAnsiEscapes([]byte(output)))
	}

	if output != "" {
		fmt.Fprint(s.writer, progress, "\n", output)
	} else {
		fmt.Fprintln(s.writer, progress)
	}
}

func (s *simpleStatusOutput) Flush() {}

func (s *simpleStatusOutput) Write(p []byte) (int, error) {
	fmt.Fprint(s.writer, string(p))
	return len(p), nil
}
