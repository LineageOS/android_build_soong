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
	"fmt"
	"strings"

	"github.com/google/blueprint"

	"android/soong/common"
)

type sanitizerType int

func init() {
	pctx.StaticVariable("clangAsanLibDir", "${clangPath}/lib64/clang/3.8/lib/linux")
}

const (
	asan sanitizerType = iota + 1
	tsan
)

func (t sanitizerType) String() string {
	switch t {
	case asan:
		return "asan"
	case tsan:
		return "tsan"
	default:
		panic(fmt.Errorf("unknown sanitizerType %d", t))
	}
}

type SanitizeProperties struct {
	// enable AddressSanitizer, ThreadSanitizer, or UndefinedBehaviorSanitizer
	Sanitize struct {
		Never bool `android:"arch_variant"`

		// main sanitizers
		Address bool `android:"arch_variant"`
		Thread  bool `android:"arch_variant"`

		// local sanitizers
		Undefined      bool     `android:"arch_variant"`
		All_undefined  bool     `android:"arch_variant"`
		Misc_undefined []string `android:"arch_variant"`
		Coverage       bool     `android:"arch_variant"`

		// value to pass to -fsantitize-recover=
		Recover []string

		// value to pass to -fsanitize-blacklist
		Blacklist *string
	} `android:"arch_variant"`

	SanitizerEnabled bool `blueprint:"mutated"`
	SanitizeDep      bool `blueprint:"mutated"`
	InData           bool `blueprint:"mutated"`
}

type sanitize struct {
	Properties SanitizeProperties
}

func (sanitize *sanitize) props() []interface{} {
	return []interface{}{&sanitize.Properties}
}

func (sanitize *sanitize) begin(ctx BaseModuleContext) {
	// Don't apply sanitizers to NDK code.
	if ctx.sdk() {
		sanitize.Properties.Sanitize.Never = true
	}

	// Never always wins.
	if sanitize.Properties.Sanitize.Never {
		return
	}

	if ctx.ContainsProperty("sanitize") {
		sanitize.Properties.SanitizerEnabled = true
	}

	var globalSanitizers []string
	if ctx.clang() {
		if ctx.Host() {
			globalSanitizers = ctx.AConfig().SanitizeHost()
		} else {
			globalSanitizers = ctx.AConfig().SanitizeDevice()
		}
	}

	// The sanitizer specified by the environment wins over the module.
	if len(globalSanitizers) > 0 {
		// wipe the enabled sanitizers
		sanitize.Properties = SanitizeProperties{}
		var found bool
		if found, globalSanitizers = removeFromList("undefined", globalSanitizers); found {
			sanitize.Properties.Sanitize.All_undefined = true
		} else if found, globalSanitizers = removeFromList("default-ub", globalSanitizers); found {
			sanitize.Properties.Sanitize.Undefined = true
		}

		if found, globalSanitizers = removeFromList("address", globalSanitizers); found {
			sanitize.Properties.Sanitize.Address = true
		}

		if found, globalSanitizers = removeFromList("thread", globalSanitizers); found {
			sanitize.Properties.Sanitize.Thread = true
		}

		if found, globalSanitizers = removeFromList("coverage", globalSanitizers); found {
			sanitize.Properties.Sanitize.Coverage = true
		}

		if len(globalSanitizers) > 0 {
			ctx.ModuleErrorf("unknown global sanitizer option %s", globalSanitizers[0])
		}
		sanitize.Properties.SanitizerEnabled = true
	}

	if !ctx.toolchain().Is64Bit() && sanitize.Properties.Sanitize.Thread {
		// TSAN is not supported on 32-bit architectures
		sanitize.Properties.Sanitize.Thread = false
		// TODO(ccross): error for compile_multilib = "32"?
	}

	if sanitize.Properties.Sanitize.Coverage {
		if !sanitize.Properties.Sanitize.Address {
			ctx.ModuleErrorf(`Use of "coverage" also requires "address"`)
		}
	}
}

