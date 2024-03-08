// Copyright 2020 The Android Open Source Project
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

package rust

import (
	"fmt"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/cc"
	"android/soong/rust/config"
)

// TODO: When Rust has sanitizer-parity with CC, deduplicate this struct
type SanitizeProperties struct {
	// enable AddressSanitizer, HWAddressSanitizer, and others.
	Sanitize struct {
		Address   *bool `android:"arch_variant"`
		Hwaddress *bool `android:"arch_variant"`

		// Memory-tagging, only available on arm64
		// if diag.memtag unset or false, enables async memory tagging
		Memtag_heap *bool `android:"arch_variant"`
		Fuzzer      *bool `android:"arch_variant"`
		Never       *bool `android:"arch_variant"`

		// Sanitizers to run in the diagnostic mode (as opposed to the release mode).
		// Replaces abort() on error with a human-readable error message.
		// Address and Thread sanitizers always run in diagnostic mode.
		Diag struct {
			// Memory-tagging, only available on arm64
			// requires sanitizer.memtag: true
			// if set, enables sync memory tagging
			Memtag_heap *bool `android:"arch_variant"`
		}
	}
	SanitizerEnabled bool `blueprint:"mutated"`

	// Used when we need to place libraries in their own directory, such as ASAN.
	InSanitizerDir bool `blueprint:"mutated"`
}

var fuzzerFlags = []string{
	"-C passes='sancov-module'",

	"--cfg fuzzing",
	"-C llvm-args=-sanitizer-coverage-level=3",
	"-C llvm-args=-sanitizer-coverage-trace-compares",
	"-C llvm-args=-sanitizer-coverage-inline-8bit-counters",
	"-C llvm-args=-sanitizer-coverage-pc-table",

	// See https://github.com/rust-fuzz/cargo-fuzz/pull/193
	"-C link-dead-code",

	// Sancov breaks with lto
	// TODO: Remove when https://bugs.llvm.org/show_bug.cgi?id=41734 is resolved and sancov-module works with LTO
	"-C lto=no",
}

var asanFlags = []string{
	"-Z sanitizer=address",
}

// See cc/sanitize.go's hwasanGlobalOptions for global hwasan options.
var hwasanFlags = []string{
	"-Z sanitizer=hwaddress",
	"-C target-feature=+tagged-globals",

	// Flags from cc/sanitize.go hwasanFlags
	"-C llvm-args=--aarch64-enable-global-isel-at-O=-1",
	"-C llvm-args=-fast-isel=false",
	"-C llvm-args=-instcombine-lower-dbg-declare=0",

	// Additional flags for HWASAN-ified Rust/C interop
	"-C llvm-args=--hwasan-with-ifunc",
}

func boolPtr(v bool) *bool {
	if v {
		return &v
	} else {
		return nil
	}
}

func init() {
}
func (sanitize *sanitize) props() []interface{} {
	return []interface{}{&sanitize.Properties}
}

