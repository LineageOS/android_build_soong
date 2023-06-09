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

	"github.com/google/blueprint/proptools"
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
		Thin  *bool `android:"arch_variant"`
	} `android:"arch_variant"`

	LtoEnabled bool `blueprint:"mutated"`

	// Dep properties indicate that this module needs to be built with LTO
	// since it is an object dependency of an LTO module.
	LtoDep   bool `blueprint:"mutated"`
	NoLtoDep bool `blueprint:"mutated"`

	// Use -fwhole-program-vtables cflag.
	Whole_program_vtables *bool
}

type lto struct {
	Properties LTOProperties
}

func (lto *lto) props() []interface{} {
	return []interface{}{&lto.Properties}
}

func (lto *lto) begin(ctx BaseModuleContext) {
	lto.Properties.LtoEnabled = lto.LTO(ctx)
}

func (lto *lto) flags(ctx BaseModuleContext, flags Flags) Flags {
	// TODO(b/131771163): CFI and Fuzzer controls LTO flags by themselves.
	// This has be checked late because these properties can be mutated.
	if ctx.isCfi() || ctx.isFuzzer() {
		return flags
	}
	if lto.Properties.LtoEnabled {
		var ltoCFlag string
		var ltoLdFlag string
		if lto.ThinLTO() {
			ltoCFlag = "-flto=thin -fsplit-lto-unit"
		} else {
			ltoCFlag = "-flto=thin -fsplit-lto-unit"
			ltoLdFlag = "-Wl,--lto-O0"
		}

		flags.Local.CFlags = append(flags.Local.CFlags, ltoCFlag)
		flags.Local.AsFlags = append(flags.Local.AsFlags, ltoCFlag)
		flags.Local.LdFlags = append(flags.Local.LdFlags, ltoCFlag)
		flags.Local.LdFlags = append(flags.Local.LdFlags, ltoLdFlag)

		if Bool(lto.Properties.Whole_program_vtables) {
			flags.Local.CFlags = append(flags.Local.CFlags, "-fwhole-program-vtables")
		}

		if ctx.Config().IsEnvTrue("USE_THINLTO_CACHE") {
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

		// If the module does not have a profile, be conservative and limit cross TU inline
		// limit to 5 LLVM IR instructions, to balance binary size increase and performance.
		if !ctx.isPgoCompile() && !ctx.isAfdoCompile() {
			flags.Local.LdFlags = append(flags.Local.LdFlags,
				"-Wl,-plugin-opt,-import-instr-limit=5")
		}
	}
	return flags
}

// Determine which LTO mode to use for the given module.
func (lto *lto) LTO(ctx BaseModuleContext) bool {
	if lto.Never() {
		return false
	}
	if ctx.Config().IsEnvTrue("DISABLE_LTO") {
		return false
	}
	// Module explicitly requests for LTO.
	if lto.ThinLTO() {
		return true
	}
	// LP32 has many subtle issues and less test coverage.
	if ctx.Arch().ArchType.Multilib == "lib32" {
		return false
	}
	// Performance and binary size are less important for host binaries and tests.
	if ctx.Host() || ctx.testBinary() || ctx.testLibrary() {
		return false
	}
	// FIXME: ThinLTO for VNDK produces different output.
	// b/169217596
	if ctx.isVndk() {
		return false
	}
	return GlobalThinLTO(ctx)
}

func (lto *lto) ThinLTO() bool {
	return lto != nil && proptools.Bool(lto.Properties.Lto.Thin)
}

func (lto *lto) Never() bool {
	return lto != nil && proptools.Bool(lto.Properties.Lto.Never)
}

func GlobalThinLTO(ctx android.BaseModuleContext) bool {
	return ctx.Config().IsEnvTrue("GLOBAL_THINLTO")
}

// Propagate lto requirements down from binaries
func ltoDepsMutator(mctx android.TopDownMutatorContext) {
	defaultLTOMode := GlobalThinLTO(mctx)

	if m, ok := mctx.Module().(*Module); ok {
		if m.lto == nil || m.lto.Properties.LtoEnabled == defaultLTOMode {
			return
		}

		mctx.WalkDeps(func(dep android.Module, parent android.Module) bool {
			tag := mctx.OtherModuleDependencyTag(dep)
			libTag, isLibTag := tag.(libraryDependencyTag)

			// Do not recurse down non-static dependencies
			if isLibTag {
				if !libTag.static() {
					return false
				}
			} else {
				if tag != objDepTag && tag != reuseObjTag {
					return false
				}
			}

			if dep, ok := dep.(*Module); ok {
				if m.lto.Properties.LtoEnabled {
					dep.lto.Properties.LtoDep = true
				} else {
					dep.lto.Properties.NoLtoDep = true
				}
			}

			// Recursively walk static dependencies
			return true
		})
	}
}

// Create lto variants for modules that need them
func ltoMutator(mctx android.BottomUpMutatorContext) {
	globalThinLTO := GlobalThinLTO(mctx)

	if m, ok := mctx.Module().(*Module); ok && m.lto != nil {
		// Create variations for LTO types required as static
		// dependencies
		variationNames := []string{""}
		if m.lto.Properties.LtoDep {
			variationNames = append(variationNames, "lto-thin")
		}
		if m.lto.Properties.NoLtoDep {
			variationNames = append(variationNames, "lto-none")
		}

		if globalThinLTO && !m.lto.Properties.LtoEnabled {
			mctx.SetDependencyVariation("lto-none")
		}
		if !globalThinLTO && m.lto.Properties.LtoEnabled {
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
				if name == "lto-thin" {
					variation.lto.Properties.LtoEnabled = true
				}
				if name == "lto-none" {
					variation.lto.Properties.LtoEnabled = false
				}
				variation.Properties.PreventInstall = true
				variation.Properties.HideFromMake = true
				variation.lto.Properties.LtoDep = false
				variation.lto.Properties.NoLtoDep = false
			}
		}
	}
}