func (sanitize *sanitize) deps(ctx BaseModuleContext, deps Deps) Deps {
	if !sanitize.Properties.SanitizerEnabled { // || c.static() {
		return deps
	}

	if ctx.Device() {
		deps.SharedLibs = append(deps.SharedLibs, "libdl")
		if sanitize.Properties.Sanitize.Address {
			deps.StaticLibs = append(deps.StaticLibs, "libasan")
		}
	}

	return deps
}

func (sanitize *sanitize) flags(ctx ModuleContext, flags Flags) Flags {
	if !sanitize.Properties.SanitizerEnabled {
		return flags
	}

	if !ctx.clang() {
		ctx.ModuleErrorf("Use of sanitizers requires clang")
	}

	var sanitizers []string

	if sanitize.Properties.Sanitize.All_undefined {
		sanitizers = append(sanitizers, "undefined")
		if ctx.Device() {
			ctx.ModuleErrorf("ubsan is not yet supported on the device")
		}
	} else {
		if sanitize.Properties.Sanitize.Undefined {
			sanitizers = append(sanitizers,
				"bool",
				"integer-divide-by-zero",
				"return",
				"returns-nonnull-attribute",
				"shift-exponent",
				"unreachable",
				"vla-bound",
				// TODO(danalbert): The following checks currently have compiler performance issues.
				//"alignment",
				//"bounds",
				//"enum",
				//"float-cast-overflow",
				//"float-divide-by-zero",
				//"nonnull-attribute",
				//"null",
				//"shift-base",
				//"signed-integer-overflow",
				// TODO(danalbert): Fix UB in libc++'s __tree so we can turn this on.
				// https://llvm.org/PR19302
				// http://reviews.llvm.org/D6974
				// "object-size",
			)
		}
		sanitizers = append(sanitizers, sanitize.Properties.Sanitize.Misc_undefined...)
	}

	if sanitize.Properties.Sanitize.Address {
		if ctx.Arch().ArchType == common.Arm {
			// Frame pointer based unwinder in ASan requires ARM frame setup.
			// TODO: put in flags?
			flags.RequiredInstructionSet = "arm"
		}
		flags.CFlags = append(flags.CFlags, "-fno-omit-frame-pointer")
		flags.LdFlags = append(flags.LdFlags, "-Wl,-u,__asan_preinit")

		// ASan runtime library must be the first in the link order.
		runtimeLibrary := ctx.toolchain().AddressSanitizerRuntimeLibrary()
		if runtimeLibrary != "" {
			flags.libFlags = append([]string{"${clangAsanLibDir}/" + runtimeLibrary}, flags.libFlags...)
		}
		if ctx.Host() {
			// -nodefaultlibs (provided with libc++) prevents the driver from linking
			// libraries needed with -fsanitize=address. http://b/18650275 (WAI)
			flags.LdFlags = append(flags.LdFlags, "-lm", "-lpthread")
			flags.LdFlags = append(flags.LdFlags, "-Wl,--no-as-needed")
		} else {
			flags.CFlags = append(flags.CFlags, "-mllvm", "-asan-globals=0")
			flags.DynamicLinker = "/system/bin/linker_asan"
			if flags.Toolchain.Is64Bit() {
				flags.DynamicLinker += "64"
			}
		}
		sanitizers = append(sanitizers, "address")
	}

	if sanitize.Properties.Sanitize.Coverage {
		flags.CFlags = append(flags.CFlags, "-fsanitize-coverage=edge,indirect-calls,8bit-counters,trace-cmp")
	}

	if sanitize.Properties.Sanitize.Recover != nil {
		flags.CFlags = append(flags.CFlags, "-fsanitize-recover="+
			strings.Join(sanitize.Properties.Sanitize.Recover, ","))
	}

	if len(sanitizers) > 0 {
		sanitizeArg := "-fsanitize=" + strings.Join(sanitizers, ",")
		flags.CFlags = append(flags.CFlags, sanitizeArg)
		if ctx.Host() {
			flags.CFlags = append(flags.CFlags, "-fno-sanitize-recover=all")
			flags.LdFlags = append(flags.LdFlags, sanitizeArg)
			flags.LdFlags = append(flags.LdFlags, "-lrt", "-ldl")
		} else {
			if !sanitize.Properties.Sanitize.Address {
				flags.CFlags = append(flags.CFlags, "-fsanitize-trap=all", "-ftrap-function=abort")
			}
		}
	}

	blacklist := common.OptionalPathForModuleSrc(ctx, sanitize.Properties.Sanitize.Blacklist)
	if blacklist.Valid() {
		flags.CFlags = append(flags.CFlags, "-fsanitize-blacklist="+blacklist.String())
		flags.CFlagsDeps = append(flags.CFlagsDeps, blacklist.Path())
	}

	return flags
}

