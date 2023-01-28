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
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"android/soong/ui/logger"
)

func writeBazelProfileFile(dir string) error {
	contents := `

=== PHASE SUMMARY INFORMATION ===

Total launch phase time                              1.193 s   15.77%
Total init phase time                                1.092 s   14.44%
Total target pattern evaluation phase time           0.580 s    7.67%
Total interleaved loading-and-analysis phase time    3.646 s   48.21%
Total preparation phase time                         0.022 s    0.30%
Total execution phase time                           0.993 s   13.13%
Total finish phase time                              0.036 s    0.48%
---------------------------------------------------------------------
Total run time                                       7.563 s  100.00%

Critical path (178 ms):
       Time Percentage   Description
     178 ms  100.00%   action 'BazelWorkspaceStatusAction stable-status.txt'

`
	file := filepath.Join(dir, "bazel_metrics.txt")
	return os.WriteFile(file, []byte(contents), 0666)
}

func TestPruneMetricsFiles(t *testing.T) {
	rootDir := t.TempDir()

	dirs := []string{
		filepath.Join(rootDir, "d1"),
		filepath.Join(rootDir, "d1", "d2"),
		filepath.Join(rootDir, "d1", "d2", "d3"),
	}

	files := []string{
		filepath.Join(rootDir, "d1", "f1"),
		filepath.Join(rootDir, "d1", "d2", "f1"),
		filepath.Join(rootDir, "d1", "d2", "d3", "f1"),
	}

	for _, d := range dirs {
		if err := os.MkdirAll(d, 0777); err != nil {
			t.Fatalf("got %v, expecting nil error for making directory %q", err, d)
		}
	}

	for _, f := range files {
		if err := ioutil.WriteFile(f, []byte{}, 0777); err != nil {
			t.Fatalf("got %v, expecting nil error on writing file %q", err, f)
		}
	}

	want := []string{
		filepath.Join(rootDir, "d1", "f1"),
		filepath.Join(rootDir, "d1", "d2", "f1"),
		filepath.Join(rootDir, "d1", "d2", "d3", "f1"),
	}

	got := pruneMetricsFiles([]string{rootDir})

	sort.Strings(got)
	sort.Strings(want)

	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q after pruning metrics files", got, want)
	}
}

func TestUploadMetrics(t *testing.T) {
	ctx := testContext()
	tests := []struct {
		description string
		uploader    string
		createFiles bool
		files       []string
	}{{
		description: "no metrics uploader",
	}, {
		description: "non-existent metrics files no upload",
		uploader:    "echo",
		files:       []string{"metrics_file_1", "metrics_file_2", "metrics_file_3, bazel_metrics.pb"},
	}, {
		description: "trigger upload",
		uploader:    "echo",
		createFiles: true,
		files:       []string{"metrics_file_1", "metrics_file_2, bazel_metrics.pb"},
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

			// Supply our own tmpDir to delete the temp dir once the test is done.
			orgTmpDir := tmpDir
			tmpDir = func(string, string) (string, error) {
				retDir := filepath.Join(outDir, "tmp_upload_dir")
				if err := os.Mkdir(retDir, 0755); err != nil {
					t.Fatalf("failed to create temporary directory %q: %v", retDir, err)
				}
				return retDir, nil
			}
			defer func() { tmpDir = orgTmpDir }()

			metricsUploadDir := filepath.Join(outDir, ".metrics_uploader")
			if err := os.Mkdir(metricsUploadDir, 0755); err != nil {
				t.Fatalf("failed to create %q directory for oauth valid check: %v", metricsUploadDir, err)
			}

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
			if err := writeBazelProfileFile(outDir); err != nil {
				t.Fatalf("failed to create bazel profile file in dir: %v", outDir)
			}

			config := Config{&configImpl{
				environ: &Environment{
					"OUT_DIR=" + outDir,
				},
				buildDateTime:   strconv.FormatInt(time.Now().UnixNano()/int64(time.Millisecond), 10),
				metricsUploader: tt.uploader,
			}}

			UploadMetrics(ctx, config, false, time.Now(), "out/bazel_metrics.txt", "out/bazel_metrics.pb", metricsFiles...)
		})
	}
}

