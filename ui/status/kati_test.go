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
	"testing"
)

type lastOutput struct {
	counterOutput

	action *Action
	result ActionResult

	msgLevel MsgLevel
	msg      string
}

func (l *lastOutput) StartAction(a *Action, c Counts) {
	l.action = a
	l.counterOutput.StartAction(a, c)
}
func (l *lastOutput) FinishAction(r ActionResult, c Counts) {
	l.result = r
	l.counterOutput.FinishAction(r, c)
}
func (l *lastOutput) Message(level MsgLevel, msg string) {
	l.msgLevel = level
	l.msg = msg
}
func (l *lastOutput) Flush() {}

func TestKatiNormalCase(t *testing.T) {
	status := &Status{}
	output := &lastOutput{}
	status.AddOutput(output)

	parser := &katiOutputParser{
		st: status.StartTool(),
	}

	msg := "*kati*: verbose msg"
	parser.parseLine(msg)
	output.Expect(t, Counts{})

	if output.msgLevel != VerboseLvl {
		t.Errorf("Expected verbose message, but got %d", output.msgLevel)
	}
	if output.msg != msg {
		t.Errorf("unexpected message contents:\nwant: %q\n got: %q\n", msg, output.msg)
	}

	parser.parseLine("out/build-aosp_arm.ninja is missing, regenerating...")
	output.Expect(t, Counts{})

	parser.parseLine("[1/1] initializing legacy Make module parser ...")
	output.Expect(t, Counts{
		TotalActions:    1,
		RunningActions:  1,
		StartedActions:  1,
		FinishedActions: 0,
	})

	parser.parseLine("[2/5] including out/soong/Android-aosp_arm.mk ...")
	output.Expect(t, Counts{
		TotalActions:    5,
		RunningActions:  1,
		StartedActions:  2,
		FinishedActions: 1,
	})

	parser.parseLine("[3/5] including a ...")
	msg = "a random message"
	parser.parseLine(msg)

	// Start the next line to flush the previous result
	parser.parseLine("[4/5] finishing legacy Make module parsing ...")

	msg += "\n"
	if output.result.Output != msg {
		t.Errorf("output for action did not match:\nwant: %q\n got: %q\n", msg, output.result.Output)
	}

	parser.parseLine("[5/5] writing legacy Make module rules ...")
	parser.parseLine("*kati*: verbose msg")
	parser.flushAction()

	if output.result.Output != "" {
		t.Errorf("expected no output for last action, but got %q", output.result.Output)
	}

	output.Expect(t, Counts{
		TotalActions:    5,
		RunningActions:  0,
		StartedActions:  5,
		FinishedActions: 5,
	})
}

func TestKatiExtraIncludes(t *testing.T) {
	status := &Status{}
	output := &lastOutput{}
	status.AddOutput(output)

	parser := &katiOutputParser{
		st: status.StartTool(),
	}

	parser.parseLine("[1/1] initializing legacy Make module parser ...")
	parser.parseLine("[2/5] including out/soong/Android-aosp_arm.mk ...")
	output.Expect(t, Counts{
		TotalActions:    5,
		RunningActions:  1,
		StartedActions:  2,
		FinishedActions: 1,
	})

	parser.parseLine("including a ...")

	output.Expect(t, Counts{
		TotalActions:    6,
		RunningActions:  1,
		StartedActions:  3,
		FinishedActions: 2,
	})

	parser.parseLine("including b ...")

	output.Expect(t, Counts{
		TotalActions:    7,
		RunningActions:  1,
		StartedActions:  4,
		FinishedActions: 3,
	})

	parser.parseLine("[3/5] finishing legacy Make module parsing ...")

	output.Expect(t, Counts{
		TotalActions:    7,
		RunningActions:  1,
		StartedActions:  5,
		FinishedActions: 4,
	})
}

func TestKatiFailOnError(t *testing.T) {
	status := &Status{}
	output := &lastOutput{}
	status.AddOutput(output)

	parser := &katiOutputParser{
		st: status.StartTool(),
	}

	parser.parseLine("[1/1] initializing legacy Make module parser ...")
	parser.parseLine("[2/5] inclduing out/soong/Android-aosp_arm.mk ...")
	parser.parseLine("build/make/tools/Android.mk:19: error: testing")
	parser.flushAction()

	if output.result.Error == nil {
		t.Errorf("Expected the last action to be marked as an error")
	}
}
