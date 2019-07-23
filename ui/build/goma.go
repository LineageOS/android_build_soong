// Copyright 2018 Google Inc. All rights reserved.
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
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"strings"

	"android/soong/ui/metrics"
)

const gomaCtlScript = "goma_ctl.py"
const gomaLeastNProcs = 2500
const gomaLeastNFiles = 16000

// ulimit returns ulimit result for |opt|.
// if the resource is unlimited, it returns math.MaxInt32 so that a caller do
// not need special handling of the returned value.
//
// Note that since go syscall package do not have RLIMIT_NPROC constant,
// we use bash ulimit instead.
func ulimitOrFatal(ctx Context, config Config, opt string) int {
	commandText := fmt.Sprintf("ulimit %s", opt)
	cmd := Command(ctx, config, commandText, "bash", "-c", commandText)
	output := strings.TrimRight(string(cmd.CombinedOutputOrFatal()), "\n")
	ctx.Verbose(output + "\n")
	ctx.Verbose("done\n")

	if output == "unlimited" {
		return math.MaxInt32
	}
	num, err := strconv.Atoi(output)
	if err != nil {
		ctx.Fatalf("ulimit returned unexpected value: %s: %v\n", opt, err)
	}
	return num
}

func startGoma(ctx Context, config Config) {
	ctx.BeginTrace(metrics.RunSetupTool, "goma_ctl")
	defer ctx.EndTrace()

	if u := ulimitOrFatal(ctx, config, "-u"); u < gomaLeastNProcs {
		ctx.Fatalf("max user processes is insufficient: %d; want >= %d.\n", u, gomaLeastNProcs)
	}
	if n := ulimitOrFatal(ctx, config, "-n"); n < gomaLeastNFiles {
		ctx.Fatalf("max open files is insufficient: %d; want >= %d.\n", n, gomaLeastNFiles)
	}

	var gomaCtl string
	if gomaDir, ok := config.Environment().Get("GOMA_DIR"); ok {
		gomaCtl = filepath.Join(gomaDir, gomaCtlScript)
	} else if home, ok := config.Environment().Get("HOME"); ok {
		gomaCtl = filepath.Join(home, "goma", gomaCtlScript)
	} else {
		ctx.Fatalln("goma_ctl.py not found")
	}

	cmd := Command(ctx, config, "goma_ctl.py ensure_start", gomaCtl, "ensure_start")
	cmd.Environment.Set("DIST_DIR", config.DistDir())

	if output, err := cmd.CombinedOutput(); err != nil {
		ctx.Fatalf("goma_ctl.py ensure_start failed with: %v\n%s\n", err, output)
	}
}
