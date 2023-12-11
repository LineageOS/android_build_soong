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

// StripProperties defines the type of stripping applied to the module.
type StripProperties struct {
	Strip struct {
		// none forces all stripping to be disabled.
		// Device modules default to stripping enabled leaving mini debuginfo.
		// Host modules default to stripping disabled, but can be enabled by setting any other
		// strip boolean property.
		None *bool `android:"arch_variant"`

		// all forces stripping everything, including the mini debug info.
		All *bool `android:"arch_variant"`

		// keep_symbols enables stripping but keeps all symbols.
		Keep_symbols *bool `android:"arch_variant"`

		// keep_symbols_list specifies a list of symbols to keep if keep_symbols is enabled.
		// If it is unset then all symbols are kept.
		Keep_symbols_list []string `android:"arch_variant"`

		// keep_symbols_and_debug_frame enables stripping but keeps all symbols and debug frames.
		Keep_symbols_and_debug_frame *bool `android:"arch_variant"`
	} `android:"arch_variant"`
}

// Stripper defines the stripping actions and properties for a module.
type Stripper struct {
	StripProperties StripProperties
}

// NeedsStrip determines if stripping is required for a module.
func (stripper *Stripper) NeedsStrip(actx android.ModuleContext) bool {
	forceDisable := Bool(stripper.StripProperties.Strip.None)
	defaultEnable := (!actx.Config().KatiEnabled() || actx.Device())
	forceEnable := Bool(stripper.StripProperties.Strip.All) ||
		Bool(stripper.StripProperties.Strip.Keep_symbols) ||
		Bool(stripper.StripProperties.Strip.Keep_symbols_and_debug_frame)
	return !forceDisable && (forceEnable || defaultEnable)
}

func (stripper *Stripper) strip(actx android.ModuleContext, in android.Path, out android.ModuleOutPath,
	flags StripFlags, isStaticLib bool) {
	if actx.Darwin() {
		transformDarwinStrip(actx, in, out)
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
		transformStrip(actx, in, out, flags)
	}
}

// StripExecutableOrSharedLib strips a binary or shared library from its debug
// symbols and other debugging information. The helper function
// flagsToStripFlags may be used to generate the flags argument.
func (stripper *Stripper) StripExecutableOrSharedLib(actx android.ModuleContext, in android.Path,
	out android.ModuleOutPath, flags StripFlags) {
	stripper.strip(actx, in, out, flags, false)
}

// StripStaticLib strips a static library from its debug symbols and other
// debugging information. The helper function flagsToStripFlags may be used to
// generate the flags argument.
func (stripper *Stripper) StripStaticLib(actx android.ModuleContext, in android.Path, out android.ModuleOutPath,
	flags StripFlags) {
	stripper.strip(actx, in, out, flags, true)
}
