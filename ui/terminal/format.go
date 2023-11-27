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
	"strings"
	"time"

	"android/soong/ui/status"
)

type formatter struct {
	format string
	quiet  bool
	start  time.Time
}

// newFormatter returns a formatter for formatting output to
// the terminal in a format similar to Ninja.
// format takes nearly all the same options as NINJA_STATUS.
// %c is currently unsupported.
func newFormatter(format string, quiet bool) formatter {
	return formatter{
		format: format,
		quiet:  quiet,
		start:  time.Now(),
	}
}

func (s formatter) message(level status.MsgLevel, message string) string {
	if level >= status.ErrorLvl {
		return fmt.Sprintf("FAILED: %s", message)
	} else if level > status.StatusLvl {
		return fmt.Sprintf("%s%s", level.Prefix(), message)
	} else if level == status.StatusLvl {
		return message
	}
	return ""
}

func remainingTimeString(t time.Time) string {
	now := time.Now()
	if t.After(now) {
		return t.Sub(now).Round(time.Duration(time.Second)).String()
	}
	return time.Duration(0).Round(time.Duration(time.Second)).String()
}
func (s formatter) progress(counts status.Counts) string {
	if s.format == "" {
		output := fmt.Sprintf("[%3d%% %d/%d", 100*counts.FinishedActions/counts.TotalActions, counts.FinishedActions, counts.TotalActions)

		if !counts.EstimatedTime.IsZero() {
			output += fmt.Sprintf(" %s remaining", remainingTimeString(counts.EstimatedTime))
		}
		output += "] "
		return output
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
		case 'l':
			if counts.EstimatedTime.IsZero() {
				// No esitimated data
				buf.WriteRune('?')
			} else {
				fmt.Fprintf(buf, "%s", remainingTimeString(counts.EstimatedTime))
			}
		default:
			buf.WriteString("unknown placeholder '")
			buf.WriteByte(c)
			buf.WriteString("'")
		}
	}
	return buf.String()
}

func (s formatter) result(result status.ActionResult) string {
	var ret string
	if result.Error != nil {
		targets := strings.Join(result.Outputs, " ")
		if s.quiet || result.Command == "" {
			ret = fmt.Sprintf("FAILED: %s\n%s", targets, result.Output)
		} else {
			ret = fmt.Sprintf("FAILED: %s\n%s\n%s", targets, result.Command, result.Output)
		}
	} else if result.Output != "" {
		ret = result.Output
	}

	if len(ret) > 0 && ret[len(ret)-1] != '\n' {
		ret += "\n"
	}

	return ret
}
