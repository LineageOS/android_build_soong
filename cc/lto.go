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
// This file adds support to soong to automatically propagate LTO options to a
// new variant of all static dependencies for each module with LTO enabled.

type LTOProperties struct {
	// Lto must violate capitalization style for acronyms so that it can be
	// referred to in blueprint files as "lto"
	Lto struct {
		Never *bool `android:"arch_variant"`
		Thin  *bool `android:"arch_variant"`
	} `android:"arch_variant"`

	LtoEnabled bool `blueprint:"mutated"`
	LtoDefault bool `blueprint:"mutated"`

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
	// First, determine the module independent default LTO mode.
	ltoDefault := true
	if ctx.Config().IsEnvTrue("DISABLE_LTO") {
		ltoDefault = false
	} else if lto.Never() {
		ltoDefault = false
	} else if ctx.Host() {
		// Performance and binary size are less important for host binaries.
		ltoDefault = false
	} else if ctx.Arch().ArchType.Multilib == "lib32" {
		// LP32 has many subtle issues and less test coverage.
		ltoDefault = false
	}

	// Then, determine the actual LTO mode to use. If different from `ltoDefault`, a variant needs
	// to be created.
	ltoEnabled := ltoDefault
	if lto.Never() {
		ltoEnabled = false
	} else if lto.ThinLTO() {
		// Module explicitly requests for LTO.
		ltoEnabled = true
	} else if ctx.testBinary() || ctx.testLibrary() {
		// Do not enable LTO for tests for better debugging.
		ltoEnabled = false
	} else if ctx.isVndk() {
		// FIXME: ThinLTO for VNDK produces different output.
		// b/169217596
		ltoEnabled = false
	}

	lto.Properties.LtoDefault = ltoDefault
	lto.Properties.LtoEnabled = ltoEnabled
}

func (lto *lto) flags(ctx BaseModuleContext, flags Flags) Flags {
	// TODO(b/131771163): CFI and Fuzzer controls LTO flags by themselves.
	// This has be checked late because these properties can be mutated.
	if ctx.isCfi() || ctx.isFuzzer() {
		return flags
	}
	if lto.Properties.LtoEnabled {
		ltoCFlags := []string{"-flto=thin", "-fsplit-lto-unit"}
		var ltoLdFlags []string

		// The module did not explicitly turn on LTO. Only leverage LTO's
		// better dead code elimination and CFG simplification, but do
		// not perform costly optimizations for a balance between compile
		// time, binary size and performance.
		// Apply the same for Eng builds as well.
		if !lto.ThinLTO() || ctx.Config().Eng() {
			ltoLdFlags = append(ltoLdFlags, "-Wl,--lto-O0")
		}

		if Bool(lto.Properties.Whole_program_vtables) {
			ltoCFlags = append(ltoCFlags, "-fwhole-program-vtables")
		}

		if ctx.Config().IsEnvTrue("USE_THINLTO_CACHE") {
			// Set appropriate ThinLTO cache policy
			cacheDirFormat := "-Wl,--thinlto-cache-dir="
			cacheDir := android.PathForOutput(ctx, "thinlto-cache").String()
			ltoLdFlags = append(ltoLdFlags, cacheDirFormat+cacheDir)

			// Limit the size of the ThinLTO cache to the lesser of 10% of available
			// disk space and 10GB.
			cachePolicyFormat := "-Wl,--thinlto-cache-policy="
			policy := "cache_size=10%:cache_size_bytes=10g"
			ltoLdFlags = append(ltoLdFlags, cachePolicyFormat+policy)
		}

		// Reduce the inlining threshold for a better balance of binary size and
		// performance.
		if !ctx.Darwin() {
			if ctx.isAfdoCompile() {
				ltoLdFlags = append(ltoLdFlags, "-Wl,-plugin-opt,-import-instr-limit=40")
			} else {
				ltoLdFlags = append(ltoLdFlags, "-Wl,-plugin-opt,-import-instr-limit=5")
			}
		}

		if !ctx.Config().IsEnvFalse("THINLTO_USE_MLGO") {
			// Register allocation MLGO flags for ARM64.
			if ctx.Arch().ArchType == android.Arm64 {
				ltoLdFlags = append(ltoLdFlags, "-Wl,-mllvm,-regalloc-enable-advisor=release")
			}
			// Flags for training MLGO model.
			if ctx.Config().IsEnvTrue("THINLTO_EMIT_INDEXES_AND_IMPORTS") {
				ltoLdFlags = append(ltoLdFlags, "-Wl,--save-temps=import")
				ltoLdFlags = append(ltoLdFlags, "-Wl,--thinlto-emit-index-files")
			}
		}

		flags.Local.CFlags = append(flags.Local.CFlags, ltoCFlags...)
		flags.Local.AsFlags = append(flags.Local.AsFlags, ltoCFlags...)
		flags.Local.LdFlags = append(flags.Local.LdFlags, ltoCFlags...)
		flags.Local.LdFlags = append(flags.Local.LdFlags, ltoLdFlags...)
	}
	return flags
}

func (lto *lto) ThinLTO() bool {
	return lto != nil && proptools.Bool(lto.Properties.Lto.Thin)
}

func (lto *lto) Never() bool {
	return lto != nil && proptools.Bool(lto.Properties.Lto.Never)
}

// Propagate lto requirements down from binaries
func ltoDepsMutator(mctx android.TopDownMutatorContext) {
	if m, ok := mctx.Module().(*Module); ok {
		if m.lto == nil || m.lto.Properties.LtoEnabled == m.lto.Properties.LtoDefault {
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

		if !m.lto.Properties.LtoEnabled {
			mctx.SetDependencyVariation("lto-none")
		}
		if m.lto.Properties.LtoEnabled {
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
				variation.lto.Properties.LtoDefault = m.lto.Properties.LtoDefault
				variation.lto.Properties.LtoDep = false
				variation.lto.Properties.NoLtoDep = false
			}
		}
	}
}
