// Copyright 2018 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package metrics

import (
	"time"

	"android/soong/ui/metrics/metrics_proto"
	"android/soong/ui/tracer"
)

type timeEvent struct {
	desc string
	name string

	atNanos uint64 // timestamp measured in nanoseconds since the reference date
}

type TimeTracer interface {
	Begin(name, desc string, thread tracer.Thread)
	End(thread tracer.Thread) soong_metrics_proto.PerfInfo
}

type timeTracerImpl struct {
	activeEvents []timeEvent
}

var _ TimeTracer = &timeTracerImpl{}

func (t *timeTracerImpl) now() uint64 {
	return uint64(time.Now().UnixNano())
}

func (t *timeTracerImpl) Begin(name, desc string, thread tracer.Thread) {
	t.beginAt(name, desc, t.now())
}

func (t *timeTracerImpl) beginAt(name, desc string, atNanos uint64) {
	t.activeEvents = append(t.activeEvents, timeEvent{name: name, desc: desc, atNanos: atNanos})
}

func (t *timeTracerImpl) End(thread tracer.Thread) soong_metrics_proto.PerfInfo {
	return t.endAt(t.now())
}

func (t *timeTracerImpl) endAt(atNanos uint64) soong_metrics_proto.PerfInfo {
	if len(t.activeEvents) < 1 {
		panic("Internal error: No pending events for endAt to end!")
	}
	lastEvent := t.activeEvents[len(t.activeEvents)-1]
	t.activeEvents = t.activeEvents[:len(t.activeEvents)-1]
	realTime := atNanos - lastEvent.atNanos

	return soong_metrics_proto.PerfInfo{
		Desc:      &lastEvent.desc,
		Name:      &lastEvent.name,
		StartTime: &lastEvent.atNanos,
		RealTime:  &realTime}
}
