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
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"android/soong/ui/logger"
)

func TestUploadMetrics(t *testing.T) {
	ctx := testContext()
	tests := []struct {
		description string
		uploader    string
		createFiles bool
		files       []string
	}{{
		description: "ANDROID_ENABLE_METRICS_UPLOAD not set",
	}, {
		description: "no metrics files to upload",
		uploader:    "fake",
	}, {
		description: "non-existent metrics files no upload",
		uploader:    "fake",
		files:       []string{"metrics_file_1", "metrics_file_2", "metrics_file_3"},
	}, {
		description: "trigger upload",
		uploader:    "echo",
		createFiles: true,
		files:       []string{"metrics_file_1", "metrics_file_2"},
	}}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			defer logger.Recover(func(err error) {
				t.Fatalf("got unexpected error: %v", err)
			})

			outDir, err := ioutil.TempDir("", "")
			if err != nil {
				t.Fatalf("failed to create out directory: %v", outDir)
			}
			defer os.RemoveAll(outDir)

			var metricsFiles []string
			if tt.createFiles {
				for _, f := range tt.files {
					filename := filepath.Join(outDir, f)
					metricsFiles = append(metricsFiles, filename)
					if err := ioutil.WriteFile(filename, []byte("test file"), 0644); err != nil {
						t.Fatalf("failed to create a fake metrics file %q for uploading: %v", filename, err)
					}
				}
			}

			config := Config{&configImpl{
				environ: &Environment{
					"OUT_DIR=" + outDir,
					"ANDROID_ENABLE_METRICS_UPLOAD=" + tt.uploader,
				},
				buildDateTime: strconv.FormatInt(time.Now().UnixNano()/int64(time.Millisecond), 10),
			}}

			UploadMetrics(ctx, config, 1591031903, metricsFiles...)

			if _, err := os.Stat(filepath.Join(outDir, uploadPbFilename)); err == nil {
				t.Error("got true, want false for upload protobuf file to exist")
			}
		})
	}
}

func TestUploadMetricsErrors(t *testing.T) {
	expectedErr := "failed to write the marshaled"
	defer logger.Recover(func(err error) {
		got := err.Error()
		if !strings.Contains(got, expectedErr) {
			t.Errorf("got %q, want %q to be contained in error", got, expectedErr)
		}
	})

	outDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("failed to create out directory: %v", outDir)
	}
	defer os.RemoveAll(outDir)

	metricsFile := filepath.Join(outDir, "metrics_file_1")
	if err := ioutil.WriteFile(metricsFile, []byte("test file"), 0644); err != nil {
		t.Fatalf("failed to create a fake metrics file %q for uploading: %v", metricsFile, err)
	}

	config := Config{&configImpl{
		environ: &Environment{
			"ANDROID_ENABLE_METRICS_UPLOAD=fake",
			"OUT_DIR=/bad",
		}}}

	UploadMetrics(testContext(), config, 1591031903, metricsFile)
	t.Errorf("got nil, expecting %q as a failure", expectedErr)
}
