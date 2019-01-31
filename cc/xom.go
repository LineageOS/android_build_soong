// Copyright 2018 Google Inc. All rights reserved.
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
	"android/soong/android"
)

type XomProperties struct {
	Xom *bool
}

type xom struct {
	Properties XomProperties
}

func (xom *xom) props() []interface{} {
	return []interface{}{&xom.Properties}
}

func (xom *xom) begin(ctx BaseModuleContext) {}

func (xom *xom) deps(ctx BaseModuleContext, deps Deps) Deps {
	return deps
}

func (xom *xom) flags(ctx ModuleContext, flags Flags) Flags {
	disableXom := false

	if !ctx.Config().EnableXOM() || ctx.Config().XOMDisabledForPath(ctx.ModuleDir()) {
		disableXom = true
	}

	if xom.Properties.Xom != nil && !*xom.Properties.Xom {
		return flags
	}

	// If any static dependencies have XOM disabled, we should disable XOM in this module,
	// the assumption being if it's been explicitly disabled then there's probably incompatible
	// code in the library which may get pulled in.
	if !disableXom {
		ctx.VisitDirectDeps(func(m android.Module) {
			cc, ok := m.(*Module)
			if !ok || cc.xom == nil || !cc.static() {
				return
			}
			if cc.xom.Properties.Xom != nil && !*cc.xom.Properties.Xom {
				disableXom = true
				return
			}
		})
	}

	// Enable execute-only if none of the dependencies disable it,
	// also if it's explicitly set true (allows overriding dependencies disabling it).
	if !disableXom || (xom.Properties.Xom != nil && *xom.Properties.Xom) {
		// XOM is only supported on AArch64 when using lld.
		if ctx.Arch().ArchType == android.Arm64 && ctx.useClangLld(ctx) {
			flags.LdFlags = append(flags.LdFlags, "-Wl,-execute-only")
		}
	}

	return flags
}
