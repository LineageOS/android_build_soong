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
	"bytes"
	"fmt"
	"testing"

	"android/soong/ui/status"
)

func TestStatusOutput(t *testing.T) {
	tests := []struct {
		name  string
		calls func(stat status.StatusOutput)
		smart string
		dumb  string
	}{
		{
			name:  "two actions",
			calls: twoActions,
			smart: "\r[  0% 0/2] action1\x1b[K\r[ 50% 1/2] action1\x1b[K\r[ 50% 1/2] action2\x1b[K\r[100% 2/2] action2\x1b[K\n",
			dumb:  "[ 50% 1/2] action1\n[100% 2/2] action2\n",
		},
		{
			name:  "two parallel actions",
			calls: twoParallelActions,
			smart: "\r[  0% 0/2] action1\x1b[K\r[  0% 0/2] action2\x1b[K\r[ 50% 1/2] action1\x1b[K\r[100% 2/2] action2\x1b[K\n",
			dumb:  "[ 50% 1/2] action1\n[100% 2/2] action2\n",
		},
		{
			name:  "action with output",
			calls: actionsWithOutput,
			smart: "\r[  0% 0/3] action1\x1b[K\r[ 33% 1/3] action1\x1b[K\r[ 33% 1/3] action2\x1b[K\r[ 66% 2/3] action2\x1b[K\noutput1\noutput2\n\r[ 66% 2/3] action3\x1b[K\r[100% 3/3] action3\x1b[K\n",
			dumb:  "[ 33% 1/3] action1\n[ 66% 2/3] action2\noutput1\noutput2\n[100% 3/3] action3\n",
		},
		{
			name:  "action with output without newline",
			calls: actionsWithOutputWithoutNewline,
			smart: "\r[  0% 0/3] action1\x1b[K\r[ 33% 1/3] action1\x1b[K\r[ 33% 1/3] action2\x1b[K\r[ 66% 2/3] action2\x1b[K\noutput1\noutput2\n\r[ 66% 2/3] action3\x1b[K\r[100% 3/3] action3\x1b[K\n",
			dumb:  "[ 33% 1/3] action1\n[ 66% 2/3] action2\noutput1\noutput2\n[100% 3/3] action3\n",
		},
		{
			name:  "action with error",
			calls: actionsWithError,
			smart: "\r[  0% 0/3] action1\x1b[K\r[ 33% 1/3] action1\x1b[K\r[ 33% 1/3] action2\x1b[K\r[ 66% 2/3] action2\x1b[K\nFAILED: f1 f2\ntouch f1 f2\nerror1\nerror2\n\r[ 66% 2/3] action3\x1b[K\r[100% 3/3] action3\x1b[K\n",
			dumb:  "[ 33% 1/3] action1\n[ 66% 2/3] action2\nFAILED: f1 f2\ntouch f1 f2\nerror1\nerror2\n[100% 3/3] action3\n",
		},
		{
			name:  "action with empty description",
			calls: actionWithEmptyDescription,
			smart: "\r[  0% 0/1] command1\x1b[K\r[100% 1/1] command1\x1b[K\n",
			dumb:  "[100% 1/1] command1\n",
		},
		{
			name:  "messages",
			calls: actionsWithMessages,
			smart: "\r[  0% 0/2] action1\x1b[K\r[ 50% 1/2] action1\x1b[K\rstatus\x1b[K\r\x1b[Kprint\nFAILED: error\n\r[ 50% 1/2] action2\x1b[K\r[100% 2/2] action2\x1b[K\n",
			dumb:  "[ 50% 1/2] action1\nstatus\nprint\nFAILED: error\n[100% 2/2] action2\n",
		},
		{
			name:  "action with long description",
			calls: actionWithLongDescription,
			smart: "\r[  0% 0/2] action with very long descrip\x1b[K\r[ 50% 1/2] action with very long descrip\x1b[K\n",
			dumb:  "[ 50% 1/2] action with very long description to test eliding\n",
		},
		{
			name:  "action with output with ansi codes",
			calls: actionWithOuptutWithAnsiCodes,
			smart: "\r[  0% 0/1] action1\x1b[K\r[100% 1/1] action1\x1b[K\n\x1b[31mcolor\x1b[0m\n",
			dumb:  "[100% 1/1] action1\ncolor\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Run("smart", func(t *testing.T) {
				smart := &fakeSmartTerminal{termWidth: 40}
				stat := NewStatusOutput(smart, "", false)
				tt.calls(stat)
				stat.Flush()

				if g, w := smart.String(), tt.smart; g != w {
					t.Errorf("want:\n%q\ngot:\n%q", w, g)
				}
			})

			t.Run("dumb", func(t *testing.T) {
				dumb := &bytes.Buffer{}
				stat := NewStatusOutput(dumb, "", false)
				tt.calls(stat)
				stat.Flush()

				if g, w := dumb.String(), tt.dumb; g != w {
					t.Errorf("want:\n%q\ngot:\n%q", w, g)
				}
			})
		})
	}
}

type runner struct {
	counts status.Counts
	stat   status.StatusOutput
}

