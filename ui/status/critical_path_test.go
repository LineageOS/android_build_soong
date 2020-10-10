// Copyright 2019 Google Inc. All rights reserved.
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
	"reflect"
	"testing"
	"time"
)

type testCriticalPath struct {
	*criticalPath
	Counts

	actions map[int]*Action
}

type testClock time.Time

func (t testClock) Now() time.Time { return time.Time(t) }

func (t *testCriticalPath) start(id int, startTime time.Duration, outputs, inputs []string) {
	t.clock = testClock(time.Unix(0, 0).Add(startTime))
	action := &Action{
		Description: outputs[0],
		Outputs:     outputs,
		Inputs:      inputs,
	}

	t.actions[id] = action
	t.StartAction(action, t.Counts)
}

func (t *testCriticalPath) finish(id int, endTime time.Duration) {
	t.clock = testClock(time.Unix(0, 0).Add(endTime))
	t.FinishAction(ActionResult{
		Action: t.actions[id],
	}, t.Counts)
}

func TestCriticalPath(t *testing.T) {
	tests := []struct {
		name     string
		msgs     func(*testCriticalPath)
		want     []string
		wantTime time.Duration
	}{
		{
			name: "empty",
			msgs: func(cp *testCriticalPath) {},
		},
		{
			name: "duplicate",
			msgs: func(cp *testCriticalPath) {
				cp.start(0, 0, []string{"a"}, nil)
				cp.start(1, 0, []string{"a"}, nil)
				cp.finish(0, 1000)
				cp.finish(0, 2000)
			},
			want:     []string{"a"},
			wantTime: 1000,
		},
		{
			name: "linear",
			//  a
			//  |
			//  b
			//  |
			//  c
			msgs: func(cp *testCriticalPath) {
				cp.start(0, 0, []string{"a"}, nil)
				cp.finish(0, 1000)
				cp.start(1, 1000, []string{"b"}, []string{"a"})
				cp.finish(1, 2000)
				cp.start(2, 3000, []string{"c"}, []string{"b"})
				cp.finish(2, 4000)
			},
			want:     []string{"c", "b", "a"},
			wantTime: 3000,
		},
		{
			name: "diamond",
			//  a
			//  |\
			//  b c
			//  |/
			//  d
			msgs: func(cp *testCriticalPath) {
				cp.start(0, 0, []string{"a"}, nil)
				cp.finish(0, 1000)
				cp.start(1, 1000, []string{"b"}, []string{"a"})
				cp.start(2, 1000, []string{"c"}, []string{"a"})
				cp.finish(1, 2000)
				cp.finish(2, 3000)
				cp.start(3, 3000, []string{"d"}, []string{"b", "c"})
				cp.finish(3, 4000)
			},
			want:     []string{"d", "c", "a"},
			wantTime: 4000,
		},
		{
			name: "multiple",
			//  a d
			//  | |
			//  b e
			//  |
			//  c
			msgs: func(cp *testCriticalPath) {
				cp.start(0, 0, []string{"a"}, nil)
				cp.start(3, 0, []string{"d"}, nil)
				cp.finish(0, 1000)
				cp.finish(3, 1000)
				cp.start(1, 1000, []string{"b"}, []string{"a"})
				cp.start(4, 1000, []string{"e"}, []string{"d"})
				cp.finish(1, 2000)
				cp.start(2, 2000, []string{"c"}, []string{"b"})
				cp.finish(2, 3000)
				cp.finish(4, 4000)

			},
			want:     []string{"e", "d"},
			wantTime: 4000,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cp := &testCriticalPath{
				criticalPath: NewCriticalPath(nil).(*criticalPath),
				actions:      make(map[int]*Action),
			}

			tt.msgs(cp)

			criticalPath := cp.criticalPath.criticalPath()

			var descs []string
			for _, x := range criticalPath {
				descs = append(descs, x.action.Description)
			}

			if !reflect.DeepEqual(descs, tt.want) {
				t.Errorf("criticalPath.criticalPath() = %v, want %v", descs, tt.want)
			}

			var gotTime time.Duration
			if len(criticalPath) > 0 {
				gotTime = criticalPath[0].cumulativeDuration
			}
			if gotTime != tt.wantTime {
				t.Errorf("cumulativeDuration[0].cumulativeDuration = %v, want %v", gotTime, tt.wantTime)
			}
		})
	}
}
