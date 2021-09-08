// Copyright 2016 Google Inc. All rights reserved.
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
	"strings"
)

func init() {
	// Many clang-tidy checks like altera-*, llvm-*, modernize-*
	// are not designed for Android source code or creating too
	// many (false-positive) warnings. The global default tidy checks
	// should include only tested groups and exclude known noisy checks.
	// See https://clang.llvm.org/extra/clang-tidy/checks/list.html
	pctx.VariableFunc("TidyDefaultGlobalChecks", func(ctx android.PackageVarContext) string {
		if override := ctx.Config().Getenv("DEFAULT_GLOBAL_TIDY_CHECKS"); override != "" {
			return override
		}
		checks := strings.Join([]string{
			"-*",
			"android-*",
			"bugprone-*",
			"cert-*",
			"clang-diagnostic-unused-command-line-argument",
			"google-*",
			"misc-*",
			"performance-*",
			"portability-*",
			"-bugprone-narrowing-conversions",
			"-google-readability*",
			"-google-runtime-references",
			"-misc-no-recursion",
			"-misc-non-private-member-variables-in-classes",
			"-misc-unused-parameters",
			// the following groups are excluded by -*
			// -altera-*
			// -cppcoreguidelines-*
			// -darwin-*
			// -fuchsia-*
			// -hicpp-*
			// -llvm-*
			// -llvmlibc-*
			// -modernize-*
			// -mpi-*
			// -objc-*
			// -readability-*
			// -zircon-*
		}, ",")
		// clang-analyzer-* checks are too slow to be in the default for WITH_TIDY=1.
		// nightly builds add CLANG_ANALYZER_CHECKS=1 to run those checks.
		if ctx.Config().IsEnvTrue("CLANG_ANALYZER_CHECKS") {
			checks += ",clang-analyzer-*"
		}
		return checks
	})

	// There are too many clang-tidy warnings in external and vendor projects.
	// Enable only some google checks for these projects.
	pctx.VariableFunc("TidyExternalVendorChecks", func(ctx android.PackageVarContext) string {
		if override := ctx.Config().Getenv("DEFAULT_EXTERNAL_VENDOR_TIDY_CHECKS"); override != "" {
			return override
		}
		return strings.Join([]string{
			"-*",
			"clang-diagnostic-unused-command-line-argument",
			"google*",
			"-google-build-using-namespace",
			"-google-default-arguments",
			"-google-explicit-constructor",
			"-google-readability*",
			"-google-runtime-int",
			"-google-runtime-references",
		}, ",")
	})

	// To reduce duplicate warnings from the same header files,
	// header-filter will contain only the module directory and
	// those specified by DEFAULT_TIDY_HEADER_DIRS.
	pctx.VariableFunc("TidyDefaultHeaderDirs", func(ctx android.PackageVarContext) string {
		return ctx.Config().Getenv("DEFAULT_TIDY_HEADER_DIRS")
	})

	// Use WTIH_TIDY_FLAGS to pass extra global default clang-tidy flags.
	pctx.VariableFunc("TidyWithTidyFlags", func(ctx android.PackageVarContext) string {
		return ctx.Config().Getenv("WITH_TIDY_FLAGS")
	})
}

type PathBasedTidyCheck struct {
	PathPrefix string
	Checks     string
}

const tidyDefault = "${config.TidyDefaultGlobalChecks}"
const tidyExternalVendor = "${config.TidyExternalVendorChecks}"
const tidyDefaultNoAnalyzer = "${config.TidyDefaultGlobalChecks},-clang-analyzer-*"

// This is a map of local path prefixes to the set of default clang-tidy checks
// to be used.
// The last matched local_path_prefix should be the most specific to be used.
var DefaultLocalTidyChecks = []PathBasedTidyCheck{
	{"external/", tidyExternalVendor},
	{"external/google", tidyDefault},
	{"external/webrtc", tidyDefault},
	{"external/googletest/", tidyExternalVendor},
	{"frameworks/compile/mclinker/", tidyExternalVendor},
	{"hardware/qcom", tidyExternalVendor},
	{"vendor/", tidyExternalVendor},
	{"vendor/google", tidyDefault},
	{"vendor/google_devices", tidyExternalVendor},
}

var reversedDefaultLocalTidyChecks = reverseTidyChecks(DefaultLocalTidyChecks)

func reverseTidyChecks(in []PathBasedTidyCheck) []PathBasedTidyCheck {
	ret := make([]PathBasedTidyCheck, len(in))
	for i, check := range in {
		ret[len(in)-i-1] = check
	}
	return ret
}

func TidyChecksForDir(dir string) string {
	dir = dir + "/"
	for _, pathCheck := range reversedDefaultLocalTidyChecks {
		if strings.HasPrefix(dir, pathCheck.PathPrefix) {
			return pathCheck.Checks
		}
	}
	return tidyDefault
}

func TidyFlagsForSrcFile(srcFile android.Path, flags string) string {
	// Disable clang-analyzer-* checks globally for generated source files
	// because some of them are too huge. Local .bp files can add wanted
	// clang-analyzer checks through the tidy_checks property.
	// Need to do this patch per source file, because some modules
	// have both generated and organic source files.
	if _, ok := srcFile.(android.WritablePath); ok {
		if strings.Contains(flags, tidyDefault) {
			return strings.ReplaceAll(flags, tidyDefault, tidyDefaultNoAnalyzer)
		}
	}
	return flags
}
