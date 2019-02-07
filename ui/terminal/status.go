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

package terminal

import (
	"fmt"
	"strings"
	"time"

	"android/soong/ui/status"
)

type statusOutput struct {
	writer Writer
	format string

	start time.Time
	quiet bool
}

// NewStatusOutput returns a StatusOutput that represents the
// current build status similarly to Ninja's built-in terminal
// output.
//
// statusFormat takes nearly all the same options as NINJA_STATUS.
// %c is currently unsupported.
func NewStatusOutput(w Writer, statusFormat string, quietBuild bool) status.StatusOutput {
	return &statusOutput{
		writer: w,
		format: statusFormat,

		start: time.Now(),
		quiet: quietBuild,
	}
}

func (s *statusOutput) Message(level status.MsgLevel, message string) {
	if level >= status.ErrorLvl {
		s.writer.Print(fmt.Sprintf("FAILED: %s", message))
	} else if level > status.StatusLvl {
		s.writer.Print(fmt.Sprintf("%s%s", level.Prefix(), message))
	} else if level == status.StatusLvl {
		s.writer.StatusLine(message)
	}
}

func (s *statusOutput) StartAction(action *status.Action, counts status.Counts) {
	if !s.writer.isSmartTerminal() {
		return
	}

	str := action.Description
	if str == "" {
		str = action.Command
	}

	s.writer.StatusLine(s.progress(counts) + str)
}

func (s *statusOutput) FinishAction(result status.ActionResult, counts status.Counts) {
	str := result.Description
	if str == "" {
		str = result.Command
	}

	progress := s.progress(counts) + str

	if result.Error != nil {
		targets := strings.Join(result.Outputs, " ")
		if s.quiet || result.Command == "" {
			s.writer.StatusAndMessage(progress, fmt.Sprintf("FAILED: %s\n%s", targets, result.Output))
		} else {
			s.writer.StatusAndMessage(progress, fmt.Sprintf("FAILED: %s\n%s\n%s", targets, result.Command, result.Output))
		}
	} else if result.Output != "" {
		s.writer.StatusAndMessage(progress, result.Output)
	} else {
		s.writer.StatusLine(progress)
	}
}

func (s *statusOutput) Flush() {}

func (s *statusOutput) progress(counts status.Counts) string {
	if s.format == "" {
		return fmt.Sprintf("[%3d%% %d/%d] ", 100*counts.FinishedActions/counts.TotalActions, counts.FinishedActions, counts.TotalActions)
	}

	buf := &strings.Builder{}
	for i := 0; i < len(s.format); i++ {
		c := s.format[i]
		if c != '%' {
			buf.WriteByte(c)
			continue
		}

		i = i + 1
		if i == len(s.format) {
			buf.WriteByte(c)
			break
		}

		c = s.format[i]
		switch c {
		case '%':
			buf.WriteByte(c)
		case 's':
			fmt.Fprintf(buf, "%d", counts.StartedActions)
		case 't':
			fmt.Fprintf(buf, "%d", counts.TotalActions)
		case 'r':
			fmt.Fprintf(buf, "%d", counts.RunningActions)
		case 'u':
			fmt.Fprintf(buf, "%d", counts.TotalActions-counts.StartedActions)
		case 'f':
			fmt.Fprintf(buf, "%d", counts.FinishedActions)
		case 'o':
			fmt.Fprintf(buf, "%.1f", float64(counts.FinishedActions)/time.Since(s.start).Seconds())
		case 'c':
			// TODO: implement?
			buf.WriteRune('?')
		case 'p':
			fmt.Fprintf(buf, "%3d%%", 100*counts.FinishedActions/counts.TotalActions)
		case 'e':
			fmt.Fprintf(buf, "%.3f", time.Since(s.start).Seconds())
		default:
			buf.WriteString("unknown placeholder '")
			buf.WriteByte(c)
			buf.WriteString("'")
		}
	}
	return buf.String()
}