func (sanitize *sanitize) inData() bool {
	return sanitize.Properties.InData
}

func (sanitize *sanitize) Sanitizer(t sanitizerType) bool {
	if sanitize == nil {
		return false
	}

	switch t {
	case asan:
		return sanitize.Properties.Sanitize.Address
	case tsan:
		return sanitize.Properties.Sanitize.Thread
	default:
		panic(fmt.Errorf("unknown sanitizerType %d", t))
	}
}

func (sanitize *sanitize) SetSanitizer(t sanitizerType, b bool) {
	switch t {
	case asan:
		sanitize.Properties.Sanitize.Address = b
	case tsan:
		sanitize.Properties.Sanitize.Thread = b
	default:
		panic(fmt.Errorf("unknown sanitizerType %d", t))
	}
	if b {
		sanitize.Properties.SanitizerEnabled = true
	}
}

// Propagate asan requirements down from binaries
func sanitizerDepsMutator(t sanitizerType) func(common.AndroidTopDownMutatorContext) {
	return func(mctx common.AndroidTopDownMutatorContext) {
		if c, ok := mctx.Module().(*Module); ok && c.sanitize.Sanitizer(t) {
			mctx.VisitDepsDepthFirst(func(module blueprint.Module) {
				if d, ok := mctx.Module().(*Module); ok && c.sanitize != nil &&
					!c.sanitize.Properties.Sanitize.Never {
					d.sanitize.Properties.SanitizeDep = true
				}
			})
		}
	}
}

// Create asan variants for modules that need them
func sanitizerMutator(t sanitizerType) func(common.AndroidBottomUpMutatorContext) {
	return func(mctx common.AndroidBottomUpMutatorContext) {
		if c, ok := mctx.Module().(*Module); ok && c.sanitize != nil {
			if d, ok := c.linker.(baseLinkerInterface); ok && d.isDependencyRoot() && c.sanitize.Sanitizer(t) {
				modules := mctx.CreateVariations(t.String())
				modules[0].(*Module).sanitize.SetSanitizer(t, true)
				if mctx.AConfig().EmbeddedInMake() {
					modules[0].(*Module).sanitize.Properties.InData = true
				}
			} else if c.sanitize.Properties.SanitizeDep {
				if mctx.AConfig().EmbeddedInMake() {
					modules := mctx.CreateVariations(t.String())
					modules[0].(*Module).sanitize.SetSanitizer(t, true)
					modules[0].(*Module).sanitize.Properties.InData = true
				} else {
					modules := mctx.CreateVariations("", t.String())
					modules[0].(*Module).sanitize.SetSanitizer(t, false)
					modules[1].(*Module).sanitize.SetSanitizer(t, true)
					modules[1].(*Module).appendVariantName("_" + t.String())
					modules[0].(*Module).sanitize.Properties.SanitizeDep = false
					modules[1].(*Module).sanitize.Properties.SanitizeDep = false
				}
			}
			c.sanitize.Properties.SanitizeDep = false
		}
	}
}
