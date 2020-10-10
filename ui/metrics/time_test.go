// Copyright 2020 Google Inc. All Rights Reserved.
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
	"testing"
	"time"

	"android/soong/ui/tracer"
)

func TestEnd(t *testing.T) {
	startTime := time.Date(2020, time.July, 13, 13, 0, 0, 0, time.UTC)
	dur := time.Nanosecond * 10
	initialNow := _now
	_now = func() time.Time { return startTime.Add(dur) }
	defer func() { _now = initialNow }()

	timeTracer := &timeTracerImpl{}
	timeTracer.activeEvents = append(timeTracer.activeEvents, timeEvent{
		desc:  "test",
		name:  "test",
		start: startTime,
	})

	perf := timeTracer.End(tracer.Thread(0))
	if perf.GetRealTime() != uint64(dur.Nanoseconds()) {
		t.Errorf("got %d, want %d nanoseconds for event duration", perf.GetRealTime(), dur.Nanoseconds())
	}
}
