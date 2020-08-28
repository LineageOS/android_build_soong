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

type Stripper struct {
	StripProperties StripProperties
}

func (stripper *Stripper) NeedsStrip(actx android.ModuleContext) bool {
	// TODO(ccross): enable host stripping when embedded in make?  Make never had support for stripping host binaries.
	return (!actx.Config().EmbeddedInMake() || actx.Device()) && !Bool(stripper.StripProperties.Strip.None)
}

func (stripper *Stripper) strip(actx android.ModuleContext, in android.Path, out android.ModuleOutPath,
	flags StripFlags, isStaticLib bool) {
	if actx.Darwin() {
		TransformDarwinStrip(actx, in, out)
	} else {
		if Bool(stripper.StripProperties.Strip.Keep_symbols) {
			flags.StripKeepSymbols = true
		} else if Bool(stripper.StripProperties.Strip.Keep_symbols_and_debug_frame) {
			flags.StripKeepSymbolsAndDebugFrame = true
		} else if len(stripper.StripProperties.Strip.Keep_symbols_list) > 0 {
			flags.StripKeepSymbolsList = strings.Join(stripper.StripProperties.Strip.Keep_symbols_list, ",")
		} else if !Bool(stripper.StripProperties.Strip.All) {
			flags.StripKeepMiniDebugInfo = true
		}
		if actx.Config().Debuggable() && !flags.StripKeepMiniDebugInfo && !isStaticLib {
			flags.StripAddGnuDebuglink = true
		}
		TransformStrip(actx, in, out, flags)
	}
}

func (stripper *Stripper) StripExecutableOrSharedLib(actx android.ModuleContext, in android.Path,
	out android.ModuleOutPath, flags StripFlags) {
	stripper.strip(actx, in, out, flags, false)
}

func (stripper *Stripper) StripStaticLib(actx android.ModuleContext, in android.Path, out android.ModuleOutPath,
	flags StripFlags) {
	stripper.strip(actx, in, out, flags, true)
}
