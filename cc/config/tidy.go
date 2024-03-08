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
	// Some clang-tidy checks have bugs or don't work for Android.
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
		// http://b/241125373
		"-bugprone-unchecked-optional-access",
		// http://b/265438407
		"-misc-use-anonymous-namespace",
		// http://b/285005947
		"-performance-avoid-endl",
	}

	// Some clang-tidy checks are included in some tidy_checks_as_errors lists,
	// but not all warnings are fixed/suppressed yet. These checks are not
	// disabled in the TidyGlobalNoChecks list, so we can see them and fix/suppress them.
	globalNoErrorCheckList = []string{
		// http://b/241997913
		"-bugprone-assignment-in-if-condition",
		// http://b/155034972
		"-bugprone-branch-clone",
		// http://b/155034563
		"-bugprone-signed-char-misuse",
		// http://b/241819232
		"-misc-const-correctness",
		// http://b/285356805
		"-bugprone-unsafe-functions",
		"-cert-msc24-c",
		"-cert-msc33-c",
		// http://b/285356799
		"-modernize-type-traits",
		// http://b/285361108
		"-readability-avoid-unconditional-preprocessor-if",
	}

	extraArgFlags = []string{
		// We might be using the static analyzer through clang tidy.
		// https://bugs.llvm.org/show_bug.cgi?id=32914
		"-D__clang_analyzer__",

		// A recent change in clang-tidy (r328258) enabled destructor inlining, which
		// appears to cause a number of false positives. Until that's resolved, this turns
		// off the effects of r328258.
		// https://bugs.llvm.org/show_bug.cgi?id=37459
		"-Xclang",
		"-analyzer-config",
		"-Xclang",
		"c++-temp-dtor-inlining=false",
	}
)

func init() {
	// The global default tidy checks should include clang-tidy
	// default checks and tested groups, but exclude known noisy checks.
	// See https://clang.llvm.org/extra/clang-tidy/checks/list.html
	exportedVars.ExportVariableConfigMethod("TidyDefaultGlobalChecks", func(config android.Config) string {
		if override := config.Getenv("DEFAULT_GLOBAL_TIDY_CHECKS"); override != "" {
			return override
		}
		checks := strings.Join([]string{
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
			"-bugprone-assignment-in-if-condition",
			"-bugprone-easily-swappable-parameters",
			"-bugprone-narrowing-conversions",
			"-misc-const-correctness",
			"-misc-no-recursion",
			"-misc-non-private-member-variables-in-classes",
			"-misc-unused-parameters",
			"-performance-no-int-to-ptr",
			// the following groups are not in clang-tidy default checks.
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
		// clang-analyzer-* checks are slow for large files, but we have TIDY_TIMEOUT to
		// limit clang-tidy runtime. We allow clang-tidy default clang-analyzer-* checks,
		// and add it explicitly when CLANG_ANALYZER_CHECKS is set.
		// The insecureAPI.DeprecatedOrUnsafeBufferHandling warning does not apply to Android.
		if config.IsEnvTrue("CLANG_ANALYZER_CHECKS") {
			checks += ",clang-analyzer-*,-clang-analyzer-security.insecureAPI.DeprecatedOrUnsafeBufferHandling"
		} else {
			checks += ",-clang-analyzer-security.insecureAPI.DeprecatedOrUnsafeBufferHandling"
		}
		return checks
	})

	// The external and vendor projects do not run clang-tidy unless TIDY_EXTERNAL_VENDOR is set.
	// We do not add "-*" to the check list to avoid suppressing the check list in .clang-tidy config files.
	// There are too many clang-tidy warnings in external and vendor projects, so we only
	// enable some google checks for these projects. Users can add more checks locally with the
	// "tidy_checks" list in .bp files, or the "Checks" list in .clang-tidy config files.
	exportedVars.ExportVariableConfigMethod("TidyExternalVendorChecks", func(config android.Config) string {
		if override := config.Getenv("DEFAULT_EXTERNAL_VENDOR_TIDY_CHECKS"); override != "" {
			return override
		}
		return strings.Join([]string{
			"clang-diagnostic-unused-command-line-argument",
			"google-build-explicit-make-pair",
			"google-build-namespaces",
			"google-runtime-operator",
			"google-upgrade-*",
			"-clang-analyzer-security.insecureAPI.DeprecatedOrUnsafeBufferHandling",
		}, ",")
	})

	exportedVars.ExportVariableFuncVariable("TidyGlobalNoChecks", func() string {
		return strings.Join(globalNoCheckList, ",")
	})

	exportedVars.ExportVariableFuncVariable("TidyGlobalNoErrorChecks", func() string {
		return strings.Join(globalNoErrorCheckList, ",")
	})

	exportedVars.ExportStringListStaticVariable("TidyExtraArgFlags", extraArgFlags)

	// To reduce duplicate warnings from the same header files,
	// header-filter will contain only the module directory and
	// those specified by DEFAULT_TIDY_HEADER_DIRS.
	exportedVars.ExportVariableConfigMethod("TidyDefaultHeaderDirs", func(config android.Config) string {
		return config.Getenv("DEFAULT_TIDY_HEADER_DIRS")
	})

	// Use WTIH_TIDY_FLAGS to pass extra global default clang-tidy flags.
	exportedVars.ExportVariableConfigMethod("TidyWithTidyFlags", func(config android.Config) string {
		return config.Getenv("WITH_TIDY_FLAGS")
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
// to be used.  This is like android.IsThirdPartyPath, but with more patterns.
// The last matched local_path_prefix should be the most specific to be used.
var DefaultLocalTidyChecks = []PathBasedTidyCheck{
	{"external/", tidyExternalVendor},
	{"frameworks/compile/mclinker/", tidyExternalVendor},
	{"hardware/", tidyExternalVendor},
	{"hardware/google/", tidyDefault},
	{"hardware/interfaces/", tidyDefault},
	{"hardware/ril/", tidyDefault},
	{"hardware/libhardware", tidyDefault}, // all 'hardware/libhardware*'
	{"vendor/", tidyExternalVendor},
	{"vendor/google", tidyDefault}, // all 'vendor/google*'
	{"vendor/google/external/", tidyExternalVendor},
	{"vendor/google_arc/libs/org.chromium.arc.mojom", tidyExternalVendor},
	{"vendor/google_devices/", tidyExternalVendor}, // many have vendor code
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

func neverTidyForDir(dir string) bool {
	// This function can be extended if tidy needs to be disabled for more directories.
	return strings.HasPrefix(dir, "external/grpc-grpc")
}

func NoClangTidyForDir(allowExternalVendor bool, dir string) bool {
	// Tidy can be disable for a module in dir, if the dir is "neverTidyForDir",
	// or if it belongs to external|vendor and !allowExternalVendor.
	// This function depends on TidyChecksForDir, which selects tidyExternalVendor
	// checks for external/vendor projects.
	return neverTidyForDir(dir) ||
		(!allowExternalVendor && TidyChecksForDir(dir) == tidyExternalVendor)
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

func TidyExtraArgFlags() []string {
	return extraArgFlags
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
