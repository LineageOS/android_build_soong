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

var (
	// Some clang-tidy checks have bugs or not work for Android.
	// They are disabled here, overriding any locally selected checks.
	globalNoCheckList = []string{
		// https://b.corp.google.com/issues/153464409
		// many local projects enable cert-* checks, which
		// trigger bugprone-reserved-identifier.
		"-bugprone-reserved-identifier*,-cert-dcl51-cpp,-cert-dcl37-c",
		// http://b/153757728
		"-readability-qualified-auto",
		// http://b/193716442
		"-bugprone-implicit-widening-of-multiplication-result",
		// Too many existing functions trigger this rule, and fixing it requires large code
		// refactoring. The cost of maintaining this tidy rule outweighs the benefit it brings.
		"-bugprone-easily-swappable-parameters",
		// http://b/216364337 - TODO: Follow-up after compiler update to
		// disable or fix individual instances.
		"-cert-err33-c",
	}

	// Some clang-tidy checks are included in some tidy_checks_as_errors lists,
	// but not all warnings are fixed/suppressed yet. These checks are not
	// disabled in the TidyGlobalNoChecks list, so we can see them and fix/suppress them.
	globalNoErrorCheckList = []string{
		// http://b/155034563
		"-bugprone-signed-char-misuse",
		// http://b/155034972
		"-bugprone-branch-clone",
	}
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
			// Select only google-* checks that do not have thousands of warnings.
			// Add more such checks when we clean up source code.
			// "google-build-using-namespace",
			// "google-default-arguments",
			// "google-explicit-constructor",
			// "google-global-names-in-headers",
			// "google-runtime-int",
			"google-build-explicit-make-pair",
			"google-build-namespaces",
			"google-runtime-operator",
			"google-upgrade-*",
			"misc-*",
			"performance-*",
			"portability-*",
			"-bugprone-easily-swappable-parameters",
			"-bugprone-narrowing-conversions",
			"-misc-no-recursion",
			"-misc-non-private-member-variables-in-classes",
			"-misc-unused-parameters",
			"-performance-no-int-to-ptr",
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
		// The insecureAPI.DeprecatedOrUnsafeBufferHandling warning does not apply to Android.
		if ctx.Config().IsEnvTrue("CLANG_ANALYZER_CHECKS") {
			checks += ",clang-analyzer-*,-clang-analyzer-security.insecureAPI.DeprecatedOrUnsafeBufferHandling"
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
			"google-build-explicit-make-pair",
			"google-build-namespaces",
			"google-runtime-operator",
			"google-upgrade-*",
		}, ",")
	})

	pctx.VariableFunc("TidyGlobalNoChecks", func(ctx android.PackageVarContext) string {
		return strings.Join(globalNoCheckList, ",")
	})

	pctx.VariableFunc("TidyGlobalNoErrorChecks", func(ctx android.PackageVarContext) string {
		return strings.Join(globalNoErrorCheckList, ",")
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
	{"vendor/google_arc/libs/org.chromium.arc.mojom", tidyExternalVendor},
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

// Returns a globally disabled tidy checks, overriding locally selected checks.
func TidyGlobalNoChecks() string {
	if len(globalNoCheckList) > 0 {
		return ",${config.TidyGlobalNoChecks}"
	}
	return ""
}

// Returns a globally allowed/no-error tidy checks, appended to -warnings-as-errors.
func TidyGlobalNoErrorChecks() string {
	if len(globalNoErrorCheckList) > 0 {
		return ",${config.TidyGlobalNoErrorChecks}"
	}
	return ""
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
