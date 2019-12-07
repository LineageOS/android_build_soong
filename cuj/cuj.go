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

// This executable runs a series of build commands to test and benchmark some critical user journeys.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"android/soong/ui/build"
	"android/soong/ui/logger"
	"android/soong/ui/metrics"
	"android/soong/ui/status"
	"android/soong/ui/terminal"
	"android/soong/ui/tracer"
)

type Test struct {
	name string
	args []string

	results TestResults
}

type TestResults struct {
	metrics *metrics.Metrics
	err     error
}

// Run runs a single build command.  It emulates the "m" command line by calling into Soong UI directly.
func (t *Test) Run(logsDir string) {
	output := terminal.NewStatusOutput(os.Stdout, "", false, false)

	log := logger.New(output)
	defer log.Cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	trace := tracer.New(log)
	defer trace.Close()

	met := metrics.New()

	stat := &status.Status{}
	defer stat.Finish()
	stat.AddOutput(output)
	stat.AddOutput(trace.StatusTracer())

	build.SetupSignals(log, cancel, func() {
		trace.Close()
		log.Cleanup()
		stat.Finish()
	})

	buildCtx := build.Context{ContextImpl: &build.ContextImpl{
		Context: ctx,
		Logger:  log,
		Metrics: met,
		Tracer:  trace,
		Writer:  output,
		Status:  stat,
	}}

	defer logger.Recover(func(err error) {
		t.results.err = err
	})

	config := build.NewConfig(buildCtx, t.args...)
	build.SetupOutDir(buildCtx, config)

	os.MkdirAll(logsDir, 0777)
	log.SetOutput(filepath.Join(logsDir, "soong.log"))
	trace.SetOutput(filepath.Join(logsDir, "build.trace"))
	stat.AddOutput(status.NewVerboseLog(log, filepath.Join(logsDir, "verbose.log")))
	stat.AddOutput(status.NewErrorLog(log, filepath.Join(logsDir, "error.log")))
	stat.AddOutput(status.NewProtoErrorLog(log, filepath.Join(logsDir, "build_error")))
	stat.AddOutput(status.NewCriticalPath(log))

	defer met.Dump(filepath.Join(logsDir, "soong_metrics"))

	if start, ok := os.LookupEnv("TRACE_BEGIN_SOONG"); ok {
		if !strings.HasSuffix(start, "N") {
			if start_time, err := strconv.ParseUint(start, 10, 64); err == nil {
				log.Verbosef("Took %dms to start up.",
					time.Since(time.Unix(0, int64(start_time))).Nanoseconds()/time.Millisecond.Nanoseconds())
				buildCtx.CompleteTrace(metrics.RunSetupTool, "startup", start_time, uint64(time.Now().UnixNano()))
			}
		}

		if executable, err := os.Executable(); err == nil {
			trace.ImportMicrofactoryLog(filepath.Join(filepath.Dir(executable), "."+filepath.Base(executable)+".trace"))
		}
	}

	f := build.NewSourceFinder(buildCtx, config)
	defer f.Shutdown()
	build.FindSources(buildCtx, config, f)

	build.Build(buildCtx, config, build.BuildAll)

	t.results.metrics = met
}

func main() {
	outDir := os.Getenv("OUT_DIR")
	if outDir == "" {
		outDir = "out"
	}

	cujDir := filepath.Join(outDir, "cuj_tests")

	// Use a subdirectory for the out directory for the tests to keep them isolated.
	os.Setenv("OUT_DIR", filepath.Join(cujDir, "out"))

	// Each of these tests is run in sequence without resetting the output tree.  The state of the output tree will
	// affect each successive test.  To maintain the validity of the benchmarks across changes, care must be taken
	// to avoid changing the state of the tree when a test is run.  This is most easily accomplished by adding tests
	// at the end.
	tests := []Test{
		{
			// Reset the out directory to get reproducible results.
			name: "clean",
			args: []string{"clean"},
		},
		{
			// Parse the build files.
			name: "nothing",
			args: []string{"nothing"},
		},
		{
			// Parse the build files again to monitor issues like globs rerunning.
			name: "nothing_rebuild",
			args: []string{"nothing"},
		},
		{
			// Parse the build files again, this should always be very short.
			name: "nothing_rebuild_twice",
			args: []string{"nothing"},
		},
		{
			// Build the framework as a common developer task and one that keeps getting longer.
			name: "framework",
			args: []string{"framework"},
		},
		{
			// Build the framework again to make sure it doesn't rebuild anything.
			name: "framework_rebuild",
			args: []string{"framework"},
		},
		{
			// Build the framework again to make sure it doesn't rebuild anything even if it did the second time.
			name: "framework_rebuild_twice",
			args: []string{"framework"},
		},
	}

	cujMetrics := metrics.NewCriticalUserJourneysMetrics()
	defer cujMetrics.Dump(filepath.Join(cujDir, "logs", "cuj_metrics.pb"))

	for i, t := range tests {
		logsSubDir := fmt.Sprintf("%02d_%s", i, t.name)
		logsDir := filepath.Join(cujDir, "logs", logsSubDir)
		t.Run(logsDir)
		if t.results.err != nil {
			fmt.Printf("error running test %q: %s\n", t.name, t.results.err)
			break
		}
		if t.results.metrics != nil {
			cujMetrics.Add(t.name, t.results.metrics)
		}
	}
}
