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
	"android/soong/android"
	"fmt"
	"strconv"
)

func getNdkStlFamily(m *Module) string {
	family, _ := getNdkStlFamilyAndLinkType(m)
	return family
}

func getNdkStlFamilyAndLinkType(m *Module) (string, string) {
	stl := m.stl.Properties.SelectedStl
	switch stl {
	case "ndk_libc++_shared":
		return "libc++", "shared"
	case "ndk_libc++_static":
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
		}
		if ctx.useSdk() && ctx.Device() {
			switch s {
			case "", "system":
				return "ndk_system"
			case "c++_shared", "c++_static":
				return "ndk_lib" + s
			case "libc++":
				return "ndk_libc++_shared"
			case "libc++_static":
				return "ndk_libc++_static"
			case "none":
				return ""
			default:
				ctx.ModuleErrorf("stl: %q is not a supported STL with sdk_version set", s)
				return ""
			}
		} else if ctx.Windows() {
			switch s {
			case "libc++", "libc++_static", "":
				// Only use static libc++ for Windows.
				return "libc++_static"
			case "none":
				return ""
			default:
				ctx.ModuleErrorf("stl: %q is not a supported STL for windows", s)
				return ""
			}
		} else if ctx.Fuchsia() {
			switch s {
			case "c++_static":
				return "libc++_static"
			case "c++_shared":
				return "libc++"
			case "libc++", "libc++_static":
				return s
			case "none":
				return ""
			case "":
				if ctx.static() {
					return "libc++_static"
				} else {
					return "libc++"
				}
			default:
				ctx.ModuleErrorf("stl: %q is not a supported STL on Fuchsia", s)
				return ""
			}
		} else {
			switch s {
			case "libc++", "libc++_static":
				return s
			case "none":
				return ""
			case "":
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

func needsLibAndroidSupport(ctx BaseModuleContext) bool {
	versionStr, err := normalizeNdkApiLevel(ctx, ctx.sdkVersion(), ctx.Arch())
	if err != nil {
		ctx.PropertyErrorf("sdk_version", err.Error())
	}

	if versionStr == "current" {
		return false
	}

	version, err := strconv.Atoi(versionStr)
	if err != nil {
		panic(fmt.Sprintf(
			"invalid API level returned from normalizeNdkApiLevel: %q",
			versionStr))
	}

	return version < 21
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
		if ctx.toolchain().Bionic() {
			if ctx.Arch().ArchType == android.Arm {
				deps.StaticLibs = append(deps.StaticLibs, "libunwind_llvm")
			}
			if ctx.staticBinary() {
				deps.StaticLibs = append(deps.StaticLibs, "libm", "libc", "libdl")
			}
		}
	case "":
		// None or error.
	case "ndk_system":
		// TODO: Make a system STL prebuilt for the NDK.
		// The system STL doesn't have a prebuilt (it uses the system's libstdc++), but it does have
		// its own includes. The includes are handled in CCBase.Flags().
		deps.SharedLibs = append([]string{"libstdc++"}, deps.SharedLibs...)
	case "ndk_libc++_shared", "ndk_libc++_static":
		if stl.Properties.SelectedStl == "ndk_libc++_shared" {
			deps.SharedLibs = append(deps.SharedLibs, stl.Properties.SelectedStl)
		} else {
			deps.StaticLibs = append(deps.StaticLibs, stl.Properties.SelectedStl, "ndk_libc++abi")
		}
		if needsLibAndroidSupport(ctx) {
			deps.StaticLibs = append(deps.StaticLibs, "ndk_libandroid_support")
		}
		if ctx.Arch().ArchType == android.Arm {
			deps.StaticLibs = append(deps.StaticLibs, "ndk_libunwind")
		}
	default:
		panic(fmt.Errorf("Unknown stl: %q", stl.Properties.SelectedStl))
	}

	return deps
}

func (stl *stl) flags(ctx ModuleContext, flags Flags) Flags {
	switch stl.Properties.SelectedStl {
	case "libc++", "libc++_static":
		flags.CFlags = append(flags.CFlags, "-D_USING_LIBCXX")

		if ctx.Darwin() {
			// libc++'s headers are annotated with availability macros that
			// indicate which version of Mac OS was the first to ship with a
			// libc++ feature available in its *system's* libc++.dylib. We do
			// not use the system's library, but rather ship our own. As such,
			// these availability attributes are meaningless for us but cause
			// build breaks when we try to use code that would not be available
			// in the system's dylib.
			flags.CppFlags = append(flags.CppFlags,
				"-D_LIBCPP_DISABLE_AVAILABILITY")
		}

		if !ctx.toolchain().Bionic() {
			flags.CppFlags = append(flags.CppFlags, "-nostdinc++")
			flags.LdFlags = append(flags.LdFlags, "-nodefaultlibs")
			if ctx.staticBinary() {
				flags.LdFlags = append(flags.LdFlags, hostStaticGccLibs[ctx.Os()]...)
			} else {
				flags.LdFlags = append(flags.LdFlags, hostDynamicGccLibs[ctx.Os()]...)
			}
			if ctx.Windows() {
				// Use SjLj exceptions for 32-bit.  libgcc_eh implements SjLj
				// exception model for 32-bit.
				if ctx.Arch().ArchType == android.X86 {
					flags.CppFlags = append(flags.CppFlags, "-fsjlj-exceptions")
				}
				flags.CppFlags = append(flags.CppFlags,
					// Disable visiblity annotations since we're using static
					// libc++.
					"-D_LIBCPP_DISABLE_VISIBILITY_ANNOTATIONS",
					"-D_LIBCXXABI_DISABLE_VISIBILITY_ANNOTATIONS",
					// Use Win32 threads in libc++.
					"-D_LIBCPP_HAS_THREAD_API_WIN32")
			}
		} else {
			if ctx.Arch().ArchType == android.Arm {
				flags.LdFlags = append(flags.LdFlags, "-Wl,--exclude-libs,libunwind_llvm.a")
			}
		}
	case "libstdc++":
		// Nothing
	case "ndk_system":
		ndkSrcRoot := android.PathForSource(ctx, "prebuilts/ndk/current/sources/cxx-stl/system/include")
		flags.CFlags = append(flags.CFlags, "-isystem "+ndkSrcRoot.String())
	case "ndk_libc++_shared", "ndk_libc++_static":
		// Nothing.
	case "":
		// None or error.
		if !ctx.toolchain().Bionic() {
			flags.CppFlags = append(flags.CppFlags, "-nostdinc++")
			flags.LdFlags = append(flags.LdFlags, "-nodefaultlibs")
			if ctx.staticBinary() {
				flags.LdFlags = append(flags.LdFlags, hostStaticGccLibs[ctx.Os()]...)
			} else {
				flags.LdFlags = append(flags.LdFlags, hostDynamicGccLibs[ctx.Os()]...)
			}
		}
	default:
		panic(fmt.Errorf("Unknown stl: %q", stl.Properties.SelectedStl))
	}

	return flags
}

var hostDynamicGccLibs, hostStaticGccLibs map[android.OsType][]string

func init() {
	hostDynamicGccLibs = map[android.OsType][]string{
		android.Fuchsia: []string{"-lc", "-lunwind"},
		android.Linux:   []string{"-lgcc_s", "-lgcc", "-lc", "-lgcc_s", "-lgcc"},
		android.Darwin:  []string{"-lc", "-lSystem"},
		android.Windows: []string{"-Wl,--start-group", "-lmingw32", "-lgcc", "-lgcc_eh",
			"-lmoldname", "-lmingwex", "-lmsvcrt", "-lucrt", "-lpthread",
			"-ladvapi32", "-lshell32", "-luser32", "-lkernel32", "-lpsapi",
			"-Wl,--end-group"},
	}
	hostStaticGccLibs = map[android.OsType][]string{
		android.Linux:   []string{"-Wl,--start-group", "-lgcc", "-lgcc_eh", "-lc", "-Wl,--end-group"},
		android.Darwin:  []string{"NO_STATIC_HOST_BINARIES_ON_DARWIN"},
		android.Windows: []string{"NO_STATIC_HOST_BINARIES_ON_WINDOWS"},
	}
}
