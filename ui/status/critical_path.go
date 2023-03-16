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
	"android/soong/ui/metrics"

	soong_metrics_proto "android/soong/ui/metrics/metrics_proto"
	"time"

	"google.golang.org/protobuf/proto"
)

func NewCriticalPath() *CriticalPath {
	return &CriticalPath{
		running: make(map[*Action]time.Time),
		nodes:   make(map[string]*node),
		clock:   osClock{},
	}
}

type CriticalPath struct {
	nodes   map[string]*node
	running map[*Action]time.Time

	start, end time.Time

	clock clock
}

type clock interface {
	Now() time.Time
}

type osClock struct{}

func (osClock) Now() time.Time { return time.Now() }

// A critical path node stores the critical path (the minimum time to build the node and all of its dependencies given
// perfect parallelism) for an node.
type node struct {
	action             *Action
	cumulativeDuration time.Duration
	duration           time.Duration
	input              *node
}

func (cp *CriticalPath) StartAction(action *Action) {
	start := cp.clock.Now()
	if cp.start.IsZero() {
		cp.start = start
	}
	cp.running[action] = start
}

func (cp *CriticalPath) FinishAction(action *Action) {
	if start, ok := cp.running[action]; ok {
		delete(cp.running, action)

		// Determine the input to this edge with the longest cumulative duration
		var criticalPathInput *node
		for _, input := range action.Inputs {
			if x := cp.nodes[input]; x != nil {
				if criticalPathInput == nil || x.cumulativeDuration > criticalPathInput.cumulativeDuration {
					criticalPathInput = x
				}
			}
		}

		end := cp.clock.Now()
		duration := end.Sub(start)

		cumulativeDuration := duration
		if criticalPathInput != nil {
			cumulativeDuration += criticalPathInput.cumulativeDuration
		}

		node := &node{
			action:             action,
			cumulativeDuration: cumulativeDuration,
			duration:           duration,
			input:              criticalPathInput,
		}

		for _, output := range action.Outputs {
			cp.nodes[output] = node
		}

		cp.end = end
	}
}

func (cp *CriticalPath) criticalPath() (path []*node, elapsedTime time.Duration, criticalTime time.Duration) {
	var max *node

	// Find the node with the longest critical path
	for _, node := range cp.nodes {
		if max == nil || node.cumulativeDuration > max.cumulativeDuration {
			max = node
		}
	}

	node := max
	for node != nil {
		path = append(path, node)
		node = node.input
	}
	if len(path) > 0 {
		// Log the critical path to the verbose log
		criticalTime = path[0].cumulativeDuration
		if !cp.start.IsZero() {
			elapsedTime = cp.end.Sub(cp.start)
		}
	}
	return
}

func (cp *CriticalPath) longRunningJobs() (nodes []*node) {
	threshold := time.Second * 30
	for _, node := range cp.nodes {
		if node != nil && node.duration > threshold {
			nodes = append(nodes, node)
		}
	}
	return
}

func addJobInfos(jobInfos *[]*soong_metrics_proto.JobInfo, sources []*node) {
	for _, job := range sources {
		jobInfo := soong_metrics_proto.JobInfo{}
		jobInfo.ElapsedTimeMicros = proto.Uint64(uint64(job.duration.Microseconds()))
		jobInfo.JobDescription = &job.action.Description
		*jobInfos = append(*jobInfos, &jobInfo)
	}
}

func (cp *CriticalPath) WriteToMetrics(met *metrics.Metrics) {
	criticalPathInfo := soong_metrics_proto.CriticalPathInfo{}
	path, elapsedTime, criticalTime := cp.criticalPath()
	criticalPathInfo.ElapsedTimeMicros = proto.Uint64(uint64(elapsedTime.Microseconds()))
	criticalPathInfo.CriticalPathTimeMicros = proto.Uint64(uint64(criticalTime.Microseconds()))
	addJobInfos(&criticalPathInfo.LongRunningJobs, cp.longRunningJobs())
	addJobInfos(&criticalPathInfo.CriticalPath, path)
	met.SetCriticalPathInfo(criticalPathInfo)
}
