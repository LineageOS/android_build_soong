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
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"android/soong/ui/logger"
)

// SetupSignals sets up signal handling to kill our children and allow us to cleanly finish
// writing our log/trace files.
//
// Currently, on the first SIGINT|SIGALARM we call the cancel() function, which is usually
// the CancelFunc returned by context.WithCancel, which will kill all the commands running
// within that Context. Usually that's enough, and you'll run through your normal error paths.
//
// If another signal comes in after the first one, we'll trigger a panic with full stacktraces
// from every goroutine so that it's possible to debug what is stuck. Just before the process
// exits, we'll call the cleanup() function so that you can flush your log files.
func SetupSignals(log logger.Logger, cancel, cleanup func()) {
	signals := make(chan os.Signal, 5)
	// TODO: Handle other signals
	signal.Notify(signals, os.Interrupt, syscall.SIGALRM)
	go handleSignals(signals, log, cancel, cleanup)
}

func handleSignals(signals chan os.Signal, log logger.Logger, cancel, cleanup func()) {
	defer cleanup()

	var force bool

	for {
		s := <-signals
		if force {
			// So that we can better see what was stuck
			debug.SetTraceback("all")
			log.Panicln("Second signal received:", s)
		} else {
			log.Println("Got signal:", s)
			cancel()
			force = true
		}
	}
}
