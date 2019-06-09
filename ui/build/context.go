// Copyright 2017 Google Inc. All rights reserved.
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

package build

import (
	"context"
	"io"

	"android/soong/ui/logger"
	"android/soong/ui/metrics"
	"android/soong/ui/metrics/metrics_proto"
	"android/soong/ui/status"
	"android/soong/ui/tracer"
)

// Context combines a context.Context, logger.Logger, and terminal.Writer.
// These all are agnostic of the current build, and may be used for multiple
// builds, while the Config objects contain per-build information.
type Context struct{ *ContextImpl }
type ContextImpl struct {
	context.Context
	logger.Logger

	Metrics *metrics.Metrics

	Writer io.Writer
	Status *status.Status

	Thread tracer.Thread
	Tracer tracer.Tracer
}

// BeginTrace starts a new Duration Event.
func (c ContextImpl) BeginTrace(name, desc string) {
	if c.Tracer != nil {
		c.Tracer.Begin(desc, c.Thread)
	}
	if c.Metrics != nil {
		c.Metrics.TimeTracer.Begin(name, desc, c.Thread)
	}
}

// EndTrace finishes the last Duration Event.
func (c ContextImpl) EndTrace() {
	if c.Tracer != nil {
		c.Tracer.End(c.Thread)
	}
	if c.Metrics != nil {
		c.Metrics.SetTimeMetrics(c.Metrics.TimeTracer.End(c.Thread))
	}
}

// CompleteTrace writes a trace with a beginning and end times.
func (c ContextImpl) CompleteTrace(name, desc string, begin, end uint64) {
	if c.Tracer != nil {
		c.Tracer.Complete(desc, c.Thread, begin, end)
	}
	if c.Metrics != nil {
		realTime := end - begin
		c.Metrics.SetTimeMetrics(
			metrics_proto.PerfInfo{
				Desc:      &desc,
				Name:      &name,
				StartTime: &begin,
				RealTime:  &realTime})
	}
}
