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

// This file contains the functionality to upload data from one location to
// another.

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"android/soong/shared"
	"android/soong/ui/metrics"

	"google.golang.org/protobuf/proto"

	bazel_metrics_proto "android/soong/ui/metrics/bazel_metrics_proto"
	upload_proto "android/soong/ui/metrics/upload_proto"
)

const (
	// Used to generate a raw protobuf file that contains information
	// of the list of metrics files from host to destination storage.
	uploadPbFilename = ".uploader.pb"
)

var (
	// For testing purpose.
	tmpDir = ioutil.TempDir
)

// pruneMetricsFiles iterates the list of paths, checking if a path exist.
// If a path is a file, it is added to the return list. If the path is a
// directory, a recursive call is made to add the children files of the
// path.
func pruneMetricsFiles(paths []string) []string {
	var metricsFiles []string
	for _, p := range paths {
		fi, err := os.Stat(p)
		// Some paths passed may not exist. For example, build errors protobuf
		// file may not exist since the build was successful.
		if err != nil {
			continue
		}

		if fi.IsDir() {
			if l, err := ioutil.ReadDir(p); err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "Failed to find files under %s\n", p)
			} else {
				files := make([]string, 0, len(l))
				for _, fi := range l {
					files = append(files, filepath.Join(p, fi.Name()))
				}
				metricsFiles = append(metricsFiles, pruneMetricsFiles(files)...)
			}
		} else {
			metricsFiles = append(metricsFiles, p)
		}
	}
	return metricsFiles
}

func parseTimingToNanos(str string) int64 {
	millisString := removeDecimalPoint(str)
	timingMillis, _ := strconv.ParseInt(millisString, 10, 64)
	return timingMillis * 1000000
}

func parsePercentageToTenThousandths(str string) int32 {
	percentageString := removeDecimalPoint(str)
	//remove the % at the end of the string
	percentage := strings.ReplaceAll(percentageString, "%", "")
	percentagePortion, _ := strconv.ParseInt(percentage, 10, 32)
	return int32(percentagePortion)
}

func removeDecimalPoint(numString string) string {
	// The format is always 0.425 or 10.425
	return strings.ReplaceAll(numString, ".", "")
}

func parseTotal(line string) int64 {
	words := strings.Fields(line)
	timing := words[3]
	return parseTimingToNanos(timing)
}

func parsePhaseTiming(line string) bazel_metrics_proto.PhaseTiming {
	words := strings.Fields(line)
	getPhaseNameAndTimingAndPercentage := func([]string) (string, int64, int32) {
		// Sample lines include:
		// Total launch phase time   0.011 s    2.59%
		// Total target pattern evaluation phase time  0.011 s    2.59%
		var beginning int
		var end int
		for ind, word := range words {
			if word == "Total" {
				beginning = ind + 1
			} else if beginning > 0 && word == "phase" {
				end = ind
				break
			}
		}
		phaseName := strings.Join(words[beginning:end], " ")

		// end is now "phase" - advance by 2 for timing and 4 for percentage
		percentageString := words[end+4]
		timingString := words[end+2]
		timing := parseTimingToNanos(timingString)
		percentagePortion := parsePercentageToTenThousandths(percentageString)
		return phaseName, timing, percentagePortion
	}

	phaseName, timing, portion := getPhaseNameAndTimingAndPercentage(words)
	phaseTiming := bazel_metrics_proto.PhaseTiming{}
	phaseTiming.DurationNanos = &timing
	phaseTiming.PortionOfBuildTime = &portion

	phaseTiming.PhaseName = &phaseName
	return phaseTiming
}

