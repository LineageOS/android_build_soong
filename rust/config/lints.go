// Copyright 2020 The Android Open Source Project
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
	"fmt"
	"strings"

	"android/soong/android"
)

// Overarching principles for Rust lints on Android:
// The Android build system tries to avoid reporting warnings during the build.
// Therefore, by default, we upgrade warnings to denials. For some of these
// lints, an allow exception is setup, using the variables below.
//
// The lints are split into two categories. The first one contains the built-in
// lints (https://doc.rust-lang.org/rustc/lints/index.html). The second is
// specific to Clippy lints (https://rust-lang.github.io/rust-clippy/master/).
//
// For both categories, there are 3 levels of linting possible:
// - "android", for the strictest lints that applies to all Android platform code.
// - "vendor", for relaxed rules.
// - "none", to disable the linting.
// There is a fourth option ("default") which automatically selects the linting level
// based on the module's location. See defaultLintSetForPath.
//
// When developing a module, you may set `lints = "none"` and `clippy_lints =
// "none"` to disable all the linting. Expect some questioning during code review
// if you enable one of these options.
var (
	// Default Rust lints that applies to Google-authored modules.
	defaultRustcLints = []string{
		"-A deprecated",
		"-D missing-docs",
		"-D warnings",
		"-D unsafe_op_in_unsafe_fn",
	}
	// Default Clippy lints. These are applied on top of defaultRustcLints.
	// It should be assumed that any warning lint will be promoted to a
	// deny.
	defaultClippyLints = []string{
		"-A clippy::type-complexity",
		"-A clippy::unnecessary-wraps",
		"-A clippy::unusual-byte-groupings",
		"-A clippy::upper-case-acronyms",
		"-D clippy::undocumented_unsafe_blocks",
	}

	// Rust lints for vendor code.
	defaultRustcVendorLints = []string{
		"-A deprecated",
		"-D warnings",
	}
	// Clippy lints for vendor source. These are applied on top of
	// defaultRustcVendorLints.  It should be assumed that any warning lint
	// will be promoted to a deny.
	defaultClippyVendorLints = []string{
		"-A clippy::complexity",
		"-A clippy::perf",
		"-A clippy::style",
	}

	// For prebuilts/ and external/, no linting is expected. If a warning
	// or a deny is reported, it should be fixed upstream.
	allowAllLints = []string{
		"--cap-lints allow",
	}
)

func init() {
	// Default Rust lints. These apply to all Google-authored modules.
	pctx.VariableFunc("RustDefaultLints", func(ctx android.PackageVarContext) string {
		if override := ctx.Config().Getenv("RUST_DEFAULT_LINTS"); override != "" {
			return override
		}
		return strings.Join(defaultRustcLints, " ")
	})
	pctx.VariableFunc("ClippyDefaultLints", func(ctx android.PackageVarContext) string {
		if override := ctx.Config().Getenv("CLIPPY_DEFAULT_LINTS"); override != "" {
			return override
		}
		return strings.Join(defaultClippyLints, " ")
	})

	// Rust lints that only applies to external code.
	pctx.VariableFunc("RustVendorLints", func(ctx android.PackageVarContext) string {
		if override := ctx.Config().Getenv("RUST_VENDOR_LINTS"); override != "" {
			return override
		}
		return strings.Join(defaultRustcVendorLints, " ")
	})
	pctx.VariableFunc("ClippyVendorLints", func(ctx android.PackageVarContext) string {
		if override := ctx.Config().Getenv("CLIPPY_VENDOR_LINTS"); override != "" {
			return override
		}
		return strings.Join(defaultClippyVendorLints, " ")
	})
	pctx.StaticVariable("RustAllowAllLints", strings.Join(allowAllLints, " "))
}

const noLint = ""
const rustcDefault = "${config.RustDefaultLints}"
const rustcVendor = "${config.RustVendorLints}"
const rustcAllowAll = "${config.RustAllowAllLints}"
const clippyDefault = "${config.ClippyDefaultLints}"
const clippyVendor = "${config.ClippyVendorLints}"

// lintConfig defines a set of lints and clippy configuration.
type lintConfig struct {
	rustcConfig   string // for the lints to apply to rustc.
	clippyEnabled bool   // to indicate if clippy should be executed.
	clippyConfig  string // for the lints to apply to clippy.
}

const (
	androidLints = "android"
	vendorLints  = "vendor"
	noneLints    = "none"
)

// lintSets defines the categories of linting for Android and their mapping to lintConfigs.
var lintSets = map[string]lintConfig{
	androidLints: {rustcDefault, true, clippyDefault},
	vendorLints:  {rustcVendor, true, clippyVendor},
	noneLints:    {rustcAllowAll, false, noLint},
}

type pathLintSet struct {
	prefix string
	set    string
}

// This is a map of local path prefixes to a lint set.  The first entry
// matching will be used. If no entry matches, androidLints ("android") will be
// used.
var defaultLintSetForPath = []pathLintSet{
	{"external", noneLints},
	{"hardware", vendorLints},
	{"prebuilts", noneLints},
	{"vendor/google", androidLints},
	{"vendor", vendorLints},
}

// ClippyLintsForDir returns a boolean if Clippy should be executed and if so, the lints to be used.
func ClippyLintsForDir(dir string, clippyLintsProperty *string) (bool, string, error) {
	if clippyLintsProperty != nil {
		set, ok := lintSets[*clippyLintsProperty]
		if ok {
			return set.clippyEnabled, set.clippyConfig, nil
		}
		if *clippyLintsProperty != "default" {
			return false, "", fmt.Errorf("unknown value for `clippy_lints`: %v, valid options are: default, android, vendor or none", *clippyLintsProperty)
		}
	}
	for _, p := range defaultLintSetForPath {
		if strings.HasPrefix(dir, p.prefix) {
			setConfig := lintSets[p.set]
			return setConfig.clippyEnabled, setConfig.clippyConfig, nil
		}
	}
	return true, clippyDefault, nil
}

// RustcLintsForDir returns the standard lints to be used for a repository.
func RustcLintsForDir(dir string, lintProperty *string) (string, error) {
	if lintProperty != nil {
		set, ok := lintSets[*lintProperty]
		if ok {
			return set.rustcConfig, nil
		}
		if *lintProperty != "default" {
			return "", fmt.Errorf("unknown value for `lints`: %v, valid options are: default, android, vendor or none", *lintProperty)
		}

	}
	for _, p := range defaultLintSetForPath {
		if strings.HasPrefix(dir, p.prefix) {
			return lintSets[p.set].rustcConfig, nil
		}
	}
	return rustcDefault, nil
}
