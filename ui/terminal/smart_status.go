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
	"strings"
	"sync"

	"android/soong/ui/status"
)

type smartStatusOutput struct {
	writer    io.Writer
	formatter formatter

	lock sync.Mutex

	haveBlankLine bool
}

// NewSmartStatusOutput returns a StatusOutput that represents the
// current build status similarly to Ninja's built-in terminal
// output.
func NewSmartStatusOutput(w io.Writer, formatter formatter) status.StatusOutput {
	return &smartStatusOutput{
		writer:    w,
		formatter: formatter,

		haveBlankLine: true,
	}
}

func (s *smartStatusOutput) Message(level status.MsgLevel, message string) {
	if level < status.StatusLvl {
		return
	}

	str := s.formatter.message(level, message)

	s.lock.Lock()
	defer s.lock.Unlock()

	if level > status.StatusLvl {
		s.print(str)
	} else {
		s.statusLine(str)
	}
}

func (s *smartStatusOutput) StartAction(action *status.Action, counts status.Counts) {
	str := action.Description
	if str == "" {
		str = action.Command
	}

	progress := s.formatter.progress(counts)

	s.lock.Lock()
	defer s.lock.Unlock()

	s.statusLine(progress + str)
}

func (s *smartStatusOutput) FinishAction(result status.ActionResult, counts status.Counts) {
	str := result.Description
	if str == "" {
		str = result.Command
	}

	progress := s.formatter.progress(counts) + str

	output := s.formatter.result(result)

	s.lock.Lock()
	defer s.lock.Unlock()

	if output != "" {
		s.statusLine(progress)
		s.requestLine()
		s.print(output)
	} else {
		s.statusLine(progress)
	}
}

func (s *smartStatusOutput) Flush() {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.requestLine()
}

func (s *smartStatusOutput) requestLine() {
	if !s.haveBlankLine {
		fmt.Fprintln(s.writer)
		s.haveBlankLine = true
	}
}

func (s *smartStatusOutput) print(str string) {
	if !s.haveBlankLine {
		fmt.Fprint(s.writer, "\r", "\x1b[K")
		s.haveBlankLine = true
	}
	fmt.Fprint(s.writer, str)
	if len(str) == 0 || str[len(str)-1] != '\n' {
		fmt.Fprint(s.writer, "\n")
	}
}

func (s *smartStatusOutput) statusLine(str string) {
	idx := strings.IndexRune(str, '\n')
	if idx != -1 {
		str = str[0:idx]
	}

	// Limit line width to the terminal width, otherwise we'll wrap onto
	// another line and we won't delete the previous line.
	//
	// Run this on every line in case the window has been resized while
	// we're printing. This could be optimized to only re-run when we get
	// SIGWINCH if it ever becomes too time consuming.
	if max, ok := termWidth(s.writer); ok {
		if len(str) > max {
			// TODO: Just do a max. Ninja elides the middle, but that's
			// more complicated and these lines aren't that important.
			str = str[:max]
		}
	}

	// Move to the beginning on the line, print the output, then clear
	// the rest of the line.
	fmt.Fprint(s.writer, "\r", str, "\x1b[K")
	s.haveBlankLine = false
}
