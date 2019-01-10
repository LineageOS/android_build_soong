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
	"errors"
	"path/filepath"

	"android/soong/ui/metrics"
)

const gomaCtlScript = "goma_ctl.py"

var gomaCtlNotFound = errors.New("goma_ctl.py not found")

func startGoma(ctx Context, config Config) error {
	ctx.BeginTrace(metrics.RunSetupTool, "goma_ctl")
	defer ctx.EndTrace()

	var gomaCtl string
	if gomaDir, ok := config.Environment().Get("GOMA_DIR"); ok {
		gomaCtl = filepath.Join(gomaDir, gomaCtlScript)
	} else if home, ok := config.Environment().Get("HOME"); ok {
		gomaCtl = filepath.Join(home, "goma", gomaCtlScript)
	} else {
		return gomaCtlNotFound
	}

	cmd := Command(ctx, config, "goma_ctl.py ensure_start", gomaCtl, "ensure_start")

	if err := cmd.Run(); err != nil {
		ctx.Fatalf("goma_ctl.py ensure_start failed with: %v", err)
	}

	return nil
}
