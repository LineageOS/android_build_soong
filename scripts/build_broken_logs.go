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

// This is a script that can be used to analyze the results from
// build/soong/build_test.bash and recommend what devices need changes to their
// BUILD_BROKEN_* flags.
//
// To use, download the logs.zip from one or more branches, and extract them
// into subdirectories of the current directory. So for example, I have:
//
//   ./aosp-master/aosp_arm/std_full.log
//   ./aosp-master/aosp_arm64/std_full.log
//   ./aosp-master/...
//   ./internal-master/aosp_arm/std_full.log
//   ./internal-master/aosp_arm64/std_full.log
//   ./internal-master/...
//
// Then I use `go run path/to/build_broken_logs.go *`
package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func main() {
	for _, branch := range os.Args[1:] {
		fmt.Printf("\nBranch %s:\n", branch)
		PrintResults(ParseBranch(branch))
	}
}

type BuildBrokenBehavior int

const (
	DefaultFalse BuildBrokenBehavior = iota
	DefaultTrue
	DefaultDeprecated
)

var buildBrokenSettings = []struct {
	name     string
	behavior BuildBrokenBehavior
	warnings []string
}{
	{
		name:     "BUILD_BROKEN_DUP_RULES",
		behavior: DefaultFalse,
		warnings: []string{"overriding commands for target"},
	},
	{
		name:     "BUILD_BROKEN_USES_NETWORK",
		behavior: DefaultDeprecated,
	},
}

type ProductBranch struct {
	Branch string
	Name   string
}

type ProductLog struct {
	ProductBranch
	Log
	Device string
}

type Log struct {
	BuildBroken []*bool
	HasBroken   []bool
}

func Merge(l, l2 Log) Log {
	if len(l.BuildBroken) == 0 {
		l.BuildBroken = make([]*bool, len(buildBrokenSettings))
	}
	if len(l.HasBroken) == 0 {
		l.HasBroken = make([]bool, len(buildBrokenSettings))
	}

	if len(l.BuildBroken) != len(l2.BuildBroken) || len(l.HasBroken) != len(l2.HasBroken) {
		panic("mis-matched logs")
	}

	for i, v := range l.BuildBroken {
		if v == nil {
			l.BuildBroken[i] = l2.BuildBroken[i]
		}
	}
	for i := range l.HasBroken {
		l.HasBroken[i] = l.HasBroken[i] || l2.HasBroken[i]
	}

	return l
}

func PrintResults(products []ProductLog) {
	devices := map[string]Log{}
	deviceNames := []string{}

	for _, product := range products {
		device := product.Device
		if _, ok := devices[device]; !ok {
			deviceNames = append(deviceNames, device)
		}
		devices[device] = Merge(devices[device], product.Log)
	}

	sort.Strings(deviceNames)

	for i, setting := range buildBrokenSettings {
		printed := false

		for _, device := range deviceNames {
			log := devices[device]

			if setting.behavior == DefaultTrue {
				if log.BuildBroken[i] == nil || *log.BuildBroken[i] == false {
					if log.HasBroken[i] {
						printed = true
						fmt.Printf("  %s needs to set %s := true\n", device, setting.name)
					}
				} else if !log.HasBroken[i] {
					printed = true
					fmt.Printf("  %s sets %s := true, but does not need it\n", device, setting.name)
				}
			} else if setting.behavior == DefaultFalse {
				if log.BuildBroken[i] == nil {
					// Nothing to be done
				} else if *log.BuildBroken[i] == false {
					printed = true
					fmt.Printf("  %s sets %s := false, which is the default and can be removed\n", device, setting.name)
				} else if !log.HasBroken[i] {
					printed = true
					fmt.Printf("  %s sets %s := true, but does not need it\n", device, setting.name)
				}
			} else if setting.behavior == DefaultDeprecated {
				if log.BuildBroken[i] != nil {
					printed = true
					if log.HasBroken[i] {
						fmt.Printf("  %s sets %s := %v, which is deprecated, but has failures\n", device, setting.name, *log.BuildBroken[i])
					} else {
						fmt.Printf("  %s sets %s := %v, which is deprecated and can be removed\n", device, setting.name, *log.BuildBroken[i])
					}
				}
			}
		}

		if printed {
			fmt.Println()
		}
	}
}

func ParseBranch(name string) []ProductLog {
	products, err := filepath.Glob(filepath.Join(name, "*"))
	if err != nil {
		log.Fatal(err)
	}

	ret := []ProductLog{}
	for _, product := range products {
		product = filepath.Base(product)

		ret = append(ret, ParseProduct(ProductBranch{Branch: name, Name: product}))
	}
	return ret
}

func ParseProduct(p ProductBranch) ProductLog {
	soongLog, err := ioutil.ReadFile(filepath.Join(p.Branch, p.Name, "soong.log"))
	if err != nil {
		log.Fatal(err)
	}

	ret := ProductLog{
		ProductBranch: p,
		Log: Log{
			BuildBroken: make([]*bool, len(buildBrokenSettings)),
			HasBroken:   make([]bool, len(buildBrokenSettings)),
		},
	}

	lines := strings.Split(string(soongLog), "\n")
	for _, line := range lines {
		fields := strings.Split(line, " ")
		if len(fields) != 5 {
			continue
		}

		if fields[3] == "TARGET_DEVICE" {
			ret.Device = fields[4]
		}

		if strings.HasPrefix(fields[3], "BUILD_BROKEN_") {
			for i, setting := range buildBrokenSettings {
				if setting.name == fields[3] {
					ret.BuildBroken[i] = ParseBoolPtr(fields[4])
				}
			}
		}
	}

	stdLog, err := ioutil.ReadFile(filepath.Join(p.Branch, p.Name, "std_full.log"))
	if err != nil {
		log.Fatal(err)
	}
	stdStr := string(stdLog)

	for i, setting := range buildBrokenSettings {
		for _, warning := range setting.warnings {
			if strings.Contains(stdStr, warning) {
				ret.HasBroken[i] = true
			}
		}
	}

	return ret
}

func ParseBoolPtr(str string) *bool {
	var ret *bool
	if str != "" {
		b := str == "true"
		ret = &b
	}
	return ret
}
