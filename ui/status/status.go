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

// Package status tracks actions run by various tools, combining the counts
// (total actions, currently running, started, finished), and giving that to
// multiple outputs.
package status

import (
	"sync"
)

// Action describes an action taken (or as Ninja calls them, Edges).
type Action struct {
	// Description is a shorter, more readable form of the command, meant
	// for users. It's optional, but one of either Description or Command
	// should be set.
	Description string

	// Outputs is the (optional) list of outputs. Usually these are files,
	// but they can be any string.
	Outputs []string

	// Command is the actual command line executed to perform the action.
	// It's optional, but one of either Description or Command should be
	// set.
	Command string
}

// ActionResult describes the result of running an Action.
type ActionResult struct {
	// Action is a pointer to the original Action struct.
	*Action

	// Output is the output produced by the command (usually stdout&stderr
	// for Actions that run commands)
	Output string

	// Error is nil if the Action succeeded, or set to an error if it
	// failed.
	Error error
}

// Counts describes the number of actions in each state
type Counts struct {
	// TotalActions is the total number of expected changes.  This can
	// generally change up or down during a build, but it should never go
	// below the number of StartedActions
	TotalActions int

	// RunningActions are the number of actions that are currently running
	// -- the number that have called StartAction, but not FinishAction.
	RunningActions int

	// StartedActions are the number of actions that have been started with
	// StartAction.
	StartedActions int

	// FinishedActions are the number of actions that have been finished
	// with FinishAction.
	FinishedActions int
}

// ToolStatus is the interface used by tools to report on their Actions, and to
// present other information through a set of messaging functions.
type ToolStatus interface {
	// SetTotalActions sets the expected total number of actions that will
	// be started by this tool.
	//
	// This call be will ignored if it sets a number that is less than the
	// current number of started actions.
	SetTotalActions(total int)

	// StartAction specifies that the associated action has been started by
	// the tool.
	//
	// A specific *Action should not be specified to StartAction more than
	// once, even if the previous action has already been finished, and the
	// contents rewritten.
	//
	// Do not re-use *Actions between different ToolStatus interfaces
	// either.
	StartAction(action *Action)

	// FinishAction specifies the result of a particular Action.
	//
	// The *Action embedded in the ActionResult structure must have already
	// been passed to StartAction (on this interface).
	//
	// Do not call FinishAction twice for the same *Action.
	FinishAction(result ActionResult)

	// Verbose takes a non-important message that is never printed to the
	// screen, but is in the verbose build log, etc
	Verbose(msg string)
	// Status takes a less important message that may be printed to the
	// screen, but overwritten by another status message. The full message
	// will still appear in the verbose build log.
	Status(msg string)
	// Print takes an message and displays it to the screen and other
	// output logs, etc.
	Print(msg string)
	// Error is similar to Print, but treats it similarly to a failed
	// action, showing it in the error logs, etc.
	Error(msg string)

	// Finish marks the end of all Actions being run by this tool.
	//
	// SetTotalEdges, StartAction, and FinishAction should not be called
	// after Finish.
	Finish()
}

// MsgLevel specifies the importance of a particular log message. See the
// descriptions in ToolStatus: Verbose, Status, Print, Error.
type MsgLevel int

const (
	VerboseLvl MsgLevel = iota
	StatusLvl
	PrintLvl
	ErrorLvl
)

func (l MsgLevel) Prefix() string {
	switch l {
	case VerboseLvl:
		return "verbose: "
	case StatusLvl:
		return "status: "
	case PrintLvl:
		return ""
	case ErrorLvl:
		return "error: "
	default:
		panic("Unknown message level")
	}
}

// StatusOutput is the interface used to get status information as a Status
// output.
//
// All of the functions here are guaranteed to be called by Status while
// holding it's internal lock, so it's safe to assume a single caller at any
// time, and that the ordering of calls will be correct. It is not safe to call
// back into the Status, or one of its ToolStatus interfaces.
type StatusOutput interface {
	// StartAction will be called once every time ToolStatus.StartAction is
	// called. counts will include the current counters across all
	// ToolStatus instances, including ones that have been finished.
	StartAction(action *Action, counts Counts)

	// FinishAction will be called once every time ToolStatus.FinishAction
	// is called. counts will include the current counters across all
	// ToolStatus instances, including ones that have been finished.
	FinishAction(result ActionResult, counts Counts)

	// Message is the equivalent of ToolStatus.Verbose/Status/Print/Error,
	// but the level is specified as an argument.
	Message(level MsgLevel, msg string)

	// Flush is called when your outputs should be flushed / closed. No
	// output is expected after this call.
	Flush()

	// Write lets StatusOutput implement io.Writer
	Write(p []byte) (n int, err error)
}

