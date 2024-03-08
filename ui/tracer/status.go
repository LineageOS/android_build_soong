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
	"strings"
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

func (s *statusOutput) parseTags(rawTags string) map[string]string {
	if rawTags == "" {
		return nil
	}

	tags := map[string]string{}
	for _, pair := range strings.Split(rawTags, ";") {
		if pair == "" {
			// Ignore empty tag pairs. It's hard to generate these cleanly from
			// make so some tag strings might be something like ";key=value".
			continue
		}
		parts := strings.SplitN(pair, "=", 2)
		tags[parts[0]] = parts[1]
	}
	return tags
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
		Arg: &statsArg{
			UserTime:                   result.Stats.UserTime,
			SystemTime:                 result.Stats.SystemTime,
			MaxRssKB:                   result.Stats.MaxRssKB,
			MinorPageFaults:            result.Stats.MinorPageFaults,
			MajorPageFaults:            result.Stats.MajorPageFaults,
			IOInputKB:                  result.Stats.IOInputKB,
			IOOutputKB:                 result.Stats.IOOutputKB,
			VoluntaryContextSwitches:   result.Stats.VoluntaryContextSwitches,
			InvoluntaryContextSwitches: result.Stats.InvoluntaryContextSwitches,
			Tags:                       s.parseTags(result.Stats.Tags),
		},
	})
}

type statsArg struct {
	UserTime                   uint32            `json:"user_time"`
	SystemTime                 uint32            `json:"system_time_ms"`
	MaxRssKB                   uint64            `json:"max_rss_kb"`
	MinorPageFaults            uint64            `json:"minor_page_faults"`
	MajorPageFaults            uint64            `json:"major_page_faults"`
	IOInputKB                  uint64            `json:"io_input_kb"`
	IOOutputKB                 uint64            `json:"io_output_kb"`
	VoluntaryContextSwitches   uint64            `json:"voluntary_context_switches"`
	InvoluntaryContextSwitches uint64            `json:"involuntary_context_switches"`
	Tags                       map[string]string `json:"tags"`
}

func (s *statusOutput) Flush()                                        {}
func (s *statusOutput) Message(level status.MsgLevel, message string) {}

func (s *statusOutput) Write(p []byte) (int, error) {
	// Discard writes
	return len(p), nil
}
