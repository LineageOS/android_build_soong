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

package tracer

import (
	"android/soong/ui/status"
	"time"
)

func (t *tracerImpl) StatusTracer() status.StatusOutput {
	return &statusOutput{
		tracer: t,

		running: map[*status.Action]actionStatus{},
	}
}

type actionStatus struct {
	cpu   int
	start time.Time
}

type statusOutput struct {
	tracer *tracerImpl

	cpus    []bool
	running map[*status.Action]actionStatus
}

func (s *statusOutput) StartAction(action *status.Action, counts status.Counts) {
	cpu := -1
	for i, busy := range s.cpus {
		if !busy {
			cpu = i
			s.cpus[i] = true
			break
		}
	}

	if cpu == -1 {
		cpu = len(s.cpus)
		s.cpus = append(s.cpus, true)
	}

	s.running[action] = actionStatus{
		cpu:   cpu,
		start: time.Now(),
	}
}

func (s *statusOutput) FinishAction(result status.ActionResult, counts status.Counts) {
	start, ok := s.running[result.Action]
	if !ok {
		return
	}
	delete(s.running, result.Action)
	s.cpus[start.cpu] = false

	str := result.Action.Description
	if len(result.Action.Outputs) > 0 {
		str = result.Action.Outputs[0]
	}

	s.tracer.writeEvent(&viewerEvent{
		Name:  str,
		Phase: "X",
		Time:  uint64(start.start.UnixNano()) / 1000,
		Dur:   uint64(time.Since(start.start).Nanoseconds()) / 1000,
		Pid:   1,
		Tid:   uint64(start.cpu),
	})
}

func (s *statusOutput) Flush()                                        {}
func (s *statusOutput) Message(level status.MsgLevel, message string) {}

func (s *statusOutput) Write(p []byte) (int, error) {
	// Discard writes
	return len(p), nil
}