func TestUploadMetricsErrors(t *testing.T) {
	ctx := testContext()
	tests := []struct {
		description string
		tmpDir      string
		tmpDirErr   error
		expectedErr string
	}{{
		description: "getTmpDir returned error",
		tmpDirErr:   errors.New("getTmpDir failed"),
		expectedErr: "getTmpDir failed",
	}, {
		description: "copyFile operation error",
		tmpDir:      "/fake_dir",
		expectedErr: "failed to copy",
	}}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			defer logger.Recover(func(err error) {
				got := err.Error()
				if !strings.Contains(got, tt.expectedErr) {
					t.Errorf("got %q, want %q to be contained in error", got, tt.expectedErr)
				}
			})

			outDir, err := ioutil.TempDir("", "")
			if err != nil {
				t.Fatalf("failed to create out directory: %v", outDir)
			}
			defer os.RemoveAll(outDir)

			orgTmpDir := tmpDir
			tmpDir = func(string, string) (string, error) {
				return tt.tmpDir, tt.tmpDirErr
			}
			defer func() { tmpDir = orgTmpDir }()

			metricsFile := filepath.Join(outDir, "metrics_file_1")
			if err := ioutil.WriteFile(metricsFile, []byte("test file"), 0644); err != nil {
				t.Fatalf("failed to create a fake metrics file %q for uploading: %v", metricsFile, err)
			}

			config := Config{&configImpl{
				environ: &Environment{
					"OUT_DIR=/bad",
				},
				metricsUploader: "echo",
			}}

			UploadMetrics(ctx, config, true, time.Now(), "", "", metricsFile)
			t.Errorf("got nil, expecting %q as a failure", tt.expectedErr)
		})
	}
}

func TestParsePercentageToTenThousandths(t *testing.T) {
	// 2.59% should be returned as 259 - representing 259/10000 of the build
	percentage := parsePercentageToTenThousandths("2.59%")
	if percentage != 259 {
		t.Errorf("Expected percentage to be returned as ten-thousandths. Expected 259, have %d\n", percentage)
	}

	// Test without a leading digit
	percentage = parsePercentageToTenThousandths(".52%")
	if percentage != 52 {
		t.Errorf("Expected percentage to be returned as ten-thousandths. Expected 52, have %d\n", percentage)
	}
}

func TestParseTimingToNanos(t *testing.T) {
	// This parses from seconds (with millis precision) and returns nanos
	timingNanos := parseTimingToNanos("0.111")
	if timingNanos != 111000000 {
		t.Errorf("Error parsing timing. Expected 111000, have %d\n", timingNanos)
	}

	// Test without a leading digit
	timingNanos = parseTimingToNanos(".112")
	if timingNanos != 112000000 {
		t.Errorf("Error parsing timing. Expected 112000, have %d\n", timingNanos)
	}
}

func TestParsePhaseTiming(t *testing.T) {
	// Sample lines include:
	// Total launch phase time   0.011 s    2.59%
	// Total target pattern evaluation phase time  0.012 s    4.59%

	line1 := "Total launch phase time   0.011 s    2.59%"
	timing := parsePhaseTiming(line1)

	if timing.GetPhaseName() != "launch" {
		t.Errorf("Failed to parse phase name. Expected launch, have %s\n", timing.GetPhaseName())
	} else if timing.GetDurationNanos() != 11000000 {
		t.Errorf("Failed to parse duration nanos. Expected 11000000, have %d\n", timing.GetDurationNanos())
	} else if timing.GetPortionOfBuildTime() != 259 {
		t.Errorf("Failed to parse portion of build time. Expected 259, have %d\n", timing.GetPortionOfBuildTime())
	}

	// Test with a multiword phase name
	line2 := "Total target pattern evaluation phase  time  0.012 s    4.59%"

	timing = parsePhaseTiming(line2)
	if timing.GetPhaseName() != "target pattern evaluation" {
		t.Errorf("Failed to parse phase name. Expected target pattern evaluation, have %s\n", timing.GetPhaseName())
	} else if timing.GetDurationNanos() != 12000000 {
		t.Errorf("Failed to parse duration nanos. Expected 12000000, have %d\n", timing.GetDurationNanos())
	} else if timing.GetPortionOfBuildTime() != 459 {
		t.Errorf("Failed to parse portion of build time. Expected 459, have %d\n", timing.GetPortionOfBuildTime())
	}
}

func TestParseTotal(t *testing.T) {
	// Total line is in the form of:
	// Total run time                                       7.563 s  100.00%

	line := "Total run time                                       7.563 s  100.00%"

	total := parseTotal(line)

	// Only the seconds field is parsed, as nanos
	if total != 7563000000 {
		t.Errorf("Failed to parse total build time. Expected 7563000000, have %d\n", total)
	}
}
