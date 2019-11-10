// Copyright 2017 Google Inc. All rights reserved.
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

// LTO (link-time optimization) allows the compiler to optimize and generate
// code for the entire module at link time, rather than per-compilation
// unit. LTO is required for Clang CFI and other whole-program optimization
// techniques. LTO also allows cross-compilation unit optimizations that should
// result in faster and smaller code, at the expense of additional compilation
// time.
//
// To properly build a module with LTO, the module and all recursive static
// dependencies should be compiled with -flto which directs the compiler to emit
// bitcode rather than native object files. These bitcode files are then passed
// by the linker to the LLVM plugin for compilation at link time. Static
// dependencies not built as bitcode will still function correctly but cannot be
// optimized at link time and may not be compatible with features that require
// LTO, such as CFI.
//
// This file adds support to soong to automatically propogate LTO options to a
// new variant of all static dependencies for each module with LTO enabled.

type LTOProperties struct {
	// Lto must violate capitialization style for acronyms so that it can be
	// referred to in blueprint files as "lto"
	Lto struct {
		Never *bool `android:"arch_variant"`
		Full  *bool `android:"arch_variant"`
		Thin  *bool `android:"arch_variant"`
	} `android:"arch_variant"`

	// Dep properties indicate that this module needs to be built with LTO
	// since it is an object dependency of an LTO module.
	FullDep bool `blueprint:"mutated"`
	ThinDep bool `blueprint:"mutated"`

	// Use clang lld instead of gnu ld.
	Use_clang_lld *bool
}

type lto struct {
	Properties LTOProperties
}

func (lto *lto) props() []interface{} {
	return []interface{}{&lto.Properties}
}

func (lto *lto) begin(ctx BaseModuleContext) {
	if ctx.Config().IsEnvTrue("DISABLE_LTO") {
		lto.Properties.Lto.Never = boolPtr(true)
	}
}

func (lto *lto) deps(ctx BaseModuleContext, deps Deps) Deps {
	return deps
}

func (lto *lto) useClangLld(ctx BaseModuleContext) bool {
	if lto.Properties.Use_clang_lld != nil {
		return Bool(lto.Properties.Use_clang_lld)
	}
	return true
}

func (lto *lto) flags(ctx BaseModuleContext, flags Flags) Flags {
	// TODO(b/131771163): Disable LTO when using explicit fuzzing configurations.
	// LTO breaks fuzzer builds.
	if inList("-fsanitize=fuzzer-no-link", flags.Local.CFlags) {
		return flags
	}

	if lto.LTO() {
		var ltoFlag string
		if Bool(lto.Properties.Lto.Thin) {
			ltoFlag = "-flto=thin -fsplit-lto-unit"
		} else {
			ltoFlag = "-flto"
		}

		flags.Local.CFlags = append(flags.Local.CFlags, ltoFlag)
		flags.Local.LdFlags = append(flags.Local.LdFlags, ltoFlag)

		if ctx.Config().IsEnvTrue("USE_THINLTO_CACHE") && Bool(lto.Properties.Lto.Thin) && lto.useClangLld(ctx) {
			// Set appropriate ThinLTO cache policy
			cacheDirFormat := "-Wl,--thinlto-cache-dir="
			cacheDir := android.PathForOutput(ctx, "thinlto-cache").String()
			flags.Local.LdFlags = append(flags.Local.LdFlags, cacheDirFormat+cacheDir)

			// Limit the size of the ThinLTO cache to the lesser of 10% of available
			// disk space and 10GB.
			cachePolicyFormat := "-Wl,--thinlto-cache-policy="
			policy := "cache_size=10%:cache_size_bytes=10g"
			flags.Local.LdFlags = append(flags.Local.LdFlags, cachePolicyFormat+policy)
		}

		// If the module does not have a profile, be conservative and do not inline
		// or unroll loops during LTO, in order to prevent significant size bloat.
		if !ctx.isPgoCompile() {
			flags.Local.LdFlags = append(flags.Local.LdFlags,
				"-Wl,-plugin-opt,-inline-threshold=0",
				"-Wl,-plugin-opt,-unroll-threshold=0")
		}
	}
	return flags
}

// Can be called with a null receiver
func (lto *lto) LTO() bool {
	if lto == nil || lto.Disabled() {
		return false
	}

	full := Bool(lto.Properties.Lto.Full)
	thin := Bool(lto.Properties.Lto.Thin)
	return full || thin
}

// Is lto.never explicitly set to true?
func (lto *lto) Disabled() bool {
	return lto.Properties.Lto.Never != nil && *lto.Properties.Lto.Never
}

// Propagate lto requirements down from binaries
func ltoDepsMutator(mctx android.TopDownMutatorContext) {
	if m, ok := mctx.Module().(*Module); ok && m.lto.LTO() {
		full := Bool(m.lto.Properties.Lto.Full)
		thin := Bool(m.lto.Properties.Lto.Thin)
		if full && thin {
			mctx.PropertyErrorf("LTO", "FullLTO and ThinLTO are mutually exclusive")
		}

		mctx.WalkDeps(func(dep android.Module, parent android.Module) bool {
			tag := mctx.OtherModuleDependencyTag(dep)
			switch tag {
			case StaticDepTag, staticExportDepTag, lateStaticDepTag, wholeStaticDepTag, objDepTag, reuseObjTag:
				if dep, ok := dep.(*Module); ok && dep.lto != nil &&
					!dep.lto.Disabled() {
					if full && !Bool(dep.lto.Properties.Lto.Full) {
						dep.lto.Properties.FullDep = true
					}
					if thin && !Bool(dep.lto.Properties.Lto.Thin) {
						dep.lto.Properties.ThinDep = true
					}
				}

				// Recursively walk static dependencies
				return true
			}

			// Do not recurse down non-static dependencies
			return false
		})
	}
}

// Create lto variants for modules that need them
func ltoMutator(mctx android.BottomUpMutatorContext) {
	if m, ok := mctx.Module().(*Module); ok && m.lto != nil {
		// Create variations for LTO types required as static
		// dependencies
		variationNames := []string{""}
		if m.lto.Properties.FullDep && !Bool(m.lto.Properties.Lto.Full) {
			variationNames = append(variationNames, "lto-full")
		}
		if m.lto.Properties.ThinDep && !Bool(m.lto.Properties.Lto.Thin) {
			variationNames = append(variationNames, "lto-thin")
		}

		// Use correct dependencies if LTO property is explicitly set
		// (mutually exclusive)
		if Bool(m.lto.Properties.Lto.Full) {
			mctx.SetDependencyVariation("lto-full")
		}
		if Bool(m.lto.Properties.Lto.Thin) {
			mctx.SetDependencyVariation("lto-thin")
		}

		if len(variationNames) > 1 {
			modules := mctx.CreateVariations(variationNames...)
			for i, name := range variationNames {
				variation := modules[i].(*Module)
				// Default module which will be
				// installed. Variation set above according to
				// explicit LTO properties
				if name == "" {
					continue
				}

				// LTO properties for dependencies
				if name == "lto-full" {
					variation.lto.Properties.Lto.Full = boolPtr(true)
					variation.lto.Properties.Lto.Thin = boolPtr(false)
				}
				if name == "lto-thin" {
					variation.lto.Properties.Lto.Full = boolPtr(false)
					variation.lto.Properties.Lto.Thin = boolPtr(true)
				}
				variation.Properties.PreventInstall = true
				variation.Properties.HideFromMake = true
				variation.lto.Properties.FullDep = false
				variation.lto.Properties.ThinDep = false
			}
		}
	}
}
