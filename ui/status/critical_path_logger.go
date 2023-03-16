// Copyright 2023 Google Inc. All rights reserved.
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

// Create a new CriticalPathLogger. if criticalPath is nil, it creates a new criticalPath,
// if not, it uses that.(its purpose is using a critical path outside logger)
func NewCriticalPathLogger(log logger.Logger, criticalPath *CriticalPath) StatusOutput {
	if criticalPath == nil {
		criticalPath = NewCriticalPath()
	}
	return &criticalPathLogger{
		log:          log,
		criticalPath: criticalPath,
	}
}

type criticalPathLogger struct {
	log          logger.Logger
	criticalPath *CriticalPath
}

func (cp *criticalPathLogger) StartAction(action *Action, counts Counts) {
	cp.criticalPath.StartAction(action)
}

func (cp *criticalPathLogger) FinishAction(result ActionResult, counts Counts) {
	cp.criticalPath.FinishAction(result.Action)
}

func (cp *criticalPathLogger) Flush() {
	criticalPath, elapsedTime, criticalTime := cp.criticalPath.criticalPath()

	if len(criticalPath) > 0 {
		cp.log.Verbosef("critical path took %s", criticalTime.String())
		if !cp.criticalPath.start.IsZero() {
			cp.log.Verbosef("elapsed time %s", elapsedTime.String())
			if elapsedTime > 0 {
				cp.log.Verbosef("perfect parallelism ratio %d%%",
					int(float64(criticalTime)/float64(elapsedTime)*100))
			}
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

func (cp *criticalPathLogger) Message(level MsgLevel, msg string) {}

func (cp *criticalPathLogger) Write(p []byte) (n int, err error) { return len(p), nil }
