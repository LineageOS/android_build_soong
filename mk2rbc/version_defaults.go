// Copyright 2021 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mk2rbc

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"sort"
	"strconv"
	"strings"

	mkparser "android/soong/androidmk/parser"
)

const codenamePrefix = "PLATFORM_VERSION_CODENAME."

// ParseVersionDefaults extracts version settings from the given file
// and returns the map.
func ParseVersionDefaults(path string) (map[string]string, error) {
	contents, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	parser := mkparser.NewParser(path, bytes.NewBuffer(contents))
	nodes, errs := parser.Parse()
	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, "ERROR:", e)
		}
		return nil, fmt.Errorf("cannot parse %s", path)
	}

	result := map[string]string{
		"DEFAULT_PLATFORM_VERSION":            "",
		"MAX_PLATFORM_VERSION":                "",
		"MIN_PLATFORM_VERSION":                "A",
		"PLATFORM_BASE_SDK_EXTENSION_VERSION": "",
		"PLATFORM_SDK_EXTENSION_VERSION":      "",
		"PLATFORM_SDK_VERSION":                "",
		"PLATFORM_SECURITY_PATCH":             "",
		"PLATFORM_VERSION_LAST_STABLE":        "",
	}
	for _, node := range nodes {
		asgn, ok := node.(*mkparser.Assignment)
		if !(ok && asgn.Name.Const()) {
			continue
		}
		s := asgn.Name.Strings[0]
		_, ok = result[s]
		if !ok {
			ok = strings.HasPrefix(s, codenamePrefix)
		}
		if !ok {
			continue
		}
		v := asgn.Value
		if !v.Const() {
			return nil, fmt.Errorf("the value of %s should be constant", s)
		}
		result[s] = strings.TrimSpace(v.Strings[0])
	}
	return result, nil
}

func genericValue(s string) interface{} {
	if ival, err := strconv.ParseInt(s, 0, 0); err == nil {
		return ival
	}
	return s
}

// VersionDefaults generates the contents of the version_defaults.rbc file
func VersionDefaults(values map[string]string) string {
	var sink bytes.Buffer
	var lines []string
	var codenames []string
	for name, value := range values {
		if strings.HasPrefix(name, codenamePrefix) {
			codenames = append(codenames,
				fmt.Sprintf("%q: %q", strings.TrimPrefix(name, codenamePrefix), value))
		} else {
			// Print numbers as such
			lines = append(lines, fmt.Sprintf("    %s = %#v,\n",
				strings.ToLower(name), genericValue(value)))
		}
	}

	sort.Strings(lines)
	sort.Strings(codenames)

	sink.WriteString("version_defaults = struct(\n")
	for _, l := range lines {
		sink.WriteString(l)
	}
	sink.WriteString("    codenames = { ")
	sink.WriteString(strings.Join(codenames, ", "))
	sink.WriteString(" }\n)\n")
	return sink.String()
}
