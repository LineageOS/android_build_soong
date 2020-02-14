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
	"sort"
	"strings"
	"sync"

	"github.com/google/blueprint"

	"android/soong/android"
	"android/soong/cc/config"
)

var (
	// Any C flags added by sanitizer which libTooling tools may not
	// understand also need to be added to ClangLibToolingUnknownCflags in
	// cc/config/clang.go

	asanCflags = []string{
		"-fno-omit-frame-pointer",
		"-fno-experimental-new-pass-manager",
	}
	asanLdflags = []string{"-Wl,-u,__asan_preinit"}

	hwasanCflags = []string{"-fno-omit-frame-pointer", "-Wno-frame-larger-than=",
		"-fsanitize-hwaddress-abi=platform",
		"-fno-experimental-new-pass-manager",
		// The following improves debug location information
		// availability at the cost of its accuracy. It increases
		// the likelihood of a stack variable's frame offset
		// to be recorded in the debug info, which is important
		// for the quality of hwasan reports. The downside is a
		// higher number of "optimized out" stack variables.
		// b/112437883.
		"-mllvm", "-instcombine-lower-dbg-declare=0",
	}

	cfiCflags = []string{"-flto", "-fsanitize-cfi-cross-dso",
		"-fsanitize-blacklist=external/compiler-rt/lib/cfi/cfi_blacklist.txt"}
	// -flto and -fvisibility are required by clang when -fsanitize=cfi is
	// used, but have no effect on assembly files
	cfiAsflags = []string{"-flto", "-fvisibility=default"}
	cfiLdflags = []string{"-flto", "-fsanitize-cfi-cross-dso", "-fsanitize=cfi",
		"-Wl,-plugin-opt,O1"}
	cfiExportsMapPath     = "build/soong/cc/config/cfi_exports.map"
	cfiStaticLibsMutex    sync.Mutex
	hwasanStaticLibsMutex sync.Mutex

	intOverflowCflags = []string{"-fsanitize-blacklist=build/soong/cc/config/integer_overflow_blacklist.txt"}

	minimalRuntimeFlags = []string{"-fsanitize-minimal-runtime", "-fno-sanitize-trap=integer,undefined",
		"-fno-sanitize-recover=integer,undefined"}
	hwasanGlobalOptions = []string{"heap_history_size=1023", "stack_history_size=512",
		"export_memory_stats=0", "max_malloc_fill_size=0"}
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
	hwasan
	tsan
	intOverflow
	cfi
	scs
	fuzzer
)

// Name of the sanitizer variation for this sanitizer type
func (t sanitizerType) variationName() string {
	switch t {
	case asan:
		return "asan"
	case hwasan:
		return "hwasan"
	case tsan:
		return "tsan"
	case intOverflow:
		return "intOverflow"
	case cfi:
		return "cfi"
	case scs:
		return "scs"
	case fuzzer:
		return "fuzzer"
	default:
		panic(fmt.Errorf("unknown sanitizerType %d", t))
	}
}

// This is the sanitizer names in SANITIZE_[TARGET|HOST]
func (t sanitizerType) name() string {
	switch t {
	case asan:
		return "address"
	case hwasan:
		return "hwaddress"
	case tsan:
		return "thread"
	case intOverflow:
		return "integer_overflow"
	case cfi:
		return "cfi"
	case scs:
		return "shadow-call-stack"
	case fuzzer:
		return "fuzzer"
	default:
		panic(fmt.Errorf("unknown sanitizerType %d", t))
	}
}

func (t sanitizerType) incompatibleWithCfi() bool {
	return t == asan || t == fuzzer || t == hwasan
}

type SanitizeProperties struct {
	// enable AddressSanitizer, ThreadSanitizer, or UndefinedBehaviorSanitizer
	Sanitize struct {
		Never *bool `android:"arch_variant"`

		// main sanitizers
		Address   *bool `android:"arch_variant"`
		Thread    *bool `android:"arch_variant"`
		Hwaddress *bool `android:"arch_variant"`

		// local sanitizers
		Undefined        *bool    `android:"arch_variant"`
		All_undefined    *bool    `android:"arch_variant"`
		Misc_undefined   []string `android:"arch_variant"`
		Fuzzer           *bool    `android:"arch_variant"`
		Safestack        *bool    `android:"arch_variant"`
		Cfi              *bool    `android:"arch_variant"`
		Integer_overflow *bool    `android:"arch_variant"`
		Scudo            *bool    `android:"arch_variant"`
		Scs              *bool    `android:"arch_variant"`

		// Sanitizers to run in the diagnostic mode (as opposed to the release mode).
		// Replaces abort() on error with a human-readable error message.
		// Address and Thread sanitizers always run in diagnostic mode.
		Diag struct {
			Undefined        *bool    `android:"arch_variant"`
			Cfi              *bool    `android:"arch_variant"`
			Integer_overflow *bool    `android:"arch_variant"`
			Misc_undefined   []string `android:"arch_variant"`
			No_recover       []string
		}

		// value to pass to -fsanitize-recover=
		Recover []string

		// value to pass to -fsanitize-blacklist
		Blacklist *string
	} `android:"arch_variant"`

	SanitizerEnabled  bool     `blueprint:"mutated"`
	SanitizeDep       bool     `blueprint:"mutated"`
	MinimalRuntimeDep bool     `blueprint:"mutated"`
	UbsanRuntimeDep   bool     `blueprint:"mutated"`
	InSanitizerDir    bool     `blueprint:"mutated"`
	Sanitizers        []string `blueprint:"mutated"`
	DiagSanitizers    []string `blueprint:"mutated"`
}

