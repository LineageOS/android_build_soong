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

package build

import (
	"path/filepath"

	"android/soong/ui/metrics"
)

const bootstrapCmd = "bootstrap"
const rbeLeastNProcs = 2500
const rbeLeastNFiles = 16000

func startRBE(ctx Context, config Config) {
	ctx.BeginTrace(metrics.RunSetupTool, "rbe_bootstrap")
	defer ctx.EndTrace()

	if u := ulimitOrFatal(ctx, config, "-u"); u < rbeLeastNProcs {
		ctx.Fatalf("max user processes is insufficient: %d; want >= %d.\n", u, rbeLeastNProcs)
	}
	if n := ulimitOrFatal(ctx, config, "-n"); n < rbeLeastNFiles {
		ctx.Fatalf("max open files is insufficient: %d; want >= %d.\n", n, rbeLeastNFiles)
	}

	var rbeBootstrap string
	if rbeDir, ok := config.Environment().Get("RBE_DIR"); ok {
		rbeBootstrap = filepath.Join(rbeDir, bootstrapCmd)
	} else if home, ok := config.Environment().Get("HOME"); ok {
		rbeBootstrap = filepath.Join(home, "rbe", bootstrapCmd)
	} else {
		ctx.Fatalln("rbe bootstrap not found")
	}

	cmd := Command(ctx, config, "boostrap", rbeBootstrap)

	if output, err := cmd.CombinedOutput(); err != nil {
		ctx.Fatalf("rbe bootstrap failed with: %v\n%s\n", err, output)
	}
}
