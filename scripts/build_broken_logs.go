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

type Setting struct {
	name     string
	behavior BuildBrokenBehavior
	warnings []string
}

var buildBrokenSettings = []Setting{
	{
		name:     "BUILD_BROKEN_DUP_RULES",
		behavior: DefaultFalse,
		warnings: []string{"overriding commands for target"},
	},
	{
		name:     "BUILD_BROKEN_USES_NETWORK",
		behavior: DefaultDeprecated,
	},
	{
		name:     "BUILD_BROKEN_USES_BUILD_COPY_HEADERS",
		behavior: DefaultTrue,
		warnings: []string{
			"COPY_HEADERS has been deprecated",
			"COPY_HEADERS is deprecated",
		},
	},
}

type Branch struct {
	Settings []Setting
	Logs     []ProductLog
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
	WarningModuleTypes []string
	ErrorModuleTypes   []string

	BuildBroken map[string]*bool
	HasBroken   map[string]int
}

func Merge(l, l2 Log) Log {
	if l.BuildBroken == nil {
		l.BuildBroken = map[string]*bool{}
	}
	if l.HasBroken == nil {
		l.HasBroken = map[string]int{}
	}

	for n, v := range l.BuildBroken {
		if v == nil {
			l.BuildBroken[n] = l2.BuildBroken[n]
		}
	}
	for n, v := range l2.BuildBroken {
		if _, ok := l.BuildBroken[n]; !ok {
			l.BuildBroken[n] = v
		}
	}

	for n := range l.HasBroken {
		if l.HasBroken[n] < l2.HasBroken[n] {
			l.HasBroken[n] = l2.HasBroken[n]
		}
	}
	for n := range l2.HasBroken {
		if _, ok := l.HasBroken[n]; !ok {
			l.HasBroken[n] = l2.HasBroken[n]
		}
	}

	return l
}

func PrintResults(branch Branch) {
	products := branch.Logs
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

	for _, setting := range branch.Settings {
		printed := false
		n := setting.name

		for _, device := range deviceNames {
			log := devices[device]

			if setting.behavior == DefaultTrue {
				if log.BuildBroken[n] == nil || *log.BuildBroken[n] == false {
					if log.HasBroken[n] > 0 {
						printed = true
						plural := ""
						if log.HasBroken[n] > 1 {
							plural = "s"
						}
						fmt.Printf("  %s needs to set %s := true  (%d instance%s)\n", device, setting.name, log.HasBroken[n], plural)
					}
				} else if log.HasBroken[n] == 0 {
					printed = true
					fmt.Printf("  %s sets %s := true, but does not need it\n", device, setting.name)
				}
			} else if setting.behavior == DefaultFalse {
				if log.BuildBroken[n] == nil {
					// Nothing to be done
				} else if *log.BuildBroken[n] == false {
					printed = true
					fmt.Printf("  %s sets %s := false, which is the default and can be removed\n", device, setting.name)
				} else if log.HasBroken[n] == 0 {
					printed = true
					fmt.Printf("  %s sets %s := true, but does not need it\n", device, setting.name)
				}
			} else if setting.behavior == DefaultDeprecated {
				if log.BuildBroken[n] != nil {
					printed = true
					if log.HasBroken[n] > 0 {
						plural := ""
						if log.HasBroken[n] > 1 {
							plural = "s"
						}
						fmt.Printf("  %s sets %s := %v, which is deprecated, but has %d failure%s\n", device, setting.name, *log.BuildBroken[n], log.HasBroken[n], plural)
					} else {
						fmt.Printf("  %s sets %s := %v, which is deprecated and can be removed\n", device, setting.name, *log.BuildBroken[n])
					}
				}
			}
		}

		if printed {
			fmt.Println()
		}
	}
}

func ParseBranch(name string) Branch {
	products, err := filepath.Glob(filepath.Join(name, "*"))
	if err != nil {
		log.Fatal(err)
	}

	ret := Branch{Logs: []ProductLog{}}
	for _, product := range products {
		product = filepath.Base(product)

		ret.Logs = append(ret.Logs, ParseProduct(ProductBranch{Branch: name, Name: product}))
	}

	ret.Settings = append(ret.Settings, buildBrokenSettings...)
	if len(ret.Logs) > 0 {
		for _, mtype := range ret.Logs[0].WarningModuleTypes {
			if mtype == "BUILD_COPY_HEADERS" || mtype == "" {
				continue
			}
			ret.Settings = append(ret.Settings, Setting{
				name:     "BUILD_BROKEN_USES_" + mtype,
				behavior: DefaultTrue,
				warnings: []string{mtype + " has been deprecated"},
			})
		}
		for _, mtype := range ret.Logs[0].ErrorModuleTypes {
			if mtype == "BUILD_COPY_HEADERS" || mtype == "" {
				continue
			}
			ret.Settings = append(ret.Settings, Setting{
				name:     "BUILD_BROKEN_USES_" + mtype,
				behavior: DefaultFalse,
				warnings: []string{mtype + " has been deprecated"},
			})
		}
	}

	for _, productLog := range ret.Logs {
		ScanProduct(ret.Settings, productLog)
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
			BuildBroken: map[string]*bool{},
			HasBroken:   map[string]int{},
		},
	}

	lines := strings.Split(string(soongLog), "\n")
	for _, line := range lines {
		fields := strings.Split(line, " ")
		if len(fields) < 5 {
			continue
		}

		if fields[3] == "TARGET_DEVICE" {
			ret.Device = fields[4]
		}

		if fields[3] == "DEFAULT_WARNING_BUILD_MODULE_TYPES" {
			ret.WarningModuleTypes = fields[4:]
		}
		if fields[3] == "DEFAULT_ERROR_BUILD_MODULE_TYPES" {
			ret.ErrorModuleTypes = fields[4:]
		}

		if strings.HasPrefix(fields[3], "BUILD_BROKEN_") {
			ret.BuildBroken[fields[3]] = ParseBoolPtr(fields[4])
		}
	}

	return ret
}

func ScanProduct(settings []Setting, l ProductLog) {
	stdLog, err := ioutil.ReadFile(filepath.Join(l.Branch, l.Name, "std_full.log"))
	if err != nil {
		log.Fatal(err)
	}
	stdStr := string(stdLog)

	for _, setting := range settings {
		for _, warning := range setting.warnings {
			if strings.Contains(stdStr, warning) {
				l.HasBroken[setting.name] += strings.Count(stdStr, warning)
			}
		}
	}
}

func ParseBoolPtr(str string) *bool {
	var ret *bool
	if str != "" {
		b := str == "true"
		ret = &b
	}
	return ret
}
