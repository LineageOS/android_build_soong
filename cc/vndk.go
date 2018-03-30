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
	"sort"
	"strings"
	"sync"

	"android/soong/android"
)

type VndkProperties struct {
	Vndk struct {
		// declared as a VNDK or VNDK-SP module. The vendor variant
		// will be installed in /system instead of /vendor partition.
		//
		// `vendor_vailable` must be explicitly set to either true or
		// false together with `vndk: {enabled: true}`.
		Enabled *bool

		// declared as a VNDK-SP module, which is a subset of VNDK.
		//
		// `vndk: { enabled: true }` must set together.
		//
		// All these modules are allowed to link to VNDK-SP or LL-NDK
		// modules only. Other dependency will cause link-type errors.
		//
		// If `support_system_process` is not set or set to false,
		// the module is VNDK-core and can link to other VNDK-core,
		// VNDK-SP or LL-NDK modules only.
		Support_system_process *bool

		// Extending another module
		Extends *string
	}
}

type vndkdep struct {
	Properties VndkProperties
}

func (vndk *vndkdep) props() []interface{} {
	return []interface{}{&vndk.Properties}
}

func (vndk *vndkdep) begin(ctx BaseModuleContext) {}

func (vndk *vndkdep) deps(ctx BaseModuleContext, deps Deps) Deps {
	return deps
}

func (vndk *vndkdep) isVndk() bool {
	return Bool(vndk.Properties.Vndk.Enabled)
}

func (vndk *vndkdep) isVndkSp() bool {
	return Bool(vndk.Properties.Vndk.Support_system_process)
}

func (vndk *vndkdep) isVndkExt() bool {
	return vndk.Properties.Vndk.Extends != nil
}

func (vndk *vndkdep) getVndkExtendsModuleName() string {
	return String(vndk.Properties.Vndk.Extends)
}

func (vndk *vndkdep) typeName() string {
	if !vndk.isVndk() {
		return "native:vendor"
	}
	if !vndk.isVndkExt() {
		if !vndk.isVndkSp() {
			return "native:vendor:vndk"
		}
		return "native:vendor:vndksp"
	}
	if !vndk.isVndkSp() {
		return "native:vendor:vndkext"
	}
	return "native:vendor:vndkspext"
}

func (vndk *vndkdep) vndkCheckLinkType(ctx android.ModuleContext, to *Module, tag dependencyTag) {
	if to.linker == nil {
		return
	}
	if !vndk.isVndk() {
		// Non-VNDK modules (those installed to /vendor) can't depend on modules marked with
		// vendor_available: false.
		violation := false
		if lib, ok := to.linker.(*llndkStubDecorator); ok && !Bool(lib.Properties.Vendor_available) {
			violation = true
		} else {
			if _, ok := to.linker.(libraryInterface); ok && to.VendorProperties.Vendor_available != nil && !Bool(to.VendorProperties.Vendor_available) {
				// Vendor_available == nil && !Bool(Vendor_available) should be okay since
				// it means a vendor-only library which is a valid dependency for non-VNDK
				// modules.
				violation = true
			}
		}
		if violation {
			ctx.ModuleErrorf("Vendor module that is not VNDK should not link to %q which is marked as `vendor_available: false`", to.Name())
		}
	}
	if lib, ok := to.linker.(*libraryDecorator); !ok || !lib.shared() {
		// Check only shared libraries.
		// Other (static and LL-NDK) libraries are allowed to link.
		return
	}
	if !to.Properties.UseVndk {
		ctx.ModuleErrorf("(%s) should not link to %q which is not a vendor-available library",
			vndk.typeName(), to.Name())
		return
	}
	if tag == vndkExtDepTag {
		// Ensure `extends: "name"` property refers a vndk module that has vendor_available
		// and has identical vndk properties.
		if to.vndkdep == nil || !to.vndkdep.isVndk() {
			ctx.ModuleErrorf("`extends` refers a non-vndk module %q", to.Name())
			return
		}
		if vndk.isVndkSp() != to.vndkdep.isVndkSp() {
			ctx.ModuleErrorf(
				"`extends` refers a module %q with mismatched support_system_process",
				to.Name())
			return
		}
		if !Bool(to.VendorProperties.Vendor_available) {
			ctx.ModuleErrorf(
				"`extends` refers module %q which does not have `vendor_available: true`",
				to.Name())
			return
		}
	}
	if to.vndkdep == nil {
		return
	}

	// Check the dependencies of VNDK shared libraries.
	if !vndkIsVndkDepAllowed(vndk, to.vndkdep) {
		ctx.ModuleErrorf("(%s) should not link to %q (%s)",
			vndk.typeName(), to.Name(), to.vndkdep.typeName())
		return
	}
}