func (sanitize *sanitize) begin(ctx BaseModuleContext) {
	s := &sanitize.Properties.Sanitize

	// Never always wins.
	if Bool(s.Never) {
		return
	}

	// rust_test targets default to SYNC MemTag unless explicitly set to ASYNC (via diag: {Memtag_heap}).
	if binary, ok := ctx.RustModule().compiler.(binaryInterface); ok && binary.testBinary() {
		if s.Memtag_heap == nil {
			s.Memtag_heap = proptools.BoolPtr(true)
		}
		if s.Diag.Memtag_heap == nil {
			s.Diag.Memtag_heap = proptools.BoolPtr(true)
		}
	}

	var globalSanitizers []string
	var globalSanitizersDiag []string

	if ctx.Host() {
		if !ctx.Windows() {
			globalSanitizers = ctx.Config().SanitizeHost()
		}
	} else {
		arches := ctx.Config().SanitizeDeviceArch()
		if len(arches) == 0 || android.InList(ctx.Arch().ArchType.Name, arches) {
			globalSanitizers = ctx.Config().SanitizeDevice()
			globalSanitizersDiag = ctx.Config().SanitizeDeviceDiag()
		}
	}

	if len(globalSanitizers) > 0 {
		var found bool

		// Global Sanitizers
		if found, globalSanitizers = android.RemoveFromList("hwaddress", globalSanitizers); found && s.Hwaddress == nil {
			// TODO(b/204776996): HWASan for static Rust binaries isn't supported yet.
			if !ctx.RustModule().StaticExecutable() {
				s.Hwaddress = proptools.BoolPtr(true)
			}
		}

		if found, globalSanitizers = android.RemoveFromList("memtag_heap", globalSanitizers); found && s.Memtag_heap == nil {
			if !ctx.Config().MemtagHeapDisabledForPath(ctx.ModuleDir()) {
				s.Memtag_heap = proptools.BoolPtr(true)
			}
		}

		if found, globalSanitizers = android.RemoveFromList("address", globalSanitizers); found && s.Address == nil {
			s.Address = proptools.BoolPtr(true)
		}

		if found, globalSanitizers = android.RemoveFromList("fuzzer", globalSanitizers); found && s.Fuzzer == nil {
			// TODO(b/204776996): HWASan for static Rust binaries isn't supported yet, and fuzzer enables HWAsan
			if !ctx.RustModule().StaticExecutable() {
				s.Fuzzer = proptools.BoolPtr(true)
			}
		}

		// Global Diag Sanitizers
		if found, globalSanitizersDiag = android.RemoveFromList("memtag_heap", globalSanitizersDiag); found &&
			s.Diag.Memtag_heap == nil && Bool(s.Memtag_heap) {
			s.Diag.Memtag_heap = proptools.BoolPtr(true)
		}
	}

	// Enable Memtag for all components in the include paths (for Aarch64 only)
	if ctx.Arch().ArchType == android.Arm64 && ctx.Os().Bionic() {
		if ctx.Config().MemtagHeapSyncEnabledForPath(ctx.ModuleDir()) {
			if s.Memtag_heap == nil {
				s.Memtag_heap = proptools.BoolPtr(true)
			}
			if s.Diag.Memtag_heap == nil {
				s.Diag.Memtag_heap = proptools.BoolPtr(true)
			}
		} else if ctx.Config().MemtagHeapAsyncEnabledForPath(ctx.ModuleDir()) {
			if s.Memtag_heap == nil {
				s.Memtag_heap = proptools.BoolPtr(true)
			}
		}
	}

	// HWASan requires AArch64 hardware feature (top-byte-ignore).
	if ctx.Arch().ArchType != android.Arm64 || !ctx.Os().Bionic() {
		s.Hwaddress = nil
	}

	// HWASan ramdisk (which is built from recovery) goes over some bootloader limit.
	// Keep libc instrumented so that ramdisk / vendor_ramdisk / recovery can run hwasan-instrumented code if necessary.
	if (ctx.RustModule().InRamdisk() || ctx.RustModule().InVendorRamdisk() || ctx.RustModule().InRecovery()) && !strings.HasPrefix(ctx.ModuleDir(), "bionic/libc") {
		s.Hwaddress = nil
	}

	if Bool(s.Hwaddress) {
		s.Address = nil
	}

	// TODO: Remove once b/304507701 is resolved
	if Bool(s.Address) && ctx.Host() {
		s.Address = nil
	}

	// Memtag_heap is only implemented on AArch64.
	if ctx.Arch().ArchType != android.Arm64 || !ctx.Os().Bionic() {
		s.Memtag_heap = nil
	}

	// Disable sanitizers for musl x86 modules, rustc does not support any sanitizers.
	if ctx.Os() == android.LinuxMusl && ctx.Arch().ArchType == android.X86 {
		s.Never = boolPtr(true)
	}

	// TODO:(b/178369775)
	// For now sanitizing is only supported on non-windows targets
	if ctx.Os() != android.Windows && (Bool(s.Hwaddress) || Bool(s.Address) || Bool(s.Memtag_heap) || Bool(s.Fuzzer)) {
		sanitize.Properties.SanitizerEnabled = true
	}
}

type sanitize struct {
	Properties SanitizeProperties
}

func (sanitize *sanitize) flags(ctx ModuleContext, flags Flags, deps PathDeps) (Flags, PathDeps) {
	if !sanitize.Properties.SanitizerEnabled {
		return flags, deps
	}

	if Bool(sanitize.Properties.Sanitize.Fuzzer) {
		flags.RustFlags = append(flags.RustFlags, fuzzerFlags...)
	}

	if Bool(sanitize.Properties.Sanitize.Hwaddress) {
		flags.RustFlags = append(flags.RustFlags, hwasanFlags...)
	}

	if Bool(sanitize.Properties.Sanitize.Address) {
		flags.RustFlags = append(flags.RustFlags, asanFlags...)
		if ctx.Host() {
			// -nodefaultlibs (provided with libc++) prevents the driver from linking
			// libraries needed with -fsanitize=address. http://b/18650275 (WAI)
			flags.LinkFlags = append(flags.LinkFlags, []string{"-Wl,--no-as-needed"}...)
		}
	}
	return flags, deps
}

