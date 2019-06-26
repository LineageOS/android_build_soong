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
	"time"

	"android/soong/ui/logger"
)

func NewCriticalPath(log logger.Logger) StatusOutput {
	return &criticalPath{
		log:     log,
		running: make(map[*Action]time.Time),
		nodes:   make(map[string]*node),
		clock:   osClock{},
	}
}

type criticalPath struct {
	log logger.Logger

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

func (cp *criticalPath) StartAction(action *Action, counts Counts) {
	start := cp.clock.Now()
	if cp.start.IsZero() {
		cp.start = start
	}
	cp.running[action] = start
}

func (cp *criticalPath) FinishAction(result ActionResult, counts Counts) {
	if start, ok := cp.running[result.Action]; ok {
		delete(cp.running, result.Action)

		// Determine the input to this edge with the longest cumulative duration
		var criticalPathInput *node
		for _, input := range result.Action.Inputs {
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
			action:             result.Action,
			cumulativeDuration: cumulativeDuration,
			duration:           duration,
			input:              criticalPathInput,
		}

		for _, output := range result.Action.Outputs {
			cp.nodes[output] = node
		}

		cp.end = end
	}
}

func (cp *criticalPath) Flush() {
	criticalPath := cp.criticalPath()

	if len(criticalPath) > 0 {
		// Log the critical path to the verbose log
		criticalTime := criticalPath[0].cumulativeDuration.Round(time.Second)
		cp.log.Verbosef("critical path took %s", criticalTime.String())
		if !cp.start.IsZero() {
			elapsedTime := cp.end.Sub(cp.start).Round(time.Second)
			cp.log.Verbosef("elapsed time %s", elapsedTime.String())
			cp.log.Verbosef("perfect parallelism ratio %d%%",
				int(float64(criticalTime)/float64(elapsedTime)*100))
		}
		cp.log.Verbose("critical path:")
		for i := len(criticalPath) - 1; i >= 0; i-- {
			duration := criticalPath[i].duration
			duration = duration.Round(time.Second)
			seconds := int(duration.Seconds())
			cp.log.Verbosef("   %2d:%02d %s",
				seconds/60, seconds%60, criticalPath[i].action.Description)
		}
	}
}

func (cp *criticalPath) Message(level MsgLevel, msg string) {}

func (cp *criticalPath) Write(p []byte) (n int, err error) { return len(p), nil }

func (cp *criticalPath) criticalPath() []*node {
	var max *node

	// Find the node with the longest critical path
	for _, node := range cp.nodes {
		if max == nil || node.cumulativeDuration > max.cumulativeDuration {
			max = node
		}
	}

	// Follow the critical path back to the leaf node
	var criticalPath []*node
	node := max
	for node != nil {
		criticalPath = append(criticalPath, node)
		node = node.input
	}

	return criticalPath
}