func processBazelMetrics(bazelProfileFile string, bazelMetricsFile string, ctx Context) {
	if bazelProfileFile == "" {
		return
	}

	readBazelProto := func(filepath string) bazel_metrics_proto.BazelMetrics {
		//serialize the proto, write it
		bazelMetrics := bazel_metrics_proto.BazelMetrics{}

		file, err := os.ReadFile(filepath)
		if err != nil {
			ctx.Fatalln("Error reading metrics file\n", err)
		}

		scanner := bufio.NewScanner(strings.NewReader(string(file)))
		scanner.Split(bufio.ScanLines)

		var phaseTimings []*bazel_metrics_proto.PhaseTiming
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "Total run time") {
				total := parseTotal(line)
				bazelMetrics.Total = &total
			} else if strings.HasPrefix(line, "Total") {
				phaseTiming := parsePhaseTiming(line)
				phaseTimings = append(phaseTimings, &phaseTiming)
			}
		}
		bazelMetrics.PhaseTimings = phaseTimings

		return bazelMetrics
	}

	if _, err := os.Stat(bazelProfileFile); err != nil {
		// We can assume bazel didn't run if the profile doesn't exist
		return
	}
	bazelProto := readBazelProto(bazelProfileFile)
	shared.Save(&bazelProto, bazelMetricsFile)
}

// UploadMetrics uploads a set of metrics files to a server for analysis.
// The metrics files are first copied to a temporary directory
// and the uploader is then executed in the background to allow the user/system
// to continue working. Soong communicates to the uploader through the
// upload_proto raw protobuf file.
func UploadMetrics(ctx Context, config Config, simpleOutput bool, buildStarted time.Time, bazelProfileFile string, bazelMetricsFile string, paths ...string) {
	ctx.BeginTrace(metrics.RunSetupTool, "upload_metrics")
	defer ctx.EndTrace()

	uploader := config.MetricsUploaderApp()
	if uploader == "" {
		// If the uploader path was not specified, no metrics shall be uploaded.
		return
	}

	processBazelMetrics(bazelProfileFile, bazelMetricsFile, ctx)
	// Several of the files might be directories.
	metricsFiles := pruneMetricsFiles(paths)
	if len(metricsFiles) == 0 {
		return
	}

	// The temporary directory cannot be deleted as the metrics uploader is started
	// in the background and requires to exist until the operation is done. The
	// uploader can delete the directory as it is specified in the upload proto.
	tmpDir, err := tmpDir("", "upload_metrics")
	if err != nil {
		ctx.Fatalf("failed to create a temporary directory to store the list of metrics files: %v\n", err)
	}

	for i, src := range metricsFiles {
		dst := filepath.Join(tmpDir, filepath.Base(src))
		if _, err := copyFile(src, dst); err != nil {
			ctx.Fatalf("failed to copy %q to %q: %v\n", src, dst, err)
		}
		metricsFiles[i] = dst
	}

	// For platform builds, the branch and target name is hardcoded to specific
	// values for later extraction of the metrics in the data metrics pipeline.
	data, err := proto.Marshal(&upload_proto.Upload{
		CreationTimestampMs:   proto.Uint64(uint64(buildStarted.UnixNano() / int64(time.Millisecond))),
		CompletionTimestampMs: proto.Uint64(uint64(time.Now().UnixNano() / int64(time.Millisecond))),
		BranchName:            proto.String("developer-metrics"),
		TargetName:            proto.String("platform-build-systems-metrics"),
		MetricsFiles:          metricsFiles,
		DirectoriesToDelete:   []string{tmpDir},
	})
	if err != nil {
		ctx.Fatalf("failed to marshal metrics upload proto buffer message: %v\n", err)
	}

	pbFile := filepath.Join(tmpDir, uploadPbFilename)
	if err := ioutil.WriteFile(pbFile, data, 0644); err != nil {
		ctx.Fatalf("failed to write the marshaled metrics upload protobuf to %q: %v\n", pbFile, err)
	}

	// Start the uploader in the background as it takes several milliseconds to start the uploader
	// and prepare the metrics for upload. This affects small shell commands like "lunch".
	cmd := Command(ctx, config, "upload metrics", uploader, "--upload-metrics", pbFile)
	if simpleOutput {
		cmd.RunOrFatal()
	} else {
		cmd.RunAndStreamOrFatal()
	}
}