func (sanitize *sanitize) deps(ctx BaseModuleContext, deps Deps) Deps {
	return deps
}

func rustSanitizerRuntimeMutator(mctx android.BottomUpMutatorContext) {
	if mod, ok := mctx.Module().(*Module); ok && mod.sanitize != nil {
		if !mod.Enabled() {
			return
		}

		if Bool(mod.sanitize.Properties.Sanitize.Memtag_heap) && mod.Binary() {
			noteDep := "note_memtag_heap_async"
			if Bool(mod.sanitize.Properties.Sanitize.Diag.Memtag_heap) {
				noteDep = "note_memtag_heap_sync"
			}
			// If we're using snapshots, redirect to snapshot whenever possible
			// TODO(b/178470649): clean manual snapshot redirections
			snapshot := mctx.Provider(cc.SnapshotInfoProvider).(cc.SnapshotInfo)
			if lib, ok := snapshot.StaticLibs[noteDep]; ok {
				noteDep = lib
			}
			depTag := cc.StaticDepTag(true)
			variations := append(mctx.Target().Variations(),
				blueprint.Variation{Mutator: "link", Variation: "static"})
			if mod.Device() {
				variations = append(variations, mod.ImageVariation())
			}
			mctx.AddFarVariationDependencies(variations, depTag, noteDep)
		}

		variations := mctx.Target().Variations()
		var depTag blueprint.DependencyTag
		var deps []string

		if mod.IsSanitizerEnabled(cc.Asan) {
			if mod.Host() {
				variations = append(variations,
					blueprint.Variation{Mutator: "link", Variation: "static"})
				depTag = cc.StaticDepTag(false)
				deps = []string{config.LibclangRuntimeLibrary(mod.toolchain(mctx), "asan.static")}
			} else {
				variations = append(variations,
					blueprint.Variation{Mutator: "link", Variation: "shared"})
				depTag = cc.SharedDepTag()
				deps = []string{config.LibclangRuntimeLibrary(mod.toolchain(mctx), "asan")}
			}
		} else if mod.IsSanitizerEnabled(cc.Hwasan) {
			// TODO(b/204776996): HWASan for static Rust binaries isn't supported yet.
			if binary, ok := mod.compiler.(binaryInterface); ok {
				if binary.staticallyLinked() {
					mctx.ModuleErrorf("HWASan is not supported for static Rust executables yet.")
				}
			}

			// Always link against the shared library -- static binaries will pull in the static
			// library during final link if necessary
			variations = append(variations,
				blueprint.Variation{Mutator: "link", Variation: "shared"})
			depTag = cc.SharedDepTag()
			deps = []string{config.LibclangRuntimeLibrary(mod.toolchain(mctx), "hwasan")}
		}

		if len(deps) > 0 {
			mctx.AddFarVariationDependencies(variations, depTag, deps...)
		}
	}
}

func (sanitize *sanitize) SetSanitizer(t cc.SanitizerType, b bool) {
	sanitizerSet := false
	switch t {
	case cc.Fuzzer:
		sanitize.Properties.Sanitize.Fuzzer = boolPtr(b)
		sanitizerSet = true
	case cc.Asan:
		sanitize.Properties.Sanitize.Address = boolPtr(b)
		sanitizerSet = true
	case cc.Hwasan:
		sanitize.Properties.Sanitize.Hwaddress = boolPtr(b)
		sanitizerSet = true
	case cc.Memtag_heap:
		sanitize.Properties.Sanitize.Memtag_heap = boolPtr(b)
		sanitizerSet = true
	default:
		panic(fmt.Errorf("setting unsupported sanitizerType %d", t))
	}
	if b && sanitizerSet {
		sanitize.Properties.SanitizerEnabled = true
	}
}

func (m *Module) UbsanRuntimeNeeded() bool {
	return false
}

func (m *Module) MinimalRuntimeNeeded() bool {
	return false
}

func (m *Module) UbsanRuntimeDep() bool {
	return false
}