type sanitize struct {
	Properties SanitizeProperties
}

func init() {
	android.RegisterMakeVarsProvider(pctx, cfiMakeVarsProvider)
	android.RegisterMakeVarsProvider(pctx, hwasanMakeVarsProvider)
}

func (sanitize *sanitize) props() []interface{} {
	return []interface{}{&sanitize.Properties}
}

func (sanitize *sanitize) begin(ctx BaseModuleContext) {
	s := &sanitize.Properties.Sanitize

	// Don't apply sanitizers to NDK code.
	if ctx.useSdk() {
		s.Never = BoolPtr(true)
	}

	// Sanitizers do not work on Fuchsia yet.
	if ctx.Fuchsia() {
		s.Never = BoolPtr(true)
	}

	// Never always wins.
	if Bool(s.Never) {
		return
	}

	var globalSanitizers []string
	var globalSanitizersDiag []string

	if ctx.Host() {
		if !ctx.Windows() {
			globalSanitizers = ctx.Config().SanitizeHost()
		}
	} else {
		arches := ctx.Config().SanitizeDeviceArch()
		if len(arches) == 0 || inList(ctx.Arch().ArchType.Name, arches) {
			globalSanitizers = ctx.Config().SanitizeDevice()
			globalSanitizersDiag = ctx.Config().SanitizeDeviceDiag()
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

		if found, globalSanitizers = removeFromList("fuzzer", globalSanitizers); found && s.Fuzzer == nil {
			s.Fuzzer = boolPtr(true)
		}

		if found, globalSanitizers = removeFromList("safe-stack", globalSanitizers); found && s.Safestack == nil {
			s.Safestack = boolPtr(true)
		}

		if found, globalSanitizers = removeFromList("cfi", globalSanitizers); found && s.Cfi == nil {
			if !ctx.Config().CFIDisabledForPath(ctx.ModuleDir()) {
				s.Cfi = boolPtr(true)
			}
		}

		// Global integer_overflow builds do not support static libraries.
		if found, globalSanitizers = removeFromList("integer_overflow", globalSanitizers); found && s.Integer_overflow == nil {
			if !ctx.Config().IntegerOverflowDisabledForPath(ctx.ModuleDir()) && !ctx.static() {
				s.Integer_overflow = boolPtr(true)
			}
		}

		if found, globalSanitizers = removeFromList("scudo", globalSanitizers); found && s.Scudo == nil {
			s.Scudo = boolPtr(true)
		}

		if found, globalSanitizers = removeFromList("hwaddress", globalSanitizers); found && s.Hwaddress == nil {
			s.Hwaddress = boolPtr(true)
		}

		if len(globalSanitizers) > 0 {
			ctx.ModuleErrorf("unknown global sanitizer option %s", globalSanitizers[0])
		}

		// Global integer_overflow builds do not support static library diagnostics.
		if found, globalSanitizersDiag = removeFromList("integer_overflow", globalSanitizersDiag); found &&
			s.Diag.Integer_overflow == nil && Bool(s.Integer_overflow) && !ctx.static() {
			s.Diag.Integer_overflow = boolPtr(true)
		}

		if found, globalSanitizersDiag = removeFromList("cfi", globalSanitizersDiag); found &&
			s.Diag.Cfi == nil && Bool(s.Cfi) {
			s.Diag.Cfi = boolPtr(true)
		}

		if len(globalSanitizersDiag) > 0 {
			ctx.ModuleErrorf("unknown global sanitizer diagnostics option %s", globalSanitizersDiag[0])
		}
	}

	// Enable CFI for all components in the include paths (for Aarch64 only)
	if s.Cfi == nil && ctx.Config().CFIEnabledForPath(ctx.ModuleDir()) && ctx.Arch().ArchType == android.Arm64 {
		s.Cfi = boolPtr(true)
		if inList("cfi", ctx.Config().SanitizeDeviceDiag()) {
			s.Diag.Cfi = boolPtr(true)
		}
	}

	// CFI needs gold linker, and mips toolchain does not have one.
	if !ctx.Config().EnableCFI() || ctx.Arch().ArchType == android.Mips || ctx.Arch().ArchType == android.Mips64 {
		s.Cfi = nil
		s.Diag.Cfi = nil
	}

	// Also disable CFI for arm32 until b/35157333 is fixed.
	if ctx.Arch().ArchType == android.Arm {
		s.Cfi = nil
		s.Diag.Cfi = nil
	}

	// HWASan requires AArch64 hardware feature (top-byte-ignore).
	if ctx.Arch().ArchType != android.Arm64 {
		s.Hwaddress = nil
	}

	// SCS is only implemented on AArch64.
	if ctx.Arch().ArchType != android.Arm64 {
		s.Scs = nil
	}

	// Also disable CFI if ASAN is enabled.
	if Bool(s.Address) || Bool(s.Hwaddress) {
		s.Cfi = nil
		s.Diag.Cfi = nil
	}

	// Disable sanitizers that depend on the UBSan runtime for host builds.
	if ctx.Host() {
		s.Cfi = nil
		s.Diag.Cfi = nil
		s.Misc_undefined = nil
		s.Undefined = nil
		s.All_undefined = nil
		s.Integer_overflow = nil
	}

	// Also disable CFI for VNDK variants of components
	if ctx.isVndk() && ctx.useVndk() {
		s.Cfi = nil
		s.Diag.Cfi = nil
	}

	// Also disable CFI if building against snapshot.
	vndkVersion := ctx.DeviceConfig().VndkVersion()
	if ctx.useVndk() && vndkVersion != "current" && vndkVersion != "" {
		s.Cfi = nil
	}

	// HWASan ramdisk (which is built from recovery) goes over some bootloader limit.
	// Keep libc instrumented so that ramdisk / recovery can run hwasan-instrumented code if necessary.
	if (ctx.inRamdisk() || ctx.inRecovery()) && !strings.HasPrefix(ctx.ModuleDir(), "bionic/libc") {
		s.Hwaddress = nil
	}

	if ctx.staticBinary() {
		s.Address = nil
		s.Fuzzer = nil
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

	if ctx.Os() != android.Windows && (Bool(s.All_undefined) || Bool(s.Undefined) || Bool(s.Address) || Bool(s.Thread) ||
		Bool(s.Fuzzer) || Bool(s.Safestack) || Bool(s.Cfi) || Bool(s.Integer_overflow) || len(s.Misc_undefined) > 0 ||
		Bool(s.Scudo) || Bool(s.Hwaddress) || Bool(s.Scs)) {
		sanitize.Properties.SanitizerEnabled = true
	}

	// Disable Scudo if ASan or TSan is enabled, or if it's disabled globally.
	if Bool(s.Address) || Bool(s.Thread) || Bool(s.Hwaddress) || ctx.Config().DisableScudo() {
		s.Scudo = nil
	}

	if Bool(s.Hwaddress) {
		s.Address = nil
		s.Thread = nil
	}

	// TODO(b/131771163): CFI transiently depends on LTO, and thus Fuzzer is
	// mutually incompatible.
	if Bool(s.Fuzzer) {
		s.Cfi = nil
	}
}

func (sanitize *sanitize) deps(ctx BaseModuleContext, deps Deps) Deps {
	if !sanitize.Properties.SanitizerEnabled { // || c.static() {
		return deps
	}

	if ctx.Device() {
		if Bool(sanitize.Properties.Sanitize.Address) {
			// Compiling asan and having libc_scudo in the same
			// executable will cause the executable to crash.
			// Remove libc_scudo since it is only used to override
			// allocation functions which asan already overrides.
			_, deps.SharedLibs = removeFromList("libc_scudo", deps.SharedLibs)
		}
	}

	return deps
}

func toDisableImplicitIntegerChange(flags []string) bool {
	// Returns true if any flag is fsanitize*integer, and there is
	// no explicit flag about sanitize=implicit-integer-sign-change.
	for _, f := range flags {
		if strings.Contains(f, "sanitize=implicit-integer-sign-change") {
			return false
		}
	}
	for _, f := range flags {
		if strings.HasPrefix(f, "-fsanitize") && strings.Contains(f, "integer") {
			return true
		}
	}
	return false
}

func (sanitize *sanitize) flags(ctx ModuleContext, flags Flags) Flags {
	minimalRuntimeLib := config.UndefinedBehaviorSanitizerMinimalRuntimeLibrary(ctx.toolchain()) + ".a"
	minimalRuntimePath := "${config.ClangAsanLibDir}/" + minimalRuntimeLib

	if ctx.Device() && sanitize.Properties.MinimalRuntimeDep {
		flags.Local.LdFlags = append(flags.Local.LdFlags,
			minimalRuntimePath,
			"-Wl,--exclude-libs,"+minimalRuntimeLib)
	}
	if !sanitize.Properties.SanitizerEnabled && !sanitize.Properties.UbsanRuntimeDep {
		return flags
	}

	if Bool(sanitize.Properties.Sanitize.Address) {
		if ctx.Arch().ArchType == android.Arm {
			// Frame pointer based unwinder in ASan requires ARM frame setup.
			// TODO: put in flags?
			flags.RequiredInstructionSet = "arm"
		}
		flags.Local.CFlags = append(flags.Local.CFlags, asanCflags...)
		flags.Local.LdFlags = append(flags.Local.LdFlags, asanLdflags...)

		if ctx.Host() {
			// -nodefaultlibs (provided with libc++) prevents the driver from linking
			// libraries needed with -fsanitize=address. http://b/18650275 (WAI)
			flags.Local.LdFlags = append(flags.Local.LdFlags, "-Wl,--no-as-needed")
		} else {
			flags.Local.CFlags = append(flags.Local.CFlags, "-mllvm", "-asan-globals=0")
			if ctx.bootstrap() {
				flags.DynamicLinker = "/system/bin/bootstrap/linker_asan"
			} else {
				flags.DynamicLinker = "/system/bin/linker_asan"
			}
			if flags.Toolchain.Is64Bit() {
				flags.DynamicLinker += "64"
			}
		}
	}

	if Bool(sanitize.Properties.Sanitize.Hwaddress) {
		flags.Local.CFlags = append(flags.Local.CFlags, hwasanCflags...)
	}

	if Bool(sanitize.Properties.Sanitize.Fuzzer) {
		flags.Local.CFlags = append(flags.Local.CFlags, "-fsanitize=fuzzer-no-link")

		// TODO(b/131771163): LTO and Fuzzer support is mutually incompatible.
		_, flags.Local.LdFlags = removeFromList("-flto", flags.Local.LdFlags)
		_, flags.Local.CFlags = removeFromList("-flto", flags.Local.CFlags)
		flags.Local.LdFlags = append(flags.Local.LdFlags, "-fno-lto")
		flags.Local.CFlags = append(flags.Local.CFlags, "-fno-lto")

		// TODO(b/142430592): Upstream linker scripts for sanitizer runtime libraries
		// discard the sancov_lowest_stack symbol, because it's emulated TLS (and thus
		// doesn't match the linker script due to the "__emutls_v." prefix).
		flags.Local.LdFlags = append(flags.Local.LdFlags, "-fno-sanitize-coverage=stack-depth")
		flags.Local.CFlags = append(flags.Local.CFlags, "-fno-sanitize-coverage=stack-depth")

		// TODO(b/133876586): Experimental PM breaks sanitizer coverage.
		flags.Local.CFlags = append(flags.Local.CFlags, "-fno-experimental-new-pass-manager")

		// Disable fortify for fuzzing builds. Generally, we'll be building with
		// UBSan or ASan here and the fortify checks pollute the stack traces.
		flags.Local.CFlags = append(flags.Local.CFlags, "-U_FORTIFY_SOURCE")

		// Build fuzzer-sanitized libraries with an $ORIGIN DT_RUNPATH. Android's
		// linker uses DT_RUNPATH, not DT_RPATH. When we deploy cc_fuzz targets and
		// their libraries to /data/fuzz/<arch>/lib, any transient shared library gets
		// the DT_RUNPATH from the shared library above it, and not the executable,
		// meaning that the lookup falls back to the system. Adding the $ORIGIN to the
		// DT_RUNPATH here means that transient shared libraries can be found
		// colocated with their parents.
		flags.Local.LdFlags = append(flags.Local.LdFlags, `-Wl,-rpath,\$$ORIGIN`)
	}

	if Bool(sanitize.Properties.Sanitize.Cfi) {
		if ctx.Arch().ArchType == android.Arm {
			// __cfi_check needs to be built as Thumb (see the code in linker_cfi.cpp). LLVM is not set up
			// to do this on a function basis, so force Thumb on the entire module.
			flags.RequiredInstructionSet = "thumb"
		}

		flags.Local.CFlags = append(flags.Local.CFlags, cfiCflags...)
		flags.Local.AsFlags = append(flags.Local.AsFlags, cfiAsflags...)
		// Only append the default visibility flag if -fvisibility has not already been set
		// to hidden.
		if !inList("-fvisibility=hidden", flags.Local.CFlags) {
			flags.Local.CFlags = append(flags.Local.CFlags, "-fvisibility=default")
		}
		flags.Local.LdFlags = append(flags.Local.LdFlags, cfiLdflags...)

		if ctx.staticBinary() {
			_, flags.Local.CFlags = removeFromList("-fsanitize-cfi-cross-dso", flags.Local.CFlags)
			_, flags.Local.LdFlags = removeFromList("-fsanitize-cfi-cross-dso", flags.Local.LdFlags)
		}
	}

	if Bool(sanitize.Properties.Sanitize.Integer_overflow) {
		flags.Local.CFlags = append(flags.Local.CFlags, intOverflowCflags...)
	}

	if len(sanitize.Properties.Sanitizers) > 0 {
		sanitizeArg := "-fsanitize=" + strings.Join(sanitize.Properties.Sanitizers, ",")

		flags.Local.CFlags = append(flags.Local.CFlags, sanitizeArg)
		flags.Local.AsFlags = append(flags.Local.AsFlags, sanitizeArg)
		if ctx.Host() {
			// Host sanitizers only link symbols in the final executable, so
			// there will always be undefined symbols in intermediate libraries.
			_, flags.Global.LdFlags = removeFromList("-Wl,--no-undefined", flags.Global.LdFlags)
			flags.Local.LdFlags = append(flags.Local.LdFlags, sanitizeArg)
		} else {
			if enableMinimalRuntime(sanitize) {
				flags.Local.CFlags = append(flags.Local.CFlags, strings.Join(minimalRuntimeFlags, " "))
				flags.libFlags = append([]string{minimalRuntimePath}, flags.libFlags...)
				flags.Local.LdFlags = append(flags.Local.LdFlags, "-Wl,--exclude-libs,"+minimalRuntimeLib)
			}
		}

		if Bool(sanitize.Properties.Sanitize.Fuzzer) {
			// When fuzzing, we wish to crash with diagnostics on any bug.
			flags.Local.CFlags = append(flags.Local.CFlags, "-fno-sanitize-trap=all", "-fno-sanitize-recover=all")
		} else if ctx.Host() {
			flags.Local.CFlags = append(flags.Local.CFlags, "-fno-sanitize-recover=all")
		} else {
			flags.Local.CFlags = append(flags.Local.CFlags, "-fsanitize-trap=all", "-ftrap-function=abort")
		}
		// http://b/119329758, Android core does not boot up with this sanitizer yet.
		if toDisableImplicitIntegerChange(flags.Local.CFlags) {
			flags.Local.CFlags = append(flags.Local.CFlags, "-fno-sanitize=implicit-integer-sign-change")
		}
	}

	if len(sanitize.Properties.DiagSanitizers) > 0 {
		flags.Local.CFlags = append(flags.Local.CFlags, "-fno-sanitize-trap="+strings.Join(sanitize.Properties.DiagSanitizers, ","))
	}
	// FIXME: enable RTTI if diag + (cfi or vptr)

	if sanitize.Properties.Sanitize.Recover != nil {
		flags.Local.CFlags = append(flags.Local.CFlags, "-fsanitize-recover="+
			strings.Join(sanitize.Properties.Sanitize.Recover, ","))
	}

	if sanitize.Properties.Sanitize.Diag.No_recover != nil {
		flags.Local.CFlags = append(flags.Local.CFlags, "-fno-sanitize-recover="+
			strings.Join(sanitize.Properties.Sanitize.Diag.No_recover, ","))
	}

	blacklist := android.OptionalPathForModuleSrc(ctx, sanitize.Properties.Sanitize.Blacklist)
	if blacklist.Valid() {
		flags.Local.CFlags = append(flags.Local.CFlags, "-fsanitize-blacklist="+blacklist.String())
		flags.CFlagsDeps = append(flags.CFlagsDeps, blacklist.Path())
	}

	return flags
}

func (sanitize *sanitize) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkData) {
	// Add a suffix for cfi/hwasan/scs-enabled static/header libraries to allow surfacing
	// both the sanitized and non-sanitized variants to make without a name conflict.
	if ret.Class == "STATIC_LIBRARIES" || ret.Class == "HEADER_LIBRARIES" {
		if Bool(sanitize.Properties.Sanitize.Cfi) {
			ret.SubName += ".cfi"
		}
		if Bool(sanitize.Properties.Sanitize.Hwaddress) {
			ret.SubName += ".hwasan"
		}
		if Bool(sanitize.Properties.Sanitize.Scs) {
			ret.SubName += ".scs"
		}
	}
}

func (sanitize *sanitize) inSanitizerDir() bool {
	return sanitize.Properties.InSanitizerDir
}

func (sanitize *sanitize) getSanitizerBoolPtr(t sanitizerType) *bool {
	switch t {
	case asan:
		return sanitize.Properties.Sanitize.Address
	case hwasan:
		return sanitize.Properties.Sanitize.Hwaddress
	case tsan:
		return sanitize.Properties.Sanitize.Thread
	case intOverflow:
		return sanitize.Properties.Sanitize.Integer_overflow
	case cfi:
		return sanitize.Properties.Sanitize.Cfi
	case scs:
		return sanitize.Properties.Sanitize.Scs
	case fuzzer:
		return sanitize.Properties.Sanitize.Fuzzer
	default:
		panic(fmt.Errorf("unknown sanitizerType %d", t))
	}
}

func (sanitize *sanitize) isUnsanitizedVariant() bool {
	return !sanitize.isSanitizerEnabled(asan) &&
		!sanitize.isSanitizerEnabled(hwasan) &&
		!sanitize.isSanitizerEnabled(tsan) &&
		!sanitize.isSanitizerEnabled(cfi) &&
		!sanitize.isSanitizerEnabled(scs) &&
		!sanitize.isSanitizerEnabled(fuzzer)
}

func (sanitize *sanitize) isVariantOnProductionDevice() bool {
	return !sanitize.isSanitizerEnabled(asan) &&
		!sanitize.isSanitizerEnabled(hwasan) &&
		!sanitize.isSanitizerEnabled(tsan) &&
		!sanitize.isSanitizerEnabled(fuzzer)
}

func (sanitize *sanitize) SetSanitizer(t sanitizerType, b bool) {
	switch t {
	case asan:
		sanitize.Properties.Sanitize.Address = boolPtr(b)
	case hwasan:
		sanitize.Properties.Sanitize.Hwaddress = boolPtr(b)
	case tsan:
		sanitize.Properties.Sanitize.Thread = boolPtr(b)
	case intOverflow:
		sanitize.Properties.Sanitize.Integer_overflow = boolPtr(b)
	case cfi:
		sanitize.Properties.Sanitize.Cfi = boolPtr(b)
	case scs:
		sanitize.Properties.Sanitize.Scs = boolPtr(b)
	case fuzzer:
		sanitize.Properties.Sanitize.Fuzzer = boolPtr(b)
	default:
		panic(fmt.Errorf("unknown sanitizerType %d", t))
	}
	if b {
		sanitize.Properties.SanitizerEnabled = true
	}
}

// Check if the sanitizer is explicitly disabled (as opposed to nil by
// virtue of not being set).
func (sanitize *sanitize) isSanitizerExplicitlyDisabled(t sanitizerType) bool {
	if sanitize == nil {
		return false
	}

	sanitizerVal := sanitize.getSanitizerBoolPtr(t)
	return sanitizerVal != nil && *sanitizerVal == false
}

// There isn't an analog of the method above (ie:isSanitizerExplicitlyEnabled)
// because enabling a sanitizer either directly (via the blueprint) or
// indirectly (via a mutator) sets the bool ptr to true, and you can't
// distinguish between the cases. It isn't needed though - both cases can be
// treated identically.
func (sanitize *sanitize) isSanitizerEnabled(t sanitizerType) bool {
	if sanitize == nil {
		return false
	}

	sanitizerVal := sanitize.getSanitizerBoolPtr(t)
	return sanitizerVal != nil && *sanitizerVal == true
}

func isSanitizableDependencyTag(tag blueprint.DependencyTag) bool {
	t, ok := tag.(DependencyTag)
	return ok && t.Library || t == reuseObjTag || t == objDepTag
}

// Propagate sanitizer requirements down from binaries
func sanitizerDepsMutator(t sanitizerType) func(android.TopDownMutatorContext) {
	return func(mctx android.TopDownMutatorContext) {
		if c, ok := mctx.Module().(*Module); ok && c.sanitize.isSanitizerEnabled(t) {
			mctx.WalkDeps(func(child, parent android.Module) bool {
				if !isSanitizableDependencyTag(mctx.OtherModuleDependencyTag(child)) {
					return false
				}
				if d, ok := child.(*Module); ok && d.sanitize != nil &&
					!Bool(d.sanitize.Properties.Sanitize.Never) &&
					!d.sanitize.isSanitizerExplicitlyDisabled(t) {
					if t == cfi || t == hwasan || t == scs {
						if d.static() {
							d.sanitize.Properties.SanitizeDep = true
						}
					} else {
						d.sanitize.Properties.SanitizeDep = true
					}
				}
				return true
			})
		} else if sanitizeable, ok := mctx.Module().(Sanitizeable); ok {
			// If an APEX module includes a lib which is enabled for a sanitizer T, then
			// the APEX module is also enabled for the same sanitizer type.
			mctx.VisitDirectDeps(func(child android.Module) {
				if c, ok := child.(*Module); ok && c.sanitize.isSanitizerEnabled(t) {
					sanitizeable.EnableSanitizer(t.name())
				}
			})
		}
	}
}

// Propagate the ubsan minimal runtime dependency when there are integer overflow sanitized static dependencies.
func sanitizerRuntimeDepsMutator(mctx android.TopDownMutatorContext) {
	if c, ok := mctx.Module().(*Module); ok && c.sanitize != nil {
		mctx.WalkDeps(func(child, parent android.Module) bool {
			if !isSanitizableDependencyTag(mctx.OtherModuleDependencyTag(child)) {
				return false
			}

			d, ok := child.(*Module)
			if !ok || !d.static() {
				return false
			}
			if d.sanitize != nil {
				if enableMinimalRuntime(d.sanitize) {
					// If a static dependency is built with the minimal runtime,
					// make sure we include the ubsan minimal runtime.
					c.sanitize.Properties.MinimalRuntimeDep = true
				} else if enableUbsanRuntime(d.sanitize) {
					// If a static dependency runs with full ubsan diagnostics,
					// make sure we include the ubsan runtime.
					c.sanitize.Properties.UbsanRuntimeDep = true
				}

				if c.sanitize.Properties.MinimalRuntimeDep &&
					c.sanitize.Properties.UbsanRuntimeDep {
					// both flags that this mutator might set are true, so don't bother recursing
					return false
				}

				return true
			}

			if p, ok := d.linker.(*vendorSnapshotLibraryDecorator); ok {
				if Bool(p.properties.Sanitize_minimal_dep) {
					c.sanitize.Properties.MinimalRuntimeDep = true
				}
				if Bool(p.properties.Sanitize_ubsan_dep) {
					c.sanitize.Properties.UbsanRuntimeDep = true
				}
			}

			return false
		})
	}
}

// Add the dependency to the runtime library for each of the sanitizer variants
func sanitizerRuntimeMutator(mctx android.BottomUpMutatorContext) {
	if c, ok := mctx.Module().(*Module); ok && c.sanitize != nil {
		if !c.Enabled() {
			return
		}
		var sanitizers []string
		var diagSanitizers []string

		if Bool(c.sanitize.Properties.Sanitize.All_undefined) {
			sanitizers = append(sanitizers, "undefined")
		} else {
			if Bool(c.sanitize.Properties.Sanitize.Undefined) {
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
			sanitizers = append(sanitizers, c.sanitize.Properties.Sanitize.Misc_undefined...)
		}

		if Bool(c.sanitize.Properties.Sanitize.Diag.Undefined) {
			diagSanitizers = append(diagSanitizers, "undefined")
		}

		diagSanitizers = append(diagSanitizers, c.sanitize.Properties.Sanitize.Diag.Misc_undefined...)

		if Bool(c.sanitize.Properties.Sanitize.Address) {
			sanitizers = append(sanitizers, "address")
			diagSanitizers = append(diagSanitizers, "address")
		}

		if Bool(c.sanitize.Properties.Sanitize.Hwaddress) {
			sanitizers = append(sanitizers, "hwaddress")
		}

		if Bool(c.sanitize.Properties.Sanitize.Thread) {
			sanitizers = append(sanitizers, "thread")
		}

		if Bool(c.sanitize.Properties.Sanitize.Safestack) {
			sanitizers = append(sanitizers, "safe-stack")
		}

		if Bool(c.sanitize.Properties.Sanitize.Cfi) {
			sanitizers = append(sanitizers, "cfi")

			if Bool(c.sanitize.Properties.Sanitize.Diag.Cfi) {
				diagSanitizers = append(diagSanitizers, "cfi")
			}
		}

		if Bool(c.sanitize.Properties.Sanitize.Integer_overflow) {
			sanitizers = append(sanitizers, "unsigned-integer-overflow")
			sanitizers = append(sanitizers, "signed-integer-overflow")
			if Bool(c.sanitize.Properties.Sanitize.Diag.Integer_overflow) {
				diagSanitizers = append(diagSanitizers, "unsigned-integer-overflow")
				diagSanitizers = append(diagSanitizers, "signed-integer-overflow")
			}
		}

		if Bool(c.sanitize.Properties.Sanitize.Scudo) {
			sanitizers = append(sanitizers, "scudo")
		}

		if Bool(c.sanitize.Properties.Sanitize.Scs) {
			sanitizers = append(sanitizers, "shadow-call-stack")
		}

		if Bool(c.sanitize.Properties.Sanitize.Fuzzer) {
			sanitizers = append(sanitizers, "fuzzer-no-link")
		}

		// Save the list of sanitizers. These will be used again when generating
		// the build rules (for Cflags, etc.)
		c.sanitize.Properties.Sanitizers = sanitizers
		c.sanitize.Properties.DiagSanitizers = diagSanitizers

		// Determine the runtime library required
		runtimeLibrary := ""
		var extraStaticDeps []string
		toolchain := c.toolchain(mctx)
		if Bool(c.sanitize.Properties.Sanitize.Address) {
			runtimeLibrary = config.AddressSanitizerRuntimeLibrary(toolchain)
		} else if Bool(c.sanitize.Properties.Sanitize.Hwaddress) {
			if c.staticBinary() {
				runtimeLibrary = config.HWAddressSanitizerStaticLibrary(toolchain)
				extraStaticDeps = []string{"libdl"}
			} else {
				runtimeLibrary = config.HWAddressSanitizerRuntimeLibrary(toolchain)
			}
		} else if Bool(c.sanitize.Properties.Sanitize.Thread) {
			runtimeLibrary = config.ThreadSanitizerRuntimeLibrary(toolchain)
		} else if Bool(c.sanitize.Properties.Sanitize.Scudo) {
			if len(diagSanitizers) == 0 && !c.sanitize.Properties.UbsanRuntimeDep {
				runtimeLibrary = config.ScudoMinimalRuntimeLibrary(toolchain)
			} else {
				runtimeLibrary = config.ScudoRuntimeLibrary(toolchain)
			}
		} else if len(diagSanitizers) > 0 || c.sanitize.Properties.UbsanRuntimeDep ||
			Bool(c.sanitize.Properties.Sanitize.Fuzzer) {
			runtimeLibrary = config.UndefinedBehaviorSanitizerRuntimeLibrary(toolchain)
		}

		if mctx.Device() && runtimeLibrary != "" {
			if isLlndkLibrary(runtimeLibrary, mctx.Config()) && !c.static() && c.UseVndk() {
				runtimeLibrary = runtimeLibrary + llndkLibrarySuffix
			}

			// Adding dependency to the runtime library. We are using *FarVariation*
			// because the runtime libraries themselves are not mutated by sanitizer
			// mutators and thus don't have sanitizer variants whereas this module
			// has been already mutated.
			//
			// Note that by adding dependency with {static|shared}DepTag, the lib is
			// added to libFlags and LOCAL_SHARED_LIBRARIES by cc.Module
			if c.staticBinary() {
				deps := append(extraStaticDeps, runtimeLibrary)
				// If we're using snapshots and in vendor, redirect to snapshot whenever possible
				if c.VndkVersion() == mctx.DeviceConfig().VndkVersion() {
					snapshots := vendorSnapshotStaticLibs(mctx.Config())
					for idx, dep := range deps {
						if lib, ok := snapshots.get(dep, mctx.Arch().ArchType); ok {
							deps[idx] = lib
						}
					}
				}

				// static executable gets static runtime libs
				mctx.AddFarVariationDependencies(append(mctx.Target().Variations(), []blueprint.Variation{
					{Mutator: "link", Variation: "static"},
					c.ImageVariation(),
				}...), StaticDepTag, deps...)
			} else if !c.static() && !c.header() {
				// If we're using snapshots and in vendor, redirect to snapshot whenever possible
				if c.VndkVersion() == mctx.DeviceConfig().VndkVersion() {
					snapshots := vendorSnapshotSharedLibs(mctx.Config())
					if lib, ok := snapshots.get(runtimeLibrary, mctx.Arch().ArchType); ok {
						runtimeLibrary = lib
					}
				}

				// dynamic executable and shared libs get shared runtime libs
				mctx.AddFarVariationDependencies(append(mctx.Target().Variations(), []blueprint.Variation{
					{Mutator: "link", Variation: "shared"},
					c.ImageVariation(),
				}...), earlySharedDepTag, runtimeLibrary)
			}
			// static lib does not have dependency to the runtime library. The
			// dependency will be added to the executables or shared libs using
			// the static lib.
		}
	}
}

type Sanitizeable interface {
	android.Module
	IsSanitizerEnabled(ctx android.BaseModuleContext, sanitizerName string) bool
	EnableSanitizer(sanitizerName string)
}

// Create sanitized variants for modules that need them
func sanitizerMutator(t sanitizerType) func(android.BottomUpMutatorContext) {
	return func(mctx android.BottomUpMutatorContext) {
		if c, ok := mctx.Module().(*Module); ok && c.sanitize != nil {
			if c.isDependencyRoot() && c.sanitize.isSanitizerEnabled(t) {
				modules := mctx.CreateVariations(t.variationName())
				modules[0].(*Module).sanitize.SetSanitizer(t, true)
			} else if c.sanitize.isSanitizerEnabled(t) || c.sanitize.Properties.SanitizeDep {
				isSanitizerEnabled := c.sanitize.isSanitizerEnabled(t)
				if mctx.Device() && t.incompatibleWithCfi() {
					// TODO: Make sure that cfi mutator runs "after" any of the sanitizers that
					// are incompatible with cfi
					c.sanitize.SetSanitizer(cfi, false)
				}
				if c.static() || c.header() || t == asan || t == fuzzer {
					// Static and header libs are split into non-sanitized and sanitized variants.
					// Shared libs are not split. However, for asan and fuzzer, we split even for shared
					// libs because a library sanitized for asan/fuzzer can't be linked from a library
					// that isn't sanitized for asan/fuzzer.
					//
					// Note for defaultVariation: since we don't split for shared libs but for static/header
					// libs, it is possible for the sanitized variant of a static/header lib to depend
					// on non-sanitized variant of a shared lib. Such unfulfilled variation causes an
					// error when the module is split. defaultVariation is the name of the variation that
					// will be used when such a dangling dependency occurs during the split of the current
					// module. By setting it to the name of the sanitized variation, the dangling dependency
					// is redirected to the sanitized variant of the dependent module.
					defaultVariation := t.variationName()
					mctx.SetDefaultDependencyVariation(&defaultVariation)
					modules := mctx.CreateVariations("", t.variationName())
					modules[0].(*Module).sanitize.SetSanitizer(t, false)
					modules[1].(*Module).sanitize.SetSanitizer(t, true)
					modules[0].(*Module).sanitize.Properties.SanitizeDep = false
					modules[1].(*Module).sanitize.Properties.SanitizeDep = false

					// For cfi/scs/hwasan, we can export both sanitized and un-sanitized variants
					// to Make, because the sanitized version has a different suffix in name.
					// For other types of sanitizers, suppress the variation that is disabled.
					if t != cfi && t != scs && t != hwasan {
						if isSanitizerEnabled {
							modules[0].(*Module).Properties.PreventInstall = true
							modules[0].(*Module).Properties.HideFromMake = true
						} else {
							modules[1].(*Module).Properties.PreventInstall = true
							modules[1].(*Module).Properties.HideFromMake = true
						}
					}

					// Export the static lib name to make
					if c.static() {
						if t == cfi {
							appendStringSync(c.Name(), cfiStaticLibs(mctx.Config()), &cfiStaticLibsMutex)
						} else if t == hwasan {
							if c.UseVndk() {
								appendStringSync(c.Name(), hwasanVendorStaticLibs(mctx.Config()),
									&hwasanStaticLibsMutex)
							} else {
								appendStringSync(c.Name(), hwasanStaticLibs(mctx.Config()),
									&hwasanStaticLibsMutex)
							}
						}
					}
				} else {
					// Shared libs are not split. Only the sanitized variant is created.
					modules := mctx.CreateVariations(t.variationName())
					modules[0].(*Module).sanitize.SetSanitizer(t, true)
					modules[0].(*Module).sanitize.Properties.SanitizeDep = false

					// locate the asan libraries under /data/asan
					if mctx.Device() && t == asan && isSanitizerEnabled {
						modules[0].(*Module).sanitize.Properties.InSanitizerDir = true
					}
				}
			}
			c.sanitize.Properties.SanitizeDep = false
		} else if sanitizeable, ok := mctx.Module().(Sanitizeable); ok && sanitizeable.IsSanitizerEnabled(mctx, t.name()) {
			// APEX modules fall here
			mctx.CreateVariations(t.variationName())
		}
	}
}

var cfiStaticLibsKey = android.NewOnceKey("cfiStaticLibs")

func cfiStaticLibs(config android.Config) *[]string {
	return config.Once(cfiStaticLibsKey, func() interface{} {
		return &[]string{}
	}).(*[]string)
}

var hwasanStaticLibsKey = android.NewOnceKey("hwasanStaticLibs")

func hwasanStaticLibs(config android.Config) *[]string {
	return config.Once(hwasanStaticLibsKey, func() interface{} {
		return &[]string{}
	}).(*[]string)
}

var hwasanVendorStaticLibsKey = android.NewOnceKey("hwasanVendorStaticLibs")

func hwasanVendorStaticLibs(config android.Config) *[]string {
	return config.Once(hwasanVendorStaticLibsKey, func() interface{} {
		return &[]string{}
	}).(*[]string)
}

func appendStringSync(item string, list *[]string, mutex *sync.Mutex) {
	mutex.Lock()
	*list = append(*list, item)
	mutex.Unlock()
}

func enableMinimalRuntime(sanitize *sanitize) bool {
	if !Bool(sanitize.Properties.Sanitize.Address) &&
		!Bool(sanitize.Properties.Sanitize.Hwaddress) &&
		!Bool(sanitize.Properties.Sanitize.Fuzzer) &&
		(Bool(sanitize.Properties.Sanitize.Integer_overflow) ||
			len(sanitize.Properties.Sanitize.Misc_undefined) > 0) &&
		!(Bool(sanitize.Properties.Sanitize.Diag.Integer_overflow) ||
			Bool(sanitize.Properties.Sanitize.Diag.Cfi) ||
			len(sanitize.Properties.Sanitize.Diag.Misc_undefined) > 0) {
		return true
	}
	return false
}

func enableUbsanRuntime(sanitize *sanitize) bool {
	return Bool(sanitize.Properties.Sanitize.Diag.Integer_overflow) ||
		len(sanitize.Properties.Sanitize.Diag.Misc_undefined) > 0
}

func cfiMakeVarsProvider(ctx android.MakeVarsContext) {
	cfiStaticLibs := cfiStaticLibs(ctx.Config())
	sort.Strings(*cfiStaticLibs)
	ctx.Strict("SOONG_CFI_STATIC_LIBRARIES", strings.Join(*cfiStaticLibs, " "))
}

func hwasanMakeVarsProvider(ctx android.MakeVarsContext) {
	hwasanStaticLibs := hwasanStaticLibs(ctx.Config())
	sort.Strings(*hwasanStaticLibs)
	ctx.Strict("SOONG_HWASAN_STATIC_LIBRARIES", strings.Join(*hwasanStaticLibs, " "))

	hwasanVendorStaticLibs := hwasanVendorStaticLibs(ctx.Config())
	sort.Strings(*hwasanVendorStaticLibs)
	ctx.Strict("SOONG_HWASAN_VENDOR_STATIC_LIBRARIES", strings.Join(*hwasanVendorStaticLibs, " "))
}
