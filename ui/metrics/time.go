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
	"github.com/golang/protobuf/proto"
)

// for testing purpose only
var _now = now

type timeEvent struct {
	desc string
	name string

	// the time that the event started to occur.
	start time.Time
}

type TimeTracer interface {
	Begin(name, desc string, thread tracer.Thread)
	End(thread tracer.Thread) soong_metrics_proto.PerfInfo
}

type timeTracerImpl struct {
	activeEvents []timeEvent
}

var _ TimeTracer = &timeTracerImpl{}

func now() time.Time {
	return time.Now()
}

func (t *timeTracerImpl) Begin(name, desc string, _ tracer.Thread) {
	t.activeEvents = append(t.activeEvents, timeEvent{name: name, desc: desc, start: _now()})
}

func (t *timeTracerImpl) End(tracer.Thread) soong_metrics_proto.PerfInfo {
	if len(t.activeEvents) < 1 {
		panic("Internal error: No pending events for endAt to end!")
	}
	lastEvent := t.activeEvents[len(t.activeEvents)-1]
	t.activeEvents = t.activeEvents[:len(t.activeEvents)-1]
	realTime := uint64(_now().Sub(lastEvent.start).Nanoseconds())

	return soong_metrics_proto.PerfInfo{
		Desc:      proto.String(lastEvent.desc),
		Name:      proto.String(lastEvent.name),
		StartTime: proto.Uint64(uint64(lastEvent.start.UnixNano())),
		RealTime:  proto.Uint64(realTime),
	}
}