func newRunner(stat status.StatusOutput, totalActions int) *runner {
	return &runner{
		counts: status.Counts{TotalActions: totalActions},
		stat:   stat,
	}
}

func (r *runner) startAction(action *status.Action) {
	r.counts.StartedActions++
	r.counts.RunningActions++
	r.stat.StartAction(action, r.counts)
}

func (r *runner) finishAction(result status.ActionResult) {
	r.counts.FinishedActions++
	r.counts.RunningActions--
	r.stat.FinishAction(result, r.counts)
}

func (r *runner) finishAndStartAction(result status.ActionResult, action *status.Action) {
	r.counts.FinishedActions++
	r.stat.FinishAction(result, r.counts)

	r.counts.StartedActions++
	r.stat.StartAction(action, r.counts)
}

var (
	action1 = &status.Action{Description: "action1"}
	result1 = status.ActionResult{Action: action1}
	action2 = &status.Action{Description: "action2"}
	result2 = status.ActionResult{Action: action2}
	action3 = &status.Action{Description: "action3"}
	result3 = status.ActionResult{Action: action3}
)

func twoActions(stat status.StatusOutput) {
	runner := newRunner(stat, 2)
	runner.startAction(action1)
	runner.finishAction(result1)
	runner.startAction(action2)
	runner.finishAction(result2)
}

func twoParallelActions(stat status.StatusOutput) {
	runner := newRunner(stat, 2)
	runner.startAction(action1)
	runner.startAction(action2)
	runner.finishAction(result1)
	runner.finishAction(result2)
}

func actionsWithOutput(stat status.StatusOutput) {
	result2WithOutput := status.ActionResult{Action: action2, Output: "output1\noutput2\n"}

	runner := newRunner(stat, 3)
	runner.startAction(action1)
	runner.finishAction(result1)
	runner.startAction(action2)
	runner.finishAction(result2WithOutput)
	runner.startAction(action3)
	runner.finishAction(result3)
}

func actionsWithOutputWithoutNewline(stat status.StatusOutput) {
	result2WithOutputWithoutNewline := status.ActionResult{Action: action2, Output: "output1\noutput2"}

	runner := newRunner(stat, 3)
	runner.startAction(action1)
	runner.finishAction(result1)
	runner.startAction(action2)
	runner.finishAction(result2WithOutputWithoutNewline)
	runner.startAction(action3)
	runner.finishAction(result3)
}

func actionsWithError(stat status.StatusOutput) {
	action2WithError := &status.Action{Description: "action2", Outputs: []string{"f1", "f2"}, Command: "touch f1 f2"}
	result2WithError := status.ActionResult{Action: action2WithError, Output: "error1\nerror2\n", Error: fmt.Errorf("error1")}

	runner := newRunner(stat, 3)
	runner.startAction(action1)
	runner.finishAction(result1)
	runner.startAction(action2WithError)
	runner.finishAction(result2WithError)
	runner.startAction(action3)
	runner.finishAction(result3)
}

func actionWithEmptyDescription(stat status.StatusOutput) {
	action1 := &status.Action{Command: "command1"}
	result1 := status.ActionResult{Action: action1}

	runner := newRunner(stat, 1)
	runner.startAction(action1)
	runner.finishAction(result1)
}

func actionsWithMessages(stat status.StatusOutput) {
	runner := newRunner(stat, 2)

	runner.startAction(action1)
	runner.finishAction(result1)

	stat.Message(status.VerboseLvl, "verbose")
	stat.Message(status.StatusLvl, "status")
	stat.Message(status.PrintLvl, "print")
	stat.Message(status.ErrorLvl, "error")

	runner.startAction(action2)
	runner.finishAction(result2)
}

func actionWithLongDescription(stat status.StatusOutput) {
	action1 := &status.Action{Description: "action with very long description to test eliding"}
	result1 := status.ActionResult{Action: action1}

	runner := newRunner(stat, 2)

	runner.startAction(action1)

	runner.finishAction(result1)
}

func actionWithOuptutWithAnsiCodes(stat status.StatusOutput) {
	result1WithOutputWithAnsiCodes := status.ActionResult{Action: action1, Output: "\x1b[31mcolor\x1b[0m"}

	runner := newRunner(stat, 1)
	runner.startAction(action1)
	runner.finishAction(result1WithOutputWithAnsiCodes)
}

func TestSmartStatusOutputWidthChange(t *testing.T) {
	smart := &fakeSmartTerminal{termWidth: 40}
	stat := NewStatusOutput(smart, "", false)

	runner := newRunner(stat, 2)

	action := &status.Action{Description: "action with very long description to test eliding"}
	result := status.ActionResult{Action: action}

	runner.startAction(action)
	smart.termWidth = 30
	runner.finishAction(result)

	stat.Flush()

	w := "\r[  0% 0/2] action with very long descrip\x1b[K\r[ 50% 1/2] action with very lo\x1b[K\n"

	if g := smart.String(); g != w {
		t.Errorf("want:\n%q\ngot:\n%q", w, g)
	}
}