func (m *Module) MinimalRuntimeDep() bool {
	return false
}

// Check if the sanitizer is explicitly disabled (as opposed to nil by
// virtue of not being set).
func (sanitize *sanitize) isSanitizerExplicitlyDisabled(t cc.SanitizerType) bool {
	if sanitize == nil {
		return false
	}
	if Bool(sanitize.Properties.Sanitize.Never) {
		return true
	}
	sanitizerVal := sanitize.getSanitizerBoolPtr(t)
	return sanitizerVal != nil && *sanitizerVal == false
}

// There isn't an analog of the method above (ie:isSanitizerExplicitlyEnabled)
// because enabling a sanitizer either directly (via the blueprint) or
// indirectly (via a mutator) sets the bool ptr to true, and you can't
// distinguish between the cases. It isn't needed though - both cases can be
// treated identically.
func (sanitize *sanitize) isSanitizerEnabled(t cc.SanitizerType) bool {
	if sanitize == nil || !sanitize.Properties.SanitizerEnabled {
		return false
	}

	sanitizerVal := sanitize.getSanitizerBoolPtr(t)
	return sanitizerVal != nil && *sanitizerVal == true
}

func (sanitize *sanitize) getSanitizerBoolPtr(t cc.SanitizerType) *bool {
	switch t {
	case cc.Fuzzer:
		return sanitize.Properties.Sanitize.Fuzzer
	case cc.Asan:
		return sanitize.Properties.Sanitize.Address
	case cc.Hwasan:
		return sanitize.Properties.Sanitize.Hwaddress
	case cc.Memtag_heap:
		return sanitize.Properties.Sanitize.Memtag_heap
	default:
		return nil
	}
}

func (sanitize *sanitize) AndroidMk(ctx AndroidMkContext, entries *android.AndroidMkEntries) {
	// Add a suffix for hwasan rlib libraries to allow surfacing both the sanitized and
	// non-sanitized variants to make without a name conflict.
	if entries.Class == "RLIB_LIBRARIES" || entries.Class == "STATIC_LIBRARIES" {
		if sanitize.isSanitizerEnabled(cc.Hwasan) {
			entries.SubName += ".hwasan"
		}
	}
}

func (mod *Module) SanitizerSupported(t cc.SanitizerType) bool {
	// Sanitizers are not supported on Windows targets.
	if mod.Os() == android.Windows {
		return false
	}
	switch t {
	case cc.Fuzzer:
		return true
	case cc.Asan:
		return true
	case cc.Hwasan:
		// TODO(b/180495975): HWASan for static Rust binaries isn't supported yet.
		if mod.StaticExecutable() {
			return false
		}
		return true
	case cc.Memtag_heap:
		return true
	default:
		return false
	}
}

func (mod *Module) IsSanitizerEnabled(t cc.SanitizerType) bool {
	return mod.sanitize.isSanitizerEnabled(t)
}

func (mod *Module) IsSanitizerExplicitlyDisabled(t cc.SanitizerType) bool {
	// Sanitizers are not supported on Windows targets.
	if mod.Os() == android.Windows {
		return true
	}

	return mod.sanitize.isSanitizerExplicitlyDisabled(t)
}

func (mod *Module) SetSanitizer(t cc.SanitizerType, b bool) {
	if !Bool(mod.sanitize.Properties.Sanitize.Never) {
		mod.sanitize.SetSanitizer(t, b)
	}
}

func (mod *Module) StaticallyLinked() bool {
	if lib, ok := mod.compiler.(libraryInterface); ok {
		return lib.rlib() || lib.static()
	} else if binary, ok := mod.compiler.(binaryInterface); ok {
		return binary.staticallyLinked()
	}
	return false
}

func (mod *Module) SetInSanitizerDir() {
	mod.sanitize.Properties.InSanitizerDir = true
}

func (mod *Module) SanitizeNever() bool {
	return Bool(mod.sanitize.Properties.Sanitize.Never)
}

var _ cc.PlatformSanitizeable = (*Module)(nil)

func IsSanitizableDependencyTag(tag blueprint.DependencyTag) bool {
	switch t := tag.(type) {
	case dependencyTag:
		return t.library
	default:
		return cc.IsSanitizableDependencyTag(tag)
	}
}

func (m *Module) SanitizableDepTagChecker() cc.SantizableDependencyTagChecker {
	return IsSanitizableDependencyTag
}
