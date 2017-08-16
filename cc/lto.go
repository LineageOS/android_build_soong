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
	"github.com/google/blueprint"

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
	Lto    *bool `android:"arch_variant"`
	LTODep bool  `blueprint:"mutated"`
}

type lto struct {
	Properties LTOProperties
}

func (lto *lto) props() []interface{} {
	return []interface{}{&lto.Properties}
}

func (lto *lto) begin(ctx BaseModuleContext) {
}

func (lto *lto) deps(ctx BaseModuleContext, deps Deps) Deps {
	return deps
}

func (lto *lto) flags(ctx BaseModuleContext, flags Flags) Flags {
	if Bool(lto.Properties.Lto) {
		flags.CFlags = append(flags.CFlags, "-flto")
		flags.LdFlags = append(flags.LdFlags, "-flto")
		if ctx.Device() {
			// Work around bug in Clang that doesn't pass correct emulated
			// TLS option to target
			flags.LdFlags = append(flags.LdFlags, "-Wl,-plugin-opt,-emulated-tls")
		}
		flags.ArFlags = append(flags.ArFlags, " --plugin ${config.LLVMGoldPlugin}")
	}
	return flags
}

// Can be called with a null receiver
func (lto *lto) LTO() bool {
	if lto == nil {
		return false
	}

	return Bool(lto.Properties.Lto)
}

// Propagate lto requirements down from binaries
func ltoDepsMutator(mctx android.TopDownMutatorContext) {
	if c, ok := mctx.Module().(*Module); ok && c.lto.LTO() {
		mctx.VisitDepsDepthFirst(func(m blueprint.Module) {
			tag := mctx.OtherModuleDependencyTag(m)
			switch tag {
			case staticDepTag, staticExportDepTag, lateStaticDepTag, wholeStaticDepTag, objDepTag, reuseObjTag:
				if cc, ok := m.(*Module); ok && cc.lto != nil {
					cc.lto.Properties.LTODep = true
				}
			}
		})
	}
}

// Create lto variants for modules that need them
func ltoMutator(mctx android.BottomUpMutatorContext) {
	if c, ok := mctx.Module().(*Module); ok && c.lto != nil {
		if c.lto.LTO() {
			mctx.SetDependencyVariation("lto")
		} else if c.lto.Properties.LTODep {
			modules := mctx.CreateVariations("", "lto")
			modules[0].(*Module).lto.Properties.Lto = boolPtr(false)
			modules[1].(*Module).lto.Properties.Lto = boolPtr(true)
			modules[0].(*Module).lto.Properties.LTODep = false
			modules[1].(*Module).lto.Properties.LTODep = false
			modules[1].(*Module).Properties.PreventInstall = true
			modules[1].(*Module).Properties.HideFromMake = true
		}
		c.lto.Properties.LTODep = false
	}
}
