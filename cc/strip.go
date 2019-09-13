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
	"strings"

	"android/soong/android"
)

type StripProperties struct {
	Strip struct {
		None                         *bool    `android:"arch_variant"`
		All                          *bool    `android:"arch_variant"`
		Keep_symbols                 *bool    `android:"arch_variant"`
		Keep_symbols_list            []string `android:"arch_variant"`
		Keep_symbols_and_debug_frame *bool    `android:"arch_variant"`
	} `android:"arch_variant"`
}

type stripper struct {
	StripProperties StripProperties
}

func (stripper *stripper) needsStrip(ctx ModuleContext) bool {
	// TODO(ccross): enable host stripping when embedded in make?  Make never had support for stripping host binaries.
	return (!ctx.Config().EmbeddedInMake() || ctx.Device()) && !Bool(stripper.StripProperties.Strip.None)
}

func (stripper *stripper) strip(ctx ModuleContext, in android.Path, out android.ModuleOutPath,
	flags builderFlags, isStaticLib bool) {
	if ctx.Darwin() {
		TransformDarwinStrip(ctx, in, out)
	} else {
		if Bool(stripper.StripProperties.Strip.Keep_symbols) {
			flags.stripKeepSymbols = true
		} else if Bool(stripper.StripProperties.Strip.Keep_symbols_and_debug_frame) {
			flags.stripKeepSymbolsAndDebugFrame = true
		} else if len(stripper.StripProperties.Strip.Keep_symbols_list) > 0 {
			flags.stripKeepSymbolsList = strings.Join(stripper.StripProperties.Strip.Keep_symbols_list, ",")
		} else if !Bool(stripper.StripProperties.Strip.All) {
			flags.stripKeepMiniDebugInfo = true
		}
		if ctx.Config().Debuggable() && !flags.stripKeepMiniDebugInfo && !isStaticLib {
			flags.stripAddGnuDebuglink = true
		}
		TransformStrip(ctx, in, out, flags)
	}
}

func (stripper *stripper) stripExecutableOrSharedLib(ctx ModuleContext, in android.Path,
	out android.ModuleOutPath, flags builderFlags) {
	stripper.strip(ctx, in, out, flags, false)
}

func (stripper *stripper) stripStaticLib(ctx ModuleContext, in android.Path, out android.ModuleOutPath,
	flags builderFlags) {
	stripper.strip(ctx, in, out, flags, true)
}
