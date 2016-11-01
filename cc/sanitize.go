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

	"android/soong/android"
	"android/soong/cc/config"
)

const (
	asanCflags  = "-fno-omit-frame-pointer"
	asanLdflags = "-Wl,-u,__asan_preinit"
	asanLibs    = "libasan"
)

type sanitizerType int

func boolPtr(v bool) *bool {
	if v {
		return &v
	} else {
		return nil
	}
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
		Address *bool `android:"arch_variant"`
		Thread  *bool `android:"arch_variant"`

		// local sanitizers
		Undefined      *bool    `android:"arch_variant"`
		All_undefined  *bool    `android:"arch_variant"`
		Misc_undefined []string `android:"arch_variant"`
		Coverage       *bool    `android:"arch_variant"`
		Safestack      *bool    `android:"arch_variant"`
		Cfi            *bool    `android:"arch_variant"`

		// Sanitizers to run in the diagnostic mode (as opposed to the release mode).
		// Replaces abort() on error with a human-readable error message.
		// Address and Thread sanitizers always run in diagnostic mode.
		Diag struct {
			Undefined *bool `android:"arch_variant"`
			Cfi       *bool `android:"arch_variant"`
		}

		// value to pass to -fsanitize-recover=
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
	s := &sanitize.Properties.Sanitize

	// Don't apply sanitizers to NDK code.
	if ctx.sdk() {
		s.Never = true
	}

	// Never always wins.
	if s.Never {
		return
	}

	var globalSanitizers []string
	if ctx.clang() {
		if ctx.Host() {
			globalSanitizers = ctx.AConfig().SanitizeHost()
		} else {
			globalSanitizers = ctx.AConfig().SanitizeDevice()
		}
	}

	if len(globalSanitizers) > 0 {
		var found bool
		if found, globalSanitizers = removeFromList("undefined", globalSanitizers); found && s.All_undefined == nil {
			s.All_undefined = boolPtr(true)
		}

		if found, globalSanitizers = removeFromList("default-ub", globalSanitizers); found && s.Undefined == nil {
			s.Undefined = boolPtr(true)
		}

		if found, globalSanitizers = removeFromList("address", globalSanitizers); found && s.Address == nil {
			s.Address = boolPtr(true)
		}

		if found, globalSanitizers = removeFromList("thread", globalSanitizers); found && s.Thread == nil {
			s.Thread = boolPtr(true)
		}

		if found, globalSanitizers = removeFromList("coverage", globalSanitizers); found && s.Coverage == nil {
			s.Coverage = boolPtr(true)
		}

		if found, globalSanitizers = removeFromList("safe-stack", globalSanitizers); found && s.Safestack == nil {
			s.Safestack = boolPtr(true)
		}

		if found, globalSanitizers = removeFromList("cfi", globalSanitizers); found && s.Cfi == nil {
			s.Cfi = boolPtr(true)
		}

		if len(globalSanitizers) > 0 {
			ctx.ModuleErrorf("unknown global sanitizer option %s", globalSanitizers[0])
		}
	}

	if ctx.staticBinary() {
		s.Address = nil
		s.Coverage = nil
		s.Thread = nil
	}

	if Bool(s.All_undefined) {
		s.Undefined = nil
	}

	if !ctx.toolchain().Is64Bit() {
		// TSAN and SafeStack are not supported on 32-bit architectures
		s.Thread = nil
		s.Safestack = nil
		// TODO(ccross): error for compile_multilib = "32"?
	}

	if Bool(s.All_undefined) || Bool(s.Undefined) || Bool(s.Address) ||
		Bool(s.Thread) || Bool(s.Coverage) || Bool(s.Safestack) || Bool(s.Cfi) {
		sanitize.Properties.SanitizerEnabled = true
	}

	if Bool(s.Coverage) {
		if !Bool(s.Address) {
			ctx.ModuleErrorf(`Use of "coverage" also requires "address"`)
		}
	}
}