func vndkIsVndkDepAllowed(from *vndkdep, to *vndkdep) bool {
	// Check the dependencies of VNDK, VNDK-Ext, VNDK-SP, VNDK-SP-Ext and vendor modules.
	if from.isVndkExt() {
		if from.isVndkSp() {
			// VNDK-SP-Ext may depend on VNDK-SP, VNDK-SP-Ext, or vendor libs (excluding
			// VNDK and VNDK-Ext).
			return to.isVndkSp() || !to.isVndk()
		}
		// VNDK-Ext may depend on VNDK, VNDK-Ext, VNDK-SP, VNDK-SP-Ext, or vendor libs.
		return true
	}
	if from.isVndk() {
		if to.isVndkExt() {
			// VNDK-core and VNDK-SP must not depend on VNDK extensions.
			return false
		}
		if from.isVndkSp() {
			// VNDK-SP must only depend on VNDK-SP.
			return to.isVndkSp()
		}
		// VNDK-core may depend on VNDK-core or VNDK-SP.
		return to.isVndk()
	}
	// Vendor modules may depend on VNDK, VNDK-Ext, VNDK-SP, VNDK-SP-Ext, or vendor libs.
	return true
}

var (
	vndkCoreLibraries    []string
	vndkSpLibraries      []string
	llndkLibraries       []string
	vndkPrivateLibraries []string
	vndkLibrariesLock    sync.Mutex
)

// gather list of vndk-core, vndk-sp, and ll-ndk libs
func vndkMutator(mctx android.BottomUpMutatorContext) {
	if m, ok := mctx.Module().(*Module); ok && m.Enabled() {
		if lib, ok := m.linker.(*llndkStubDecorator); ok {
			vndkLibrariesLock.Lock()
			defer vndkLibrariesLock.Unlock()
			name := strings.TrimSuffix(m.Name(), llndkLibrarySuffix)
			if !inList(name, llndkLibraries) {
				llndkLibraries = append(llndkLibraries, name)
				sort.Strings(llndkLibraries)
			}
			if !Bool(lib.Properties.Vendor_available) {
				if !inList(name, vndkPrivateLibraries) {
					vndkPrivateLibraries = append(vndkPrivateLibraries, name)
					sort.Strings(vndkPrivateLibraries)
				}
			}
		} else {
			lib, is_lib := m.linker.(*libraryDecorator)
			prebuilt_lib, is_prebuilt_lib := m.linker.(*prebuiltLibraryLinker)
			if (is_lib && lib.shared()) || (is_prebuilt_lib && prebuilt_lib.shared()) {
				name := strings.TrimPrefix(m.Name(), "prebuilt_")
				if m.vndkdep.isVndk() && !m.vndkdep.isVndkExt() {
					vndkLibrariesLock.Lock()
					defer vndkLibrariesLock.Unlock()
					if m.vndkdep.isVndkSp() {
						if !inList(name, vndkSpLibraries) {
							vndkSpLibraries = append(vndkSpLibraries, name)
							sort.Strings(vndkSpLibraries)
						}
					} else {
						if !inList(name, vndkCoreLibraries) {
							vndkCoreLibraries = append(vndkCoreLibraries, name)
							sort.Strings(vndkCoreLibraries)
						}
					}
					if !Bool(m.VendorProperties.Vendor_available) {
						if !inList(name, vndkPrivateLibraries) {
							vndkPrivateLibraries = append(vndkPrivateLibraries, name)
							sort.Strings(vndkPrivateLibraries)
						}
					}
				}
			}
		}
	}
}
