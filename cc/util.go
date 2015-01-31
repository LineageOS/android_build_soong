// Copyright 2015 Google Inc. All rights reserved.
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

package cc

import (
	"fmt"
	"regexp"
	"strings"
)

// Efficiently converts a list of include directories to a single string
// of cflags with -I prepended to each directory.
func includeDirsToFlags(dirs []string) string {
	return joinWithPrefix(dirs, "-I")
}

func ldDirsToFlags(dirs []string) string {
	return joinWithPrefix(dirs, "-L")
}

func libNamesToFlags(names []string) string {
	return joinWithPrefix(names, "-l")
}

func joinWithPrefix(strs []string, prefix string) string {
	if len(strs) == 0 {
		return ""
	}

	if len(strs) == 1 {
		return prefix + strs[0]
	}

	n := len(" ") * (len(strs) - 1)
	for _, s := range strs {
		n += len(prefix) + len(s)
	}

	ret := make([]byte, 0, n)
	for i, s := range strs {
		if i != 0 {
			ret = append(ret, ' ')
		}
		ret = append(ret, prefix...)
		ret = append(ret, s...)
	}
	return string(ret)
}

func inList(s string, list []string) bool {
	for _, l := range list {
		if l == s {
			return true
		}
	}

	return false
}

var libNameRegexp = regexp.MustCompile(`^lib(.*)$`)

func moduleToLibName(module string) (string, error) {
	matches := libNameRegexp.FindStringSubmatch(module)
	if matches == nil {
		return "", fmt.Errorf("Library module name %s does not start with lib", module)
	}
	return matches[1], nil
}

func ccFlagsToBuilderFlags(in ccFlags) builderFlags {
	return builderFlags{
		globalFlags: strings.Join(in.globalFlags, " "),
		asFlags:     strings.Join(in.asFlags, " "),
		cFlags:      strings.Join(in.cFlags, " "),
		conlyFlags:  strings.Join(in.conlyFlags, " "),
		cppFlags:    strings.Join(in.cppFlags, " "),
		ldFlags:     strings.Join(in.ldFlags, " "),
		ldLibs:      strings.Join(in.ldLibs, " "),
		incFlags:    includeDirsToFlags(in.includeDirs),
		nocrt:       in.nocrt,
		toolchain:   in.toolchain,
		clang:       in.clang,
	}
}
