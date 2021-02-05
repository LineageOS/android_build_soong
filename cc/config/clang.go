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
	"android/soong/android"
	"sort"
	"strings"
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
	"-Werror=unused-but-set-parameter",
	"-Werror=unused-but-set-variable",
	"-Wmaybe-uninitialized",
	"-Wno-error=clobbered",
	"-Wno-error=maybe-uninitialized",
	"-Wno-error=unused-but-set-parameter",
	"-Wno-error=unused-but-set-variable",
	"-Wno-extended-offsetof",
	"-Wno-free-nonheap-object",
	"-Wno-literal-suffix",
	"-Wno-maybe-uninitialized",
	"-Wno-old-style-declaration",
	"-Wno-unused-but-set-parameter",
	"-Wno-unused-but-set-variable",
	"-Wno-unused-local-typedefs",
	"-Wunused-but-set-parameter",
	"-Wunused-but-set-variable",
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

// Ldflags that should be filtered out when linking with clang lld
var ClangUnknownLldflags = sorted([]string{
	"-Wl,--fix-cortex-a8",
	"-Wl,--no-fix-cortex-a8",
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

func init() {
	pctx.StaticVariable("ClangExtraCflags", strings.Join([]string{
		"-D__compiler_offsetof=__builtin_offsetof",

		// Emit address-significance table which allows linker to perform safe ICF. Clang does
		// not emit the table by default on Android since NDK still uses GNU binutils.
		"-faddrsig",

		// Turn on -fcommon explicitly, since Clang now defaults to -fno-common. The cleanup bug
		// tracking this is http://b/151457797.
		"-fcommon",

		// Help catch common 32/64-bit errors.
		"-Werror=int-conversion",

		// Enable the new pass manager.
		"-fexperimental-new-pass-manager",

		// Disable overly aggressive warning for macros defined with a leading underscore
		// This happens in AndroidConfig.h, which is included nearly everywhere.
		// TODO: can we remove this now?
		"-Wno-reserved-id-macro",

		// Workaround for ccache with clang.
		// See http://petereisentraut.blogspot.com/2011/05/ccache-and-clang.html.
		"-Wno-unused-command-line-argument",

		// Force clang to always output color diagnostics. Ninja will strip the ANSI
		// color codes if it is not running in a terminal.
		"-fcolor-diagnostics",

		// Warnings from clang-7.0
		"-Wno-sign-compare",

		// Warnings from clang-8.0
		"-Wno-defaulted-function-deleted",

		// Disable -Winconsistent-missing-override until we can clean up the existing
		// codebase for it.
		"-Wno-inconsistent-missing-override",

		// Warnings from clang-10
		// Nested and array designated initialization is nice to have.
		"-Wno-c99-designator",

		// Warnings from clang-12
		"-Wno-gnu-folding-constant",

		// Calls to the APIs that are newer than the min sdk version of the caller should be
		// guarded with __builtin_available.
		"-Wunguarded-availability",
		// This macro allows the bionic versioning.h to indirectly determine whether the
		// option -Wunguarded-availability is on or not.
		"-D__ANDROID_UNGUARDED_AVAILABILITY__",
	}, " "))

	pctx.StaticVariable("ClangExtraCppflags", strings.Join([]string{
		// -Wimplicit-fallthrough is not enabled by -Wall.
		"-Wimplicit-fallthrough",

		// Enable clang's thread-safety annotations in libcxx.
		"-D_LIBCPP_ENABLE_THREAD_SAFETY_ANNOTATIONS",

		// libc++'s math.h has an #include_next outside of system_headers.
		"-Wno-gnu-include-next",
	}, " "))

	pctx.StaticVariable("ClangExtraTargetCflags", strings.Join([]string{
		"-nostdlibinc",
	}, " "))

	pctx.StaticVariable("ClangExtraNoOverrideCflags", strings.Join([]string{
		"-Werror=address-of-temporary",
		// Bug: http://b/29823425 Disable -Wnull-dereference until the
		// new cases detected by this warning in Clang r271374 are
		// fixed.
		//"-Werror=null-dereference",
		"-Werror=return-type",

		// http://b/72331526 Disable -Wtautological-* until the instances detected by these
		// new warnings are fixed.
		"-Wno-tautological-constant-compare",
		"-Wno-tautological-type-limit-compare",
		// http://b/145210666
		"-Wno-reorder-init-list",
		// http://b/145211066
		"-Wno-implicit-int-float-conversion",
		// New warnings to be fixed after clang-r377782.
		"-Wno-int-in-bool-context",          // http://b/148287349
		"-Wno-sizeof-array-div",             // http://b/148815709
		"-Wno-tautological-overlap-compare", // http://b/148815696
		// New warnings to be fixed after clang-r383902.
		"-Wno-deprecated-copy",                      // http://b/153746672
		"-Wno-range-loop-construct",                 // http://b/153747076
		"-Wno-misleading-indentation",               // http://b/153746954
		"-Wno-zero-as-null-pointer-constant",        // http://b/68236239
		"-Wno-deprecated-anon-enum-enum-conversion", // http://b/153746485
		"-Wno-deprecated-enum-enum-conversion",      // http://b/153746563
		"-Wno-string-compare",                       // http://b/153764102
		"-Wno-enum-enum-conversion",                 // http://b/154138986
		"-Wno-enum-float-conversion",                // http://b/154255917
		"-Wno-pessimizing-move",                     // http://b/154270751
		// New warnings to be fixed after clang-r399163
		"-Wno-non-c-typedef-for-linkage", // http://b/161304145
		// New warnings to be fixed after clang-r407598
		"-Wno-string-concatenation", // http://b/175068488
	}, " "))

	// Extra cflags for external third-party projects to disable warnings that
	// are infeasible to fix in all the external projects and their upstream repos.
	pctx.StaticVariable("ClangExtraExternalCflags", strings.Join([]string{
		"-Wno-enum-compare",
		"-Wno-enum-compare-switch",

		// http://b/72331524 Allow null pointer arithmetic until the instances detected by
		// this new warning are fixed.
		"-Wno-null-pointer-arithmetic",

		// Bug: http://b/29823425 Disable -Wnull-dereference until the
		// new instances detected by this warning are fixed.
		"-Wno-null-dereference",

		// http://b/145211477
		"-Wno-pointer-compare",
		// http://b/145211022
		"-Wno-xor-used-as-pow",
		// http://b/145211022
		"-Wno-final-dtor-non-final-class",

		// http://b/165945989
		"-Wno-psabi",
	}, " "))
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

func ClangFilterUnknownLldflags(lldflags []string) []string {
	result, _ := android.FilterList(lldflags, ClangUnknownLldflags)
	return result
}

func ClangLibToolingFilterUnknownCflags(libToolingFlags []string) []string {
	return android.RemoveListFromList(libToolingFlags, ClangLibToolingUnknownCflags)
}

func inListSorted(s string, list []string) bool {
	for _, l := range list {
		if s == l {
			return true
		} else if s < l {
			return false
		}
	}
	return false
}

func sorted(list []string) []string {
	sort.Strings(list)
	return list
}