// Status is the multiplexer / accumulator between ToolStatus instances (via
// StartTool) and StatusOutputs (via AddOutput). There's generally one of these
// per build process (though tools like multiproduct_kati may have multiple
// independent versions).
type Status struct {
	counts  Counts
	outputs []StatusOutput

	// Protects counts and outputs, and allows each output to
	// expect only a single caller at a time.
	lock sync.Mutex
}

// AddOutput attaches an output to this object. It's generally expected that an
// output is attached to a single Status instance.
func (s *Status) AddOutput(output StatusOutput) {
	if output == nil {
		return
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	s.outputs = append(s.outputs, output)
}

// StartTool returns a new ToolStatus instance to report the status of a tool.
func (s *Status) StartTool() ToolStatus {
	return &toolStatus{
		status: s,
	}
}

// Finish will call Flush on all the outputs, generally flushing or closing all
// of their outputs. Do not call any other functions on this instance or any
// associated ToolStatus instances after this has been called.
func (s *Status) Finish() {
	s.lock.Lock()
	defer s.lock.Unlock()

	for _, o := range s.outputs {
		o.Flush()
	}
}

func (s *Status) updateTotalActions(diff int) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.counts.TotalActions += diff
}

func (s *Status) startAction(action *Action) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.counts.RunningActions += 1
	s.counts.StartedActions += 1

	for _, o := range s.outputs {
		o.StartAction(action, s.counts)
	}
}

func (s *Status) finishAction(result ActionResult) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.counts.RunningActions -= 1
	s.counts.FinishedActions += 1

	for _, o := range s.outputs {
		o.FinishAction(result, s.counts)
	}
}

func (s *Status) message(level MsgLevel, msg string) {
	s.lock.Lock()
	defer s.lock.Unlock()

	for _, o := range s.outputs {
		o.Message(level, msg)
	}
}

func (s *Status) Status(msg string) {
	s.message(StatusLvl, msg)
}

type toolStatus struct {
	status *Status

	counts Counts
	// Protects counts
	lock sync.Mutex
}

var _ ToolStatus = (*toolStatus)(nil)

func (d *toolStatus) SetTotalActions(total int) {
	diff := 0

	d.lock.Lock()
	if total >= d.counts.StartedActions && total != d.counts.TotalActions {
		diff = total - d.counts.TotalActions
		d.counts.TotalActions = total
	}
	d.lock.Unlock()

	if diff != 0 {
		d.status.updateTotalActions(diff)
	}
}

func (d *toolStatus) StartAction(action *Action) {
	totalDiff := 0

	d.lock.Lock()
	d.counts.RunningActions += 1
	d.counts.StartedActions += 1

	if d.counts.StartedActions > d.counts.TotalActions {
		totalDiff = d.counts.StartedActions - d.counts.TotalActions
		d.counts.TotalActions = d.counts.StartedActions
	}
	d.lock.Unlock()

	if totalDiff != 0 {
		d.status.updateTotalActions(totalDiff)
	}
	d.status.startAction(action)
}

func (d *toolStatus) FinishAction(result ActionResult) {
	d.lock.Lock()
	d.counts.RunningActions -= 1
	d.counts.FinishedActions += 1
	d.lock.Unlock()

	d.status.finishAction(result)
}

func (d *toolStatus) Verbose(msg string) {
	d.status.message(VerboseLvl, msg)
}
func (d *toolStatus) Status(msg string) {
	d.status.message(StatusLvl, msg)
}
func (d *toolStatus) Print(msg string) {
	d.status.message(PrintLvl, msg)
}
func (d *toolStatus) Error(msg string) {
	d.status.message(ErrorLvl, msg)
}

func (d *toolStatus) Finish() {
	d.lock.Lock()
	defer d.lock.Unlock()

	if d.counts.TotalActions != d.counts.StartedActions {
		d.status.updateTotalActions(d.counts.StartedActions - d.counts.TotalActions)
	}

	// TODO: update status to correct running/finished edges?
	d.counts.RunningActions = 0
	d.counts.TotalActions = d.counts.StartedActions
}
