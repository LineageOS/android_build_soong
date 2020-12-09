// Copyright 2020 Google Inc. All rights reserved.
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
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"android/soong/ui/logger"
)

func TestDumpRBEMetrics(t *testing.T) {
	ctx := testContext()
	tests := []struct {
		description string
		env         []string
		generated   bool
	}{{
		description: "RBE disabled",
		env: []string{
			"NOSTART_RBE=true",
		},
	}, {
		description: "rbe metrics generated",
		env: []string{
			"USE_RBE=true",
		},
		generated: true,
	}}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			tmpDir, err := ioutil.TempDir("", "")
			if err != nil {
				t.Fatalf("failed to create a temp directory: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			rbeBootstrapCmd := filepath.Join(tmpDir, bootstrapCmd)
			if err := ioutil.WriteFile(rbeBootstrapCmd, []byte(rbeBootstrapProgram), 0755); err != nil {
				t.Fatalf("failed to create a fake bootstrap command file %s: %v", rbeBootstrapCmd, err)
			}

			env := Environment(tt.env)
			env.Set("OUT_DIR", tmpDir)
			env.Set("RBE_DIR", tmpDir)

			tmpRBEDir, err := ioutil.TempDir("", "")
			if err != nil {
				t.Fatalf("failed to create a temp directory for RBE: %v", err)
			}
			defer os.RemoveAll(tmpRBEDir)
			env.Set("RBE_output_dir", tmpRBEDir)

			config := Config{&configImpl{
				environ: &env,
			}}

			rbeMetricsFilename := filepath.Join(tmpDir, rbeMetricsPBFilename)
			DumpRBEMetrics(ctx, config, rbeMetricsFilename)

			// Validate that the rbe metrics file exists if RBE is enabled.
			if _, err := os.Stat(rbeMetricsFilename); err == nil {
				if !tt.generated {
					t.Errorf("got true, want false for rbe metrics file %s to exist.", rbeMetricsFilename)
				}
			} else if os.IsNotExist(err) {
				if tt.generated {
					t.Errorf("got false, want true for rbe metrics file %s to exist.", rbeMetricsFilename)
				}
			} else {
				t.Errorf("unknown error found on checking %s exists: %v", rbeMetricsFilename, err)
			}
		})
	}
}

func TestDumpRBEMetricsErrors(t *testing.T) {
	ctx := testContext()
	tests := []struct {
		description      string
		bootstrapProgram string
		expectedErr      string
	}{{
		description:      "stopRBE failed",
		bootstrapProgram: "#!/bin/bash\nexit 1\n",
		expectedErr:      "shutdown failed",
	}}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			defer logger.Recover(func(err error) {
				got := err.Error()
				if !strings.Contains(got, tt.expectedErr) {
					t.Errorf("got %q, want %q to be contained in error", got, tt.expectedErr)
				}
			})

			tmpDir, err := ioutil.TempDir("", "")
			if err != nil {
				t.Fatalf("failed to create a temp directory: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			rbeBootstrapCmd := filepath.Join(tmpDir, bootstrapCmd)
			if err := ioutil.WriteFile(rbeBootstrapCmd, []byte(tt.bootstrapProgram), 0755); err != nil {
				t.Fatalf("failed to create a fake bootstrap command file %s: %v", rbeBootstrapCmd, err)
			}

			env := &Environment{}
			env.Set("USE_RBE", "true")
			env.Set("OUT_DIR", tmpDir)
			env.Set("RBE_DIR", tmpDir)

			config := Config{&configImpl{
				environ: env,
			}}

			rbeMetricsFilename := filepath.Join(tmpDir, rbeMetricsPBFilename)
			DumpRBEMetrics(ctx, config, rbeMetricsFilename)
			t.Errorf("got nil, expecting %q as a failure", tt.expectedErr)
		})
	}
}

var rbeBootstrapProgram = fmt.Sprintf("#!/bin/bash\necho 1 > $RBE_output_dir/%s\n", rbeMetricsPBFilename)
