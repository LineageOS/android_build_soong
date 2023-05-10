// Copyright 2023 Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package build

// This file contains functionality to parse bazel profile data into
// a bazel_metrics proto, defined in build/soong/ui/metrics/bazel_metrics_proto
// These metrics are later uploaded in upload.go

import (
	"bufio"
	"os"
	"strconv"
	"strings"

	"android/soong/shared"
	"google.golang.org/protobuf/proto"

	bazel_metrics_proto "android/soong/ui/metrics/bazel_metrics_proto"
)

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

// This method takes a file created by bazel's --analyze-profile mode and
// writes bazel metrics data to the provided filepath.
func ProcessBazelMetrics(bazelProfileFile string, bazelMetricsFile string, ctx Context, config Config) {
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
	bazelProto.ExitCode = proto.Int32(config.bazelExitCode)
	shared.Save(&bazelProto, bazelMetricsFile)
}
