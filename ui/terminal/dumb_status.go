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

	"android/soong/ui/status"
)

type dumbStatusOutput struct {
	writer    Writer
	formatter formatter
}

// NewDumbStatusOutput returns a StatusOutput that represents the
// current build status similarly to Ninja's built-in terminal
// output.
func NewDumbStatusOutput(w Writer, formatter formatter) status.StatusOutput {
	return &dumbStatusOutput{
		writer:    w,
		formatter: formatter,
	}
}

func (s *dumbStatusOutput) Message(level status.MsgLevel, message string) {
	if level >= status.StatusLvl {
		fmt.Fprintln(s.writer, s.formatter.message(level, message))
	}
}

func (s *dumbStatusOutput) StartAction(action *status.Action, counts status.Counts) {
}

func (s *dumbStatusOutput) FinishAction(result status.ActionResult, counts status.Counts) {
	str := result.Description
	if str == "" {
		str = result.Command
	}

	progress := s.formatter.progress(counts) + str

	output := s.formatter.result(result)
	output = string(stripAnsiEscapes([]byte(output)))

	if output != "" {
		fmt.Fprint(s.writer, progress, "\n", output)
	} else {
		fmt.Fprintln(s.writer, progress)
	}
}

func (s *dumbStatusOutput) Flush() {}
