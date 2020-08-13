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

package rust

import (
	"android/soong/rust/config"
)

type ClippyProperties struct {
	// name of the lint set that should be used to validate this module.
	//
	// Possible values are "default" (for using a sensible set of lints
	// depending on the module's location), "android" (for the strictest
	// lint set that applies to all Android platform code), "vendor" (for a
	// relaxed set) and "none" (to disable the execution of clippy).  The
	// default value is "default". See also the `lints` property.
	Clippy_lints *string
}

type clippy struct {
	Properties ClippyProperties
}

func (c *clippy) props() []interface{} {
	return []interface{}{&c.Properties}
}

func (c *clippy) flags(ctx ModuleContext, flags Flags, deps PathDeps) (Flags, PathDeps) {
	enabled, lints, err := config.ClippyLintsForDir(ctx.ModuleDir(), c.Properties.Clippy_lints)
	if err != nil {
		ctx.PropertyErrorf("clippy_lints", err.Error())
	}
	flags.Clippy = enabled
	flags.ClippyFlags = append(flags.ClippyFlags, lints)
	return flags, deps
}
