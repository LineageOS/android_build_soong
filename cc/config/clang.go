// Copyright 2017 Google Inc. All rights reserved.
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

package config

import (
	"sort"
	"strings"

	"android/soong/android"
)

// Cflags that should be filtered out when compiling with clang
var ClangUnknownCflags = sorted([]string{
	"-finline-functions",
	"-finline-limit=64",
	"-fno-canonical-system-headers",
	"-Wno-clobbered",
	"-fno-devirtualize",
	"-fno-tree-sra",
	"-fprefetch-loop-arrays",
	"-funswitch-loops",
	"-Wmaybe-uninitialized",
	"-Wno-error=clobbered",
	"-Wno-error=maybe-uninitialized",
	"-Wno-extended-offsetof",
	"-Wno-free-nonheap-object",
	"-Wno-literal-suffix",
	"-Wno-maybe-uninitialized",
	"-Wno-old-style-declaration",
	"-Wno-unused-local-typedefs",
	"-fdiagnostics-color",
	// http://b/153759688
	"-fuse-init-array",

	// arm + arm64
	"-fgcse-after-reload",
	"-frerun-cse-after-loop",
	"-frename-registers",
	"-fno-strict-volatile-bitfields",

	// arm + arm64
	"-fno-align-jumps",

	// arm
	"-mthumb-interwork",
	"-fno-builtin-sin",
	"-fno-caller-saves",
	"-fno-early-inlining",
	"-fno-move-loop-invariants",
	"-fno-partial-inlining",
	"-fno-tree-copy-prop",
	"-fno-tree-loop-optimize",

	// x86 + x86_64
	"-finline-limit=300",
	"-fno-inline-functions-called-once",
	"-mfpmath=sse",
	"-mbionic",

	// windows
	"--enable-stdcall-fixup",
})

var ClangLibToolingUnknownCflags = sorted([]string{})

// List of tidy checks that should be disabled globally. When the compiler is
// updated, some checks enabled by this module may be disabled if they have
// become more strict, or if they are a new match for a wildcard group like
// `modernize-*`.
var ClangTidyDisableChecks = []string{
	"misc-no-recursion",
	"readability-function-cognitive-complexity", // http://b/175055536
}

func ClangFilterUnknownCflags(cflags []string) []string {
	result, _ := android.FilterList(cflags, ClangUnknownCflags)
	return result
}

func clangTidyNegateChecks(checks []string) []string {
	ret := make([]string, 0, len(checks))
	for _, c := range checks {
		if strings.HasPrefix(c, "-") {
			ret = append(ret, c)
		} else {
			ret = append(ret, "-"+c)
		}
	}
	return ret
}

func ClangRewriteTidyChecks(checks []string) []string {
	checks = append(checks, clangTidyNegateChecks(ClangTidyDisableChecks)...)
	// clang-tidy does not allow later arguments to override earlier arguments,
	// so if we just disabled an argument that was explicitly enabled we must
	// remove the enabling argument from the list.
	result, _ := android.FilterList(checks, ClangTidyDisableChecks)
	return result
}

func ClangLibToolingFilterUnknownCflags(libToolingFlags []string) []string {
	return android.RemoveListFromList(libToolingFlags, ClangLibToolingUnknownCflags)
}

func sorted(list []string) []string {
	sort.Strings(list)
	return list
}
