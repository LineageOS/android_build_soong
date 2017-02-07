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

package main

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"android/soong/ui/build"
	"android/soong/ui/logger"
)

func indexList(s string, list []string) int {
	for i, l := range list {
		if l == s {
			return i
		}
	}

	return -1
}

func inList(s string, list []string) bool {
	return indexList(s, list) != -1
}

func main() {
	log := logger.New(os.Stderr)
	defer log.Cleanup()

	if len(os.Args) < 2 || !inList("--make-mode", os.Args) {
		log.Fatalln("The `soong` native UI is not yet available.")
	}

	// Precondition: the current directory is the top of the source tree
	if _, err := os.Stat("build/soong/root.bp"); err != nil {
		if os.IsNotExist(err) {
			log.Fatalln("soong_ui should run from the root of the source directory: build/soong/root.bp not found")
		}
		log.Fatalln("Error verifying tree state:", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	build.SetupSignals(log, cancel, log.Cleanup)

	buildCtx := &build.ContextImpl{
		Context:        ctx,
		Logger:         log,
		StdioInterface: build.StdioImpl{},
	}
	config := build.NewConfig(buildCtx, os.Args[1:]...)

	log.SetVerbose(config.IsVerbose())
	if err := os.MkdirAll(config.OutDir(), 0777); err != nil {
		log.Fatalf("Error creating out directory: %v", err)
	}
	log.SetOutput(filepath.Join(config.OutDir(), "build.log"))

	if start, ok := os.LookupEnv("TRACE_BEGIN_SOONG"); ok {
		if !strings.HasSuffix(start, "N") {
			if start_time, err := strconv.ParseUint(start, 10, 64); err == nil {
				log.Verbosef("Took %dms to start up.",
					time.Since(time.Unix(0, int64(start_time))).Nanoseconds()/time.Millisecond.Nanoseconds())
			}
		}
	}

	build.Build(buildCtx, config, build.BuildAll)
}
