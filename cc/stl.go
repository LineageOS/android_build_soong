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

	"android/soong/android"
)

func getNdkStlFamily(m LinkableInterface) string {
	family, _ := getNdkStlFamilyAndLinkType(m)
	return family
}

func deduplicateStlInput(stl string) string {
	switch stl {
	case "c++_shared":
		return "libc++"
	case "c++_static":
		return "libc++_static"
	}
	return stl
}

func getNdkStlFamilyAndLinkType(m LinkableInterface) (string, string) {
	stl := m.SelectedStl()
	switch stl {
	case "ndk_libc++_shared", "libc++":
		return "libc++", "shared"
	case "ndk_libc++_static", "libc++_static":
		return "libc++", "static"
	case "ndk_system":
		return "system", "shared"
	case "":
		return "none", "none"
	default:
		panic(fmt.Errorf("stl: %q is not a valid STL", stl))
	}
}

type StlProperties struct {
	// Select the STL library to use.  Possible values are "libc++",
	// "libc++_static", "libstdc++", or "none". Leave blank to select the
	// default.
	Stl *string `android:"arch_variant"`

	SelectedStl string `blueprint:"mutated"`
}

type stl struct {
	Properties StlProperties
}

func (stl *stl) props() []interface{} {
	return []interface{}{&stl.Properties}
}

func (stl *stl) begin(ctx BaseModuleContext) {
	stl.Properties.SelectedStl = func() string {
		s := ""
		if stl.Properties.Stl != nil {
			s = *stl.Properties.Stl
		} else if ctx.header() {
			s = "none"
		}
		if s == "none" {
			return ""
		}
		s = deduplicateStlInput(s)
		if ctx.useSdk() && ctx.Device() {
			switch s {
			case "", "system":
				return "ndk_system"
			case "libc++":
				return "ndk_libc++_shared"
			case "libc++_static":
				return "ndk_libc++_static"
			default:
				ctx.ModuleErrorf("stl: %q is not a supported STL with sdk_version set", s)
				return ""
			}
		} else if ctx.Windows() {
			switch s {
			case "libc++", "libc++_static", "":
				// Only use static libc++ for Windows.
				return "libc++_static"
			default:
				ctx.ModuleErrorf("stl: %q is not a supported STL for windows", s)
				return ""
			}
		} else {
			switch s {
			case "libc++", "libc++_static":
				return s
			case "", "system":
				if ctx.static() {
					return "libc++_static"
				} else {
					return "libc++"
				}
			default:
				ctx.ModuleErrorf("stl: %q is not a supported STL", s)
				return ""
			}
		}
	}()
}

func staticUnwinder(ctx android.BaseModuleContext) string {
	vndkVersion := ctx.Module().(*Module).VndkVersion()

	// Modules using R vndk use different unwinder
	if vndkVersion == "30" {
		if ctx.Arch().ArchType == android.Arm {
			return "libunwind_llvm"
		} else {
			return "libgcc_stripped"
		}
	}

	return "libunwind"
}

func (stl *stl) deps(ctx BaseModuleContext, deps Deps) Deps {
	switch stl.Properties.SelectedStl {
	case "libstdc++":
		// Nothing
	case "libc++", "libc++_static":
		if stl.Properties.SelectedStl == "libc++" {
			deps.SharedLibs = append(deps.SharedLibs, stl.Properties.SelectedStl)
		} else {
			deps.StaticLibs = append(deps.StaticLibs, stl.Properties.SelectedStl)
		}
		if ctx.Device() && !ctx.useSdk() {
			// __cxa_demangle is not a part of libc++.so on the device since
			// it's large and most processes don't need it. Statically link
			// libc++demangle into every process so that users still have it if
			// needed, but the linker won't include this unless it is actually
			// called.
			// http://b/138245375
			deps.StaticLibs = append(deps.StaticLibs, "libc++demangle")
		}
		if ctx.toolchain().Bionic() {
			if ctx.staticBinary() {
				deps.StaticLibs = append(deps.StaticLibs, "libm", "libc", staticUnwinder(ctx))
			} else {
				deps.StaticUnwinderIfLegacy = true
			}
		}
	case "":
		// None or error.
		if ctx.toolchain().Bionic() && ctx.Module().Name() == "libc++" {
			deps.StaticUnwinderIfLegacy = true
		}
	case "ndk_system":
		// TODO: Make a system STL prebuilt for the NDK.
		// The system STL doesn't have a prebuilt (it uses the system's libstdc++), but it does have
		// its own includes. The includes are handled in CCBase.Flags().
		deps.SharedLibs = append([]string{"libstdc++"}, deps.SharedLibs...)
		deps.HeaderLibs = append([]string{"ndk_system"}, deps.HeaderLibs...)
	case "ndk_libc++_shared", "ndk_libc++_static":
		if stl.Properties.SelectedStl == "ndk_libc++_shared" {
			deps.SharedLibs = append(deps.SharedLibs, stl.Properties.SelectedStl)
		} else {
			deps.StaticLibs = append(deps.StaticLibs, stl.Properties.SelectedStl, "ndk_libc++abi")
		}
		deps.StaticLibs = append(deps.StaticLibs, "ndk_libunwind")
	default:
		panic(fmt.Errorf("Unknown stl: %q", stl.Properties.SelectedStl))
	}

	return deps
}

func (stl *stl) flags(ctx ModuleContext, flags Flags) Flags {
	switch stl.Properties.SelectedStl {
	case "libc++", "libc++_static":
		if ctx.Darwin() {
			// libc++'s headers are annotated with availability macros that
			// indicate which version of Mac OS was the first to ship with a
			// libc++ feature available in its *system's* libc++.dylib. We do
			// not use the system's library, but rather ship our own. As such,
			// these availability attributes are meaningless for us but cause
			// build breaks when we try to use code that would not be available
			// in the system's dylib.
			flags.Local.CppFlags = append(flags.Local.CppFlags,
				"-D_LIBCPP_DISABLE_AVAILABILITY")
		}

		if !ctx.toolchain().Bionic() {
			flags.Local.CppFlags = append(flags.Local.CppFlags, "-nostdinc++")
			flags.extraLibFlags = append(flags.extraLibFlags, "-nostdlib++")
			if ctx.Windows() {
				flags.Local.CppFlags = append(flags.Local.CppFlags,
					// These macros can also be defined by libc++'s __config
					// or __config_site headers so define them the same way
					// (i.e. to nothing). Disable visibility annotations since
					// we're using static libc++.
					"-D_LIBCPP_DISABLE_VISIBILITY_ANNOTATIONS=",
					"-D_LIBCXXABI_DISABLE_VISIBILITY_ANNOTATIONS=",
					// Use Win32 threads in libc++.
					"-D_LIBCPP_HAS_THREAD_API_WIN32=")
			}
		}
	case "libstdc++":
		// Nothing
	case "ndk_system":
		// Nothing: The exports of ndk_system will be added automatically to the local cflags
	case "ndk_libc++_shared", "ndk_libc++_static":
		if ctx.Arch().ArchType == android.Arm {
			// Make sure the _Unwind_XXX symbols are not re-exported.
			flags.Local.LdFlags = append(flags.Local.LdFlags, "-Wl,--exclude-libs,libunwind.a")
		}
	case "":
		// None or error.
		if !ctx.toolchain().Bionic() {
			flags.Local.CppFlags = append(flags.Local.CppFlags, "-nostdinc++")
			flags.extraLibFlags = append(flags.extraLibFlags, "-nostdlib++")
		}
	default:
		panic(fmt.Errorf("Unknown stl: %q", stl.Properties.SelectedStl))
	}

	return flags
}
