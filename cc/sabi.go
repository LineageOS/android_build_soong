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
	"sync"

	"android/soong/android"
	"android/soong/cc/config"
)

var (
	lsdumpPaths     []string
	lsdumpPathsLock sync.Mutex
)

type SAbiProperties struct {
	// Whether ABI dump should be created for this module.
	// Set by `sabiDepsMutator` if this module is a shared library that needs ABI check, or a static
	// library that is depended on by an ABI checked library.
	ShouldCreateSourceAbiDump bool `blueprint:"mutated"`

	// Include directories that may contain ABI information exported by a library.
	// These directories are passed to the header-abi-dumper.
	ReexportedIncludes []string `blueprint:"mutated"`
}

type sabi struct {
	Properties SAbiProperties
}

func (sabi *sabi) props() []interface{} {
	return []interface{}{&sabi.Properties}
}

func (sabi *sabi) begin(ctx BaseModuleContext) {}

func (sabi *sabi) deps(ctx BaseModuleContext, deps Deps) Deps {
	return deps
}

func (sabi *sabi) flags(ctx ModuleContext, flags Flags) Flags {
	// Filter out flags which libTooling don't understand.
	// This is here for legacy reasons and future-proof, in case the version of libTooling and clang
	// diverge.
	flags.Local.ToolingCFlags = config.ClangLibToolingFilterUnknownCflags(flags.Local.CFlags)
	flags.Global.ToolingCFlags = config.ClangLibToolingFilterUnknownCflags(flags.Global.CFlags)
	flags.Local.ToolingCppFlags = config.ClangLibToolingFilterUnknownCflags(flags.Local.CppFlags)
	flags.Global.ToolingCppFlags = config.ClangLibToolingFilterUnknownCflags(flags.Global.CppFlags)
	return flags
}

// Returns true if ABI dump should be created for this library, either because library is ABI
// checked or is depended on by an ABI checked library.
// Could be called as a nil receiver.
func (sabi *sabi) shouldCreateSourceAbiDump() bool {
	return sabi != nil && sabi.Properties.ShouldCreateSourceAbiDump
}

// Returns a string that represents the class of the ABI dump.
// Returns an empty string if ABI check is disabled for this library.
func classifySourceAbiDump(ctx android.BaseModuleContext) string {
	m := ctx.Module().(*Module)
	if m.library.headerAbiCheckerExplicitlyDisabled() {
		return ""
	}
	// Return NDK if the library is both NDK and LLNDK.
	if m.IsNdk(ctx.Config()) {
		return "NDK"
	}
	if m.isImplementationForLLNDKPublic() {
		return "LLNDK"
	}
	if m.UseVndk() && m.IsVndk() && !m.IsVndkPrivate() {
		if m.IsVndkSp() {
			if m.IsVndkExt() {
				return "VNDK-SP-ext"
			} else {
				return "VNDK-SP"
			}
		} else {
			if m.IsVndkExt() {
				return "VNDK-ext"
			} else {
				return "VNDK-core"
			}
		}
	}
	if m.library.headerAbiCheckerEnabled() || m.library.hasStubsVariants() {
		return "PLATFORM"
	}
	return ""
}

// Called from sabiDepsMutator to check whether ABI dumps should be created for this module.
// ctx should be wrapping a native library type module.
func shouldCreateSourceAbiDumpForLibrary(ctx android.BaseModuleContext) bool {
	if ctx.Fuchsia() {
		return false
	}

	// Only generate ABI dump for device modules.
	if !ctx.Device() {
		return false
	}

	m := ctx.Module().(*Module)

	// Only create ABI dump for native library module types.
	if m.library == nil {
		return false
	}

	// Create ABI dump for static libraries only if they are dependencies of ABI checked libraries.
	if m.library.static() {
		return m.sabi.shouldCreateSourceAbiDump()
	}

	// Module is shared library type.

	// Don't check uninstallable modules.
	if m.IsHideFromMake() {
		return false
	}

	// Don't check ramdisk or recovery variants. Only check core, vendor or product variants.
	if m.InRamdisk() || m.InVendorRamdisk() || m.InRecovery() {
		return false
	}

	// Don't create ABI dump for prebuilts.
	if m.Prebuilt() != nil || m.IsSnapshotPrebuilt() {
		return false
	}

	// Coverage builds have extra symbols.
	if m.isCoverageVariant() {
		return false
	}

	// Some sanitizer variants may have different ABI.
	if m.sanitize != nil && !m.sanitize.isVariantOnProductionDevice() {
		return false
	}

	// Don't create ABI dump for stubs.
	if m.isNDKStubLibrary() || m.IsLlndk() || m.IsStubs() {
		return false
	}

	isPlatformVariant := ctx.Provider(android.ApexInfoProvider).(android.ApexInfo).IsForPlatform()
	if isPlatformVariant {
		// Bionic libraries that are installed to the bootstrap directory are not ABI checked.
		// Only the runtime APEX variants, which are the implementation libraries of bionic NDK stubs,
		// are checked.
		if InstallToBootstrap(m.BaseModuleName(), ctx.Config()) {
			return false
		}
	} else {
		// Don't create ABI dump if this library is for APEX but isn't exported.
		if !m.HasStubsVariants() {
			return false
		}
	}
	return classifySourceAbiDump(ctx) != ""
}

// Mark the direct and transitive dependencies of libraries that need ABI check, so that ABI dumps
// of their dependencies would be generated.
func sabiDepsMutator(mctx android.TopDownMutatorContext) {
	// Escape hatch to not check any ABI dump.
	if mctx.Config().IsEnvTrue("SKIP_ABI_CHECKS") {
		return
	}
	// Only create ABI dump for native shared libraries and their static library dependencies.
	if m, ok := mctx.Module().(*Module); ok && m.sabi != nil {
		if shouldCreateSourceAbiDumpForLibrary(mctx) {
			// Mark this module so that .sdump / .lsdump for this library can be generated.
			m.sabi.Properties.ShouldCreateSourceAbiDump = true
			// Mark all of its static library dependencies.
			mctx.VisitDirectDeps(func(child android.Module) {
				depTag := mctx.OtherModuleDependencyTag(child)
				if IsStaticDepTag(depTag) || depTag == reuseObjTag {
					if c, ok := child.(*Module); ok && c.sabi != nil {
						// Mark this module so that .sdump for this static library can be generated.
						c.sabi.Properties.ShouldCreateSourceAbiDump = true
					}
				}
			})
		}
	}
}

// Add an entry to the global list of lsdump. The list is exported to a Make variable by
// `cc.makeVarsProvider`.
func addLsdumpPath(lsdumpPath string) {
	lsdumpPathsLock.Lock()
	defer lsdumpPathsLock.Unlock()
	lsdumpPaths = append(lsdumpPaths, lsdumpPath)
}
