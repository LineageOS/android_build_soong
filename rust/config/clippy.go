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
	"strings"

	"android/soong/android"
)

var (
	defaultLints = []string{
		"-D missing-docs",
		"-D clippy::missing-safety-doc",
	}
	defaultVendorLints = []string{
		"",
	}
)

func init() {
	// Default Rust lints. These apply to all Google-authored modules.
	pctx.VariableFunc("ClippyDefaultLints", func(ctx android.PackageVarContext) string {
		if override := ctx.Config().Getenv("CLIPPY_DEFAULT_LINTS"); override != "" {
			return override
		}
		return strings.Join(defaultLints, " ")
	})

	// Rust lints that only applies to external code.
	pctx.VariableFunc("ClippyVendorLints", func(ctx android.PackageVarContext) string {
		if override := ctx.Config().Getenv("CLIPPY_VENDOR_LINTS"); override != "" {
			return override
		}
		return strings.Join(defaultVendorLints, " ")
	})
}

type PathBasedClippyConfig struct {
	PathPrefix   string
	Enabled      bool
	ClippyConfig string
}

const clippyNone = ""
const clippyDefault = "${config.ClippyDefaultLints}"
const clippyVendor = "${config.ClippyVendorLints}"

// This is a map of local path prefixes to a boolean indicating if the lint
// rule should be generated and if so, the set of lints to use. The first entry
// matching will be used. If no entry is matching, clippyDefault will be used.
var DefaultLocalTidyChecks = []PathBasedClippyConfig{
	{"external/", false, clippyNone},
	{"hardware/", true, clippyVendor},
	{"prebuilts/", false, clippyNone},
	{"vendor/google", true, clippyDefault},
	{"vendor/", true, clippyVendor},
}

// ClippyLintsForDir returns the Clippy lints to be used for a repository.
func ClippyLintsForDir(dir string) (bool, string) {
	for _, pathCheck := range DefaultLocalTidyChecks {
		if strings.HasPrefix(dir, pathCheck.PathPrefix) {
			return pathCheck.Enabled, pathCheck.ClippyConfig
		}
	}
	return true, clippyDefault
}
