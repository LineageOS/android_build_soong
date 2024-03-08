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

package status

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

var katiError = regexp.MustCompile(`^(\033\[1m)?[^ ]+:[0-9]+: (\033\[31m)?error:`)
var katiIncludeRe = regexp.MustCompile(`^(\[(\d+)/(\d+)] )?((including [^ ]+|initializing (legacy Make module parser|packaging system)|finishing (legacy Make module parsing|packaging rules)|writing (legacy Make module|packaging) rules) ...)$`)
var katiLogRe = regexp.MustCompile(`^\*kati\*: `)
var katiNinjaMissing = regexp.MustCompile("^[^ ]+ is missing, regenerating...$")

type katiOutputParser struct {
	st ToolStatus

	count int
	total int
	extra int

	action   *Action
	buf      strings.Builder
	hasError bool
}

func (k *katiOutputParser) flushAction() {
	if k.action == nil {
		return
	}

	var err error
	if k.hasError {
		err = fmt.Errorf("makefile error")
	}

	k.st.FinishAction(ActionResult{
		Action: k.action,
		Output: k.buf.String(),
		Error:  err,
	})

	k.buf.Reset()
	k.hasError = false
}

func (k *katiOutputParser) parseLine(line string) {
	// Only put kati debug/stat lines in our verbose log
	if katiLogRe.MatchString(line) {
		k.st.Verbose(line)
		return
	}

	if matches := katiIncludeRe.FindStringSubmatch(line); len(matches) > 0 {
		k.flushAction()
		k.count += 1

		matches := katiIncludeRe.FindStringSubmatch(line)
		if matches[2] != "" {
			idx, err := strconv.Atoi(matches[2])

			if err == nil && idx+k.extra != k.count {
				k.extra = k.count - idx
				k.st.SetTotalActions(k.total + k.extra)
			}
		} else {
			k.extra += 1
			k.st.SetTotalActions(k.total + k.extra)
		}

		if matches[3] != "" {
			tot, err := strconv.Atoi(matches[3])

			if err == nil && tot != k.total {
				k.total = tot
				k.st.SetTotalActions(k.total + k.extra)
			}
		}

		k.action = &Action{
			Description: matches[4],
		}
		k.st.StartAction(k.action)
	} else if k.action != nil {
		if katiError.MatchString(line) {
			k.hasError = true
		}
		k.buf.WriteString(line)
		k.buf.WriteString("\n")
	} else {
		// Before we've started executing actions from Kati
		if line == "No need to regenerate ninja file" || katiNinjaMissing.MatchString(line) {
			k.st.Status(line)
		} else {
			k.st.Print(line)
		}
	}
}

// KatiReader reads the output from Kati, and turns it into Actions and
// messages that are passed into the ToolStatus API.
func KatiReader(st ToolStatus, pipe io.ReadCloser) {
	parser := &katiOutputParser{
		st: st,
	}

	scanner := bufio.NewScanner(pipe)
	scanner.Buffer(nil, 2*1024*1024)
	for scanner.Scan() {
		parser.parseLine(scanner.Text())
	}

	parser.flushAction()

	if err := scanner.Err(); err != nil {
		var buf strings.Builder
		io.Copy(&buf, pipe)
		st.Print(fmt.Sprintf("Error from kati parser: %s", err))
		st.Print(buf.String())
	}

	st.Finish()
}
