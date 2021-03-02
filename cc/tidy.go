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

package cc

import (
	"regexp"
	"strings"

	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/cc/config"
)

type TidyProperties struct {
	// whether to run clang-tidy over C-like sources.
	Tidy *bool

	// Extra flags to pass to clang-tidy
	Tidy_flags []string

	// Extra checks to enable or disable in clang-tidy
	Tidy_checks []string

	// Checks that should be treated as errors.
	Tidy_checks_as_errors []string
}

type tidyFeature struct {
	Properties TidyProperties
}

var quotedFlagRegexp, _ = regexp.Compile(`^-?-[^=]+=('|").*('|")$`)

// When passing flag -name=value, if user add quotes around 'value',
// the quotation marks will be preserved by NinjaAndShellEscapeList
// and the 'value' string with quotes won't work like the intended value.
// So here we report an error if -*='*' is found.
func checkNinjaAndShellEscapeList(ctx ModuleContext, prop string, slice []string) []string {
	for _, s := range slice {
		if quotedFlagRegexp.MatchString(s) {
			ctx.PropertyErrorf(prop, "Extra quotes in: %s", s)
		}
	}
	return proptools.NinjaAndShellEscapeList(slice)
}

func (tidy *tidyFeature) props() []interface{} {
	return []interface{}{&tidy.Properties}
}

func (tidy *tidyFeature) begin(ctx BaseModuleContext) {
}

func (tidy *tidyFeature) deps(ctx DepsContext, deps Deps) Deps {
	return deps
}

func (tidy *tidyFeature) flags(ctx ModuleContext, flags Flags) Flags {
	CheckBadTidyFlags(ctx, "tidy_flags", tidy.Properties.Tidy_flags)
	CheckBadTidyChecks(ctx, "tidy_checks", tidy.Properties.Tidy_checks)

	// Check if tidy is explicitly disabled for this module
	if tidy.Properties.Tidy != nil && !*tidy.Properties.Tidy {
		return flags
	}

	// If not explicitly set, check the global tidy flag
	if tidy.Properties.Tidy == nil && !ctx.Config().ClangTidy() {
		return flags
	}

	flags.Tidy = true

	// Add global WITH_TIDY_FLAGS and local tidy_flags.
	withTidyFlags := ctx.Config().Getenv("WITH_TIDY_FLAGS")
	if len(withTidyFlags) > 0 {
		flags.TidyFlags = append(flags.TidyFlags, withTidyFlags)
	}
	esc := checkNinjaAndShellEscapeList
	flags.TidyFlags = append(flags.TidyFlags, esc(ctx, "tidy_flags", tidy.Properties.Tidy_flags)...)
	// If TidyFlags does not contain -header-filter, add default header filter.
	// Find the substring because the flag could also appear as --header-filter=...
	// and with or without single or double quotes.
	if !android.SubstringInList(flags.TidyFlags, "-header-filter=") {
		defaultDirs := ctx.Config().Getenv("DEFAULT_TIDY_HEADER_DIRS")
		headerFilter := "-header-filter="
		if defaultDirs == "" {
			headerFilter += ctx.ModuleDir() + "/"
		} else {
			headerFilter += "\"(" + ctx.ModuleDir() + "/|" + defaultDirs + ")\""
		}
		flags.TidyFlags = append(flags.TidyFlags, headerFilter)
	}

	// If clang-tidy is not enabled globally, add the -quiet flag.
	if !ctx.Config().ClangTidy() {
		flags.TidyFlags = append(flags.TidyFlags, "-quiet")
		flags.TidyFlags = append(flags.TidyFlags, "-extra-arg-before=-fno-caret-diagnostics")
	}

	extraArgFlags := []string{
		// We might be using the static analyzer through clang tidy.
		// https://bugs.llvm.org/show_bug.cgi?id=32914
		"-D__clang_analyzer__",

		// A recent change in clang-tidy (r328258) enabled destructor inlining, which
		// appears to cause a number of false positives. Until that's resolved, this turns
		// off the effects of r328258.
		// https://bugs.llvm.org/show_bug.cgi?id=37459
		"-Xclang", "-analyzer-config", "-Xclang", "c++-temp-dtor-inlining=false",
	}

	for _, f := range extraArgFlags {
		flags.TidyFlags = append(flags.TidyFlags, "-extra-arg-before="+f)
	}

	tidyChecks := "-checks="
	if checks := ctx.Config().TidyChecks(); len(checks) > 0 {
		tidyChecks += checks
	} else {
		tidyChecks += config.TidyChecksForDir(ctx.ModuleDir())
	}
	if len(tidy.Properties.Tidy_checks) > 0 {
		tidyChecks = tidyChecks + "," + strings.Join(esc(ctx, "tidy_checks",
			config.ClangRewriteTidyChecks(tidy.Properties.Tidy_checks)), ",")
	}
	if ctx.Windows() {
		// https://b.corp.google.com/issues/120614316
		// mingw32 has cert-dcl16-c warning in NO_ERROR,
		// which is used in many Android files.
		tidyChecks = tidyChecks + ",-cert-dcl16-c"
	}
	// https://b.corp.google.com/issues/153464409
	// many local projects enable cert-* checks, which
	// trigger bugprone-reserved-identifier.
	tidyChecks = tidyChecks + ",-bugprone-reserved-identifier*,-cert-dcl51-cpp,-cert-dcl37-c"
	// http://b/153757728
	tidyChecks = tidyChecks + ",-readability-qualified-auto"
	// http://b/155034563
	tidyChecks = tidyChecks + ",-bugprone-signed-char-misuse"
	// http://b/155034972
	tidyChecks = tidyChecks + ",-bugprone-branch-clone"
	flags.TidyFlags = append(flags.TidyFlags, tidyChecks)

	if ctx.Config().IsEnvTrue("WITH_TIDY") {
		// WITH_TIDY=1 enables clang-tidy globally. There could be many unexpected
		// warnings from new checks and many local tidy_checks_as_errors and
		// -warnings-as-errors can break a global build.
		// So allow all clang-tidy warnings.
		inserted := false
		for i, s := range flags.TidyFlags {
			if strings.Contains(s, "-warnings-as-errors=") {
				// clang-tidy accepts only one -warnings-as-errors
				// replace the old one
				re := regexp.MustCompile(`'?-?-warnings-as-errors=[^ ]* *`)
				newFlag := re.ReplaceAllString(s, "")
				if newFlag == "" {
					flags.TidyFlags[i] = "-warnings-as-errors=-*"
				} else {
					flags.TidyFlags[i] = newFlag + " -warnings-as-errors=-*"
				}
				inserted = true
				break
			}
		}
		if !inserted {
			flags.TidyFlags = append(flags.TidyFlags, "-warnings-as-errors=-*")
		}
	} else if len(tidy.Properties.Tidy_checks_as_errors) > 0 {
		tidyChecksAsErrors := "-warnings-as-errors=" + strings.Join(esc(ctx, "tidy_checks_as_errors", tidy.Properties.Tidy_checks_as_errors), ",")
		flags.TidyFlags = append(flags.TidyFlags, tidyChecksAsErrors)
	}
	return flags
}
