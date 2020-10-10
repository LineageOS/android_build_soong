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

import "testing"

type counterOutput Counts

func (c *counterOutput) StartAction(action *Action, counts Counts) {
	*c = counterOutput(counts)
}
func (c *counterOutput) FinishAction(result ActionResult, counts Counts) {
	*c = counterOutput(counts)
}
func (c counterOutput) Message(level MsgLevel, msg string) {}
func (c counterOutput) Flush()                             {}

func (c counterOutput) Write(p []byte) (int, error) {
	// Discard writes
	return len(p), nil
}

func (c counterOutput) Expect(t *testing.T, counts Counts) {
	if Counts(c) == counts {
		return
	}
	t.Helper()

	if c.TotalActions != counts.TotalActions {
		t.Errorf("Expected %d total edges, but got %d", counts.TotalActions, c.TotalActions)
	}
	if c.RunningActions != counts.RunningActions {
		t.Errorf("Expected %d running edges, but got %d", counts.RunningActions, c.RunningActions)
	}
	if c.StartedActions != counts.StartedActions {
		t.Errorf("Expected %d started edges, but got %d", counts.StartedActions, c.StartedActions)
	}
	if c.FinishedActions != counts.FinishedActions {
		t.Errorf("Expected %d finished edges, but got %d", counts.FinishedActions, c.FinishedActions)
	}
}

func TestBasicUse(t *testing.T) {
	status := &Status{}
	counts := &counterOutput{}
	status.AddOutput(counts)
	s := status.StartTool()

	s.SetTotalActions(2)

	a := &Action{}
	s.StartAction(a)

	counts.Expect(t, Counts{
		TotalActions:    2,
		RunningActions:  1,
		StartedActions:  1,
		FinishedActions: 0,
	})

	s.FinishAction(ActionResult{Action: a})

	counts.Expect(t, Counts{
		TotalActions:    2,
		RunningActions:  0,
		StartedActions:  1,
		FinishedActions: 1,
	})

	a = &Action{}
	s.StartAction(a)

	counts.Expect(t, Counts{
		TotalActions:    2,
		RunningActions:  1,
		StartedActions:  2,
		FinishedActions: 1,
	})

	s.FinishAction(ActionResult{Action: a})

	counts.Expect(t, Counts{
		TotalActions:    2,
		RunningActions:  0,
		StartedActions:  2,
		FinishedActions: 2,
	})
}

// For when a tool claims to have 2 actions, but finishes after one.
func TestFinishEarly(t *testing.T) {
	status := &Status{}
	counts := &counterOutput{}
	status.AddOutput(counts)
	s := status.StartTool()

	s.SetTotalActions(2)

	a := &Action{}
	s.StartAction(a)
	s.FinishAction(ActionResult{Action: a})
	s.Finish()

	s = status.StartTool()
	s.SetTotalActions(2)

	a = &Action{}
	s.StartAction(a)

	counts.Expect(t, Counts{
		TotalActions:    3,
		RunningActions:  1,
		StartedActions:  2,
		FinishedActions: 1,
	})
}

// For when a tool claims to have 1 action, but starts two.
func TestExtraActions(t *testing.T) {
	status := &Status{}
	counts := &counterOutput{}
	status.AddOutput(counts)
	s := status.StartTool()

	s.SetTotalActions(1)

	s.StartAction(&Action{})
	s.StartAction(&Action{})

	counts.Expect(t, Counts{
		TotalActions:    2,
		RunningActions:  2,
		StartedActions:  2,
		FinishedActions: 0,
	})
}

// When a tool calls Finish() with a running Action
func TestRunningWhenFinished(t *testing.T) {
	status := &Status{}
	counts := &counterOutput{}
	status.AddOutput(counts)

	s := status.StartTool()
	s.SetTotalActions(1)
	s.StartAction(&Action{})
	s.Finish()

	s = status.StartTool()
	s.SetTotalActions(1)
	s.StartAction(&Action{})

	counts.Expect(t, Counts{
		TotalActions:    2,
		RunningActions:  2,
		StartedActions:  2,
		FinishedActions: 0,
	})
}