func (sanitize *sanitize) deps(ctx BaseModuleContext, deps Deps) Deps {
	if !sanitize.Properties.SanitizerEnabled { // || c.static() {
		return deps
	}

	if ctx.Device() {
		if Bool(sanitize.Properties.Sanitize.Address) {
			deps.StaticLibs = append(deps.StaticLibs, asanLibs)
		}
		if Bool(sanitize.Properties.Sanitize.Address) || Bool(sanitize.Properties.Sanitize.Thread) {
			deps.SharedLibs = append(deps.SharedLibs, "libdl")
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
	var diagSanitizers []string

	if Bool(sanitize.Properties.Sanitize.All_undefined) {
		sanitizers = append(sanitizers, "undefined")
		if ctx.Device() {
			ctx.ModuleErrorf("ubsan is not yet supported on the device")
		}
	} else {
		if Bool(sanitize.Properties.Sanitize.Undefined) {
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

	if Bool(sanitize.Properties.Sanitize.Diag.Undefined) &&
		(Bool(sanitize.Properties.Sanitize.All_undefined) ||
			Bool(sanitize.Properties.Sanitize.Undefined) ||
			len(sanitize.Properties.Sanitize.Misc_undefined) > 0) {
		diagSanitizers = append(diagSanitizers, "undefined")
	}

	if Bool(sanitize.Properties.Sanitize.Address) {
		if ctx.Arch().ArchType == android.Arm {
			// Frame pointer based unwinder in ASan requires ARM frame setup.
			// TODO: put in flags?
			flags.RequiredInstructionSet = "arm"
		}
		flags.CFlags = append(flags.CFlags, asanCflags)
		flags.LdFlags = append(flags.LdFlags, asanLdflags)

		// ASan runtime library must be the first in the link order.
		runtimeLibrary := config.AddressSanitizerRuntimeLibrary(ctx.toolchain())
		if runtimeLibrary != "" {
			flags.libFlags = append([]string{"${config.ClangAsanLibDir}/" + runtimeLibrary}, flags.libFlags...)
		}
		if ctx.Host() {
			// -nodefaultlibs (provided with libc++) prevents the driver from linking
			// libraries needed with -fsanitize=address. http://b/18650275 (WAI)
			flags.LdFlags = append(flags.LdFlags, "-lm", "-lpthread")
			flags.LdFlags = append(flags.LdFlags, "-Wl,--no-as-needed")
			// Host ASAN only links symbols in the final executable, so
			// there will always be undefined symbols in intermediate libraries.
			_, flags.LdFlags = removeFromList("-Wl,--no-undefined", flags.LdFlags)
		} else {
			flags.CFlags = append(flags.CFlags, "-mllvm", "-asan-globals=0")
			flags.DynamicLinker = "/system/bin/linker_asan"
			if flags.Toolchain.Is64Bit() {
				flags.DynamicLinker += "64"
			}
		}
		sanitizers = append(sanitizers, "address")
		diagSanitizers = append(diagSanitizers, "address")
	}

	if Bool(sanitize.Properties.Sanitize.Coverage) {
		flags.CFlags = append(flags.CFlags, "-fsanitize-coverage=edge,indirect-calls,8bit-counters,trace-cmp")
	}

	if Bool(sanitize.Properties.Sanitize.Safestack) {
		sanitizers = append(sanitizers, "safe-stack")
	}

	if Bool(sanitize.Properties.Sanitize.Cfi) {
		sanitizers = append(sanitizers, "cfi")
		cfiFlags := []string{"-flto", "-fsanitize=cfi", "-fsanitize-cfi-cross-dso"}
		flags.CFlags = append(flags.CFlags, cfiFlags...)
		flags.CFlags = append(flags.CFlags, "-fvisibility=default")
		flags.LdFlags = append(flags.LdFlags, cfiFlags...)
		// FIXME: revert the __cfi_check flag when clang is updated to r280031.
		flags.LdFlags = append(flags.LdFlags, "-Wl,-plugin-opt,O1", "-Wl,-export-dynamic-symbol=__cfi_check")
		if Bool(sanitize.Properties.Sanitize.Diag.Cfi) {
			diagSanitizers = append(diagSanitizers, "cfi")
		}
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
			flags.CFlags = append(flags.CFlags, "-fsanitize-trap=all", "-ftrap-function=abort")
		}
	}

	if len(diagSanitizers) > 0 {
		flags.CFlags = append(flags.CFlags, "-fno-sanitize-trap="+strings.Join(diagSanitizers, ","))
	}
	// FIXME: enable RTTI if diag + (cfi or vptr)

	// Link a runtime library if needed.
	runtimeLibrary := ""
	if Bool(sanitize.Properties.Sanitize.Address) {
		runtimeLibrary = config.AddressSanitizerRuntimeLibrary(ctx.toolchain())
	} else if len(diagSanitizers) > 0 {
		runtimeLibrary = config.UndefinedBehaviorSanitizerRuntimeLibrary(ctx.toolchain())
	}

	// ASan runtime library must be the first in the link order.
	if runtimeLibrary != "" {
		flags.libFlags = append([]string{"${config.ClangAsanLibDir}/" + runtimeLibrary}, flags.libFlags...)
	}

	blacklist := android.OptionalPathForModuleSrc(ctx, sanitize.Properties.Sanitize.Blacklist)
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
		return Bool(sanitize.Properties.Sanitize.Address)
	case tsan:
		return Bool(sanitize.Properties.Sanitize.Thread)
	default:
		panic(fmt.Errorf("unknown sanitizerType %d", t))
	}
}

func (sanitize *sanitize) SetSanitizer(t sanitizerType, b bool) {
	switch t {
	case asan:
		sanitize.Properties.Sanitize.Address = boolPtr(b)
		if !b {
			sanitize.Properties.Sanitize.Coverage = nil
		}
	case tsan:
		sanitize.Properties.Sanitize.Thread = boolPtr(b)
	default:
		panic(fmt.Errorf("unknown sanitizerType %d", t))
	}
	if b {
		sanitize.Properties.SanitizerEnabled = true
	}
}

// Propagate asan requirements down from binaries
func sanitizerDepsMutator(t sanitizerType) func(android.TopDownMutatorContext) {
	return func(mctx android.TopDownMutatorContext) {
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
func sanitizerMutator(t sanitizerType) func(android.BottomUpMutatorContext) {
	return func(mctx android.BottomUpMutatorContext) {
		if c, ok := mctx.Module().(*Module); ok && c.sanitize != nil {
			if c.isDependencyRoot() && c.sanitize.Sanitizer(t) {
				modules := mctx.CreateVariations(t.String())
				modules[0].(*Module).sanitize.SetSanitizer(t, true)
			} else if c.sanitize.Properties.SanitizeDep {
				modules := mctx.CreateVariations("", t.String())
				modules[0].(*Module).sanitize.SetSanitizer(t, false)
				modules[1].(*Module).sanitize.SetSanitizer(t, true)
				modules[0].(*Module).sanitize.Properties.SanitizeDep = false
				modules[1].(*Module).sanitize.Properties.SanitizeDep = false
				if mctx.Device() {
					modules[1].(*Module).sanitize.Properties.InData = true
				} else {
					modules[0].(*Module).Properties.PreventInstall = true
				}
				if mctx.AConfig().EmbeddedInMake() {
					modules[0].(*Module).Properties.HideFromMake = true
				}
			}
			c.sanitize.Properties.SanitizeDep = false
		}
	}
}
