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

	"android/soong/android"
	"android/soong/cc/config"
)

var (
	modulesAddedWallKey          = android.NewOnceKey("ModulesAddedWall")
	modulesUsingWnoErrorKey      = android.NewOnceKey("ModulesUsingWnoError")
	modulesMissingProfileFileKey = android.NewOnceKey("ModulesMissingProfileFile")
)

func init() {
	android.RegisterMakeVarsProvider(pctx, makeVarsProvider)
}

func getNamedMapForConfig(config android.Config, key android.OnceKey) *sync.Map {
	return config.Once(key, func() interface{} {
		return &sync.Map{}
	}).(*sync.Map)
}

func makeStringOfKeys(ctx android.MakeVarsContext, key android.OnceKey) string {
	set := getNamedMapForConfig(ctx.Config(), key)
	keys := []string{}
	set.Range(func(key interface{}, value interface{}) bool {
		keys = append(keys, key.(string))
		return true
	})
	sort.Strings(keys)
	return strings.Join(keys, " ")
}

func makeStringOfWarningAllowedProjects() string {
	allProjects := append([]string{}, config.WarningAllowedProjects...)
	allProjects = append(allProjects, config.WarningAllowedOldProjects...)
	sort.Strings(allProjects)
	// Makefile rules use pattern "path/%" to match module paths.
	if len(allProjects) > 0 {
		return strings.Join(allProjects, "% ") + "%"
	} else {
		return ""
	}
}

func makeVarsProvider(ctx android.MakeVarsContext) {
	ctx.Strict("LLVM_RELEASE_VERSION", "${config.ClangShortVersion}")
	ctx.Strict("LLVM_PREBUILTS_VERSION", "${config.ClangVersion}")
	ctx.Strict("LLVM_PREBUILTS_BASE", "${config.ClangBase}")
	ctx.Strict("LLVM_PREBUILTS_PATH", "${config.ClangBin}")
	ctx.Strict("CLANG", "${config.ClangBin}/clang")
	ctx.Strict("CLANG_CXX", "${config.ClangBin}/clang++")
	ctx.Strict("LLVM_AS", "${config.ClangBin}/llvm-as")
	ctx.Strict("LLVM_LINK", "${config.ClangBin}/llvm-link")
	ctx.Strict("LLVM_OBJCOPY", "${config.ClangBin}/llvm-objcopy")
	ctx.Strict("LLVM_STRIP", "${config.ClangBin}/llvm-strip")
	ctx.Strict("PATH_TO_CLANG_TIDY", "${config.ClangBin}/clang-tidy")
	ctx.Strict("PATH_TO_CLANG_TIDY_SHELL", "${config.ClangTidyShellPath}")
	ctx.StrictSorted("CLANG_CONFIG_UNKNOWN_CFLAGS", strings.Join(config.ClangUnknownCflags, " "))

	ctx.Strict("RS_LLVM_PREBUILTS_VERSION", "${config.RSClangVersion}")
	ctx.Strict("RS_LLVM_PREBUILTS_BASE", "${config.RSClangBase}")
	ctx.Strict("RS_LLVM_PREBUILTS_PATH", "${config.RSLLVMPrebuiltsPath}")
	ctx.Strict("RS_LLVM_INCLUDES", "${config.RSIncludePath}")
	ctx.Strict("RS_CLANG", "${config.RSLLVMPrebuiltsPath}/clang")
	ctx.Strict("RS_LLVM_AS", "${config.RSLLVMPrebuiltsPath}/llvm-as")
	ctx.Strict("RS_LLVM_LINK", "${config.RSLLVMPrebuiltsPath}/llvm-link")

	ctx.Strict("CLANG_EXTERNAL_CFLAGS", "${config.ClangExternalCflags}")
	ctx.Strict("GLOBAL_CLANG_CFLAGS_NO_OVERRIDE", "${config.NoOverrideClangGlobalCflags}")
	ctx.Strict("GLOBAL_CLANG_CPPFLAGS_NO_OVERRIDE", "")
	ctx.Strict("NDK_PREBUILT_SHARED_LIBRARIES", strings.Join(ndkPrebuiltSharedLibs, " "))

	ctx.Strict("BOARD_VNDK_VERSION", ctx.DeviceConfig().VndkVersion())

	ctx.Strict("VNDK_CORE_LIBRARIES", strings.Join(vndkCoreLibraries, " "))
	ctx.Strict("VNDK_SAMEPROCESS_LIBRARIES", strings.Join(vndkSpLibraries, " "))

	// Make uses LLNDK_LIBRARIES to determine which libraries to install.
	// HWASAN is only part of the LL-NDK in builds in which libc depends on HWASAN.
	// Therefore, by removing the library here, we cause it to only be installed if libc
	// depends on it.
	installedLlndkLibraries := []string{}
	for _, lib := range llndkLibraries {
		if strings.HasPrefix(lib, "libclang_rt.hwasan-") {
			continue
		}
		installedLlndkLibraries = append(installedLlndkLibraries, lib)
	}
	ctx.Strict("LLNDK_LIBRARIES", strings.Join(installedLlndkLibraries, " "))

	ctx.Strict("VNDK_PRIVATE_LIBRARIES", strings.Join(vndkPrivateLibraries, " "))
	ctx.Strict("VNDK_USING_CORE_VARIANT_LIBRARIES", strings.Join(vndkUsingCoreVariantLibraries, " "))

	// Filter vendor_public_library that are exported to make
	exportedVendorPublicLibraries := []string{}
	ctx.VisitAllModules(func(module android.Module) {
		if ccModule, ok := module.(*Module); ok {
			baseName := ccModule.BaseModuleName()
			if inList(baseName, vendorPublicLibraries) && module.ExportedToMake() {
				if !inList(baseName, exportedVendorPublicLibraries) {
					exportedVendorPublicLibraries = append(exportedVendorPublicLibraries, baseName)
				}
			}
		}
	})
	sort.Strings(exportedVendorPublicLibraries)
	ctx.Strict("VENDOR_PUBLIC_LIBRARIES", strings.Join(exportedVendorPublicLibraries, " "))

	sort.Strings(lsdumpPaths)
	ctx.Strict("LSDUMP_PATHS", strings.Join(lsdumpPaths, " "))

	ctx.Strict("ANDROID_WARNING_ALLOWED_PROJECTS", makeStringOfWarningAllowedProjects())
	ctx.Strict("SOONG_MODULES_ADDED_WALL", makeStringOfKeys(ctx, modulesAddedWallKey))
	ctx.Strict("SOONG_MODULES_USING_WNO_ERROR", makeStringOfKeys(ctx, modulesUsingWnoErrorKey))
	ctx.Strict("SOONG_MODULES_MISSING_PGO_PROFILE_FILE", makeStringOfKeys(ctx, modulesMissingProfileFileKey))

	ctx.Strict("ADDRESS_SANITIZER_CONFIG_EXTRA_CFLAGS", strings.Join(asanCflags, " "))
	ctx.Strict("ADDRESS_SANITIZER_CONFIG_EXTRA_LDFLAGS", strings.Join(asanLdflags, " "))
	ctx.Strict("ADDRESS_SANITIZER_CONFIG_EXTRA_STATIC_LIBRARIES", strings.Join(asanLibs, " "))

	ctx.Strict("HWADDRESS_SANITIZER_CONFIG_EXTRA_CFLAGS", strings.Join(hwasanCflags, " "))
	ctx.Strict("HWADDRESS_SANITIZER_GLOBAL_OPTIONS", strings.Join(hwasanGlobalOptions, ","))

	ctx.Strict("CFI_EXTRA_CFLAGS", strings.Join(cfiCflags, " "))
	ctx.Strict("CFI_EXTRA_ASFLAGS", strings.Join(cfiAsflags, " "))
	ctx.Strict("CFI_EXTRA_LDFLAGS", strings.Join(cfiLdflags, " "))

	ctx.Strict("INTEGER_OVERFLOW_EXTRA_CFLAGS", strings.Join(intOverflowCflags, " "))

	ctx.Strict("DEFAULT_C_STD_VERSION", config.CStdVersion)
	ctx.Strict("DEFAULT_CPP_STD_VERSION", config.CppStdVersion)
	ctx.Strict("EXPERIMENTAL_C_STD_VERSION", config.ExperimentalCStdVersion)
	ctx.Strict("EXPERIMENTAL_CPP_STD_VERSION", config.ExperimentalCppStdVersion)

	ctx.Strict("DEFAULT_GLOBAL_TIDY_CHECKS", "${config.TidyDefaultGlobalChecks}")
	ctx.Strict("DEFAULT_LOCAL_TIDY_CHECKS", joinLocalTidyChecks(config.DefaultLocalTidyChecks))
	ctx.Strict("DEFAULT_TIDY_HEADER_DIRS", "${config.TidyDefaultHeaderDirs}")
	ctx.Strict("WITH_TIDY_FLAGS", "${config.TidyWithTidyFlags}")

	ctx.Strict("AIDL_CPP", "${aidlCmd}")

	ctx.Strict("RS_GLOBAL_INCLUDES", "${config.RsGlobalIncludes}")

	ctx.Strict("SOONG_STRIP_PATH", "${stripPath}")
	ctx.Strict("XZ", "${xzCmd}")

	nativeHelperIncludeFlags, err := ctx.Eval("${config.CommonNativehelperInclude}")
	if err != nil {
		panic(err)
	}
	nativeHelperIncludes, nativeHelperSystemIncludes := splitSystemIncludes(ctx, nativeHelperIncludeFlags)
	if len(nativeHelperSystemIncludes) > 0 {
		panic("native helper may not have any system includes")
	}
	ctx.Strict("JNI_H_INCLUDE", strings.Join(nativeHelperIncludes, " "))

	includeFlags, err := ctx.Eval("${config.CommonGlobalIncludes}")
	if err != nil {
		panic(err)
	}
	includes, systemIncludes := splitSystemIncludes(ctx, includeFlags)
	ctx.StrictRaw("SRC_HEADERS", strings.Join(includes, " "))
	ctx.StrictRaw("SRC_SYSTEM_HEADERS", strings.Join(systemIncludes, " "))

	sort.Strings(ndkMigratedLibs)
	ctx.Strict("NDK_MIGRATED_LIBS", strings.Join(ndkMigratedLibs, " "))

	hostTargets := ctx.Config().Targets[android.BuildOs]
	makeVarsToolchain(ctx, "", hostTargets[0])
	if len(hostTargets) > 1 {
		makeVarsToolchain(ctx, "2ND_", hostTargets[1])
	}

	deviceTargets := ctx.Config().Targets[android.Android]
	makeVarsToolchain(ctx, "", deviceTargets[0])
	if len(deviceTargets) > 1 {
		makeVarsToolchain(ctx, "2ND_", deviceTargets[1])
	}
}

func makeVarsToolchain(ctx android.MakeVarsContext, secondPrefix string,
	target android.Target) {
	var typePrefix string
	switch target.Os.Class {
	case android.Host:
		typePrefix = "HOST_"
	case android.Device:
		typePrefix = "TARGET_"
	}
	makePrefix := secondPrefix + typePrefix

	toolchain := config.FindToolchain(target.Os, target.Arch)

	var productExtraCflags string
	var productExtraLdflags string

	hod := "Host"
	if target.Os.Class == android.Device {
		hod = "Device"
	}

	if target.Os.Class == android.Host && ctx.Config().HostStaticBinaries() {
		productExtraLdflags += "-static"
	}

	includeFlags, err := ctx.Eval(toolchain.IncludeFlags())
	if err != nil {
		panic(err)
	}
	includes, systemIncludes := splitSystemIncludes(ctx, includeFlags)
	ctx.StrictRaw(makePrefix+"C_INCLUDES", strings.Join(includes, " "))
	ctx.StrictRaw(makePrefix+"C_SYSTEM_INCLUDES", strings.Join(systemIncludes, " "))

	if target.Arch.ArchType == android.Arm {
		flags, err := toolchain.ClangInstructionSetFlags("arm")
		if err != nil {
			panic(err)
		}
		ctx.Strict(makePrefix+"arm_CFLAGS", flags)

		flags, err = toolchain.ClangInstructionSetFlags("thumb")
		if err != nil {
			panic(err)
		}
		ctx.Strict(makePrefix+"thumb_CFLAGS", flags)
	}

	clangPrefix := secondPrefix + "CLANG_" + typePrefix
	clangExtras := "-target " + toolchain.ClangTriple()
	clangExtras += " -B" + config.ToolPath(toolchain)

	ctx.Strict(clangPrefix+"GLOBAL_CFLAGS", strings.Join([]string{
		toolchain.ClangCflags(),
		"${config.CommonClangGlobalCflags}",
		fmt.Sprintf("${config.%sClangGlobalCflags}", hod),
		toolchain.ToolchainClangCflags(),
		clangExtras,
		productExtraCflags,
	}, " "))
	ctx.Strict(clangPrefix+"GLOBAL_CPPFLAGS", strings.Join([]string{
		"${config.CommonClangGlobalCppflags}",
		fmt.Sprintf("${config.%sGlobalCppflags}", hod),
		toolchain.ClangCppflags(),
	}, " "))
	ctx.Strict(clangPrefix+"GLOBAL_LDFLAGS", strings.Join([]string{
		fmt.Sprintf("${config.%sGlobalLdflags}", hod),
		toolchain.ClangLdflags(),
		toolchain.ToolchainClangLdflags(),
		productExtraLdflags,
		clangExtras,
	}, " "))
	ctx.Strict(clangPrefix+"GLOBAL_LLDFLAGS", strings.Join([]string{
		fmt.Sprintf("${config.%sGlobalLldflags}", hod),
		toolchain.ClangLldflags(),
		toolchain.ToolchainClangLdflags(),
		productExtraLdflags,
		clangExtras,
	}, " "))

	if target.Os.Class == android.Device {
		ctx.Strict(secondPrefix+"ADDRESS_SANITIZER_RUNTIME_LIBRARY", strings.TrimSuffix(config.AddressSanitizerRuntimeLibrary(toolchain), ".so"))
		ctx.Strict(secondPrefix+"HWADDRESS_SANITIZER_RUNTIME_LIBRARY", strings.TrimSuffix(config.HWAddressSanitizerRuntimeLibrary(toolchain), ".so"))
		ctx.Strict(secondPrefix+"HWADDRESS_SANITIZER_STATIC_LIBRARY", strings.TrimSuffix(config.HWAddressSanitizerStaticLibrary(toolchain), ".a"))
		ctx.Strict(secondPrefix+"UBSAN_RUNTIME_LIBRARY", strings.TrimSuffix(config.UndefinedBehaviorSanitizerRuntimeLibrary(toolchain), ".so"))
		ctx.Strict(secondPrefix+"UBSAN_MINIMAL_RUNTIME_LIBRARY", strings.TrimSuffix(config.UndefinedBehaviorSanitizerMinimalRuntimeLibrary(toolchain), ".a"))
		ctx.Strict(secondPrefix+"TSAN_RUNTIME_LIBRARY", strings.TrimSuffix(config.ThreadSanitizerRuntimeLibrary(toolchain), ".so"))
		ctx.Strict(secondPrefix+"SCUDO_RUNTIME_LIBRARY", strings.TrimSuffix(config.ScudoRuntimeLibrary(toolchain), ".so"))
		ctx.Strict(secondPrefix+"SCUDO_MINIMAL_RUNTIME_LIBRARY", strings.TrimSuffix(config.ScudoMinimalRuntimeLibrary(toolchain), ".so"))
	}

	// This is used by external/gentoo/...
	ctx.Strict("CLANG_CONFIG_"+target.Arch.ArchType.Name+"_"+typePrefix+"TRIPLE",
		toolchain.ClangTriple())

	if target.Os == android.Darwin {
		ctx.Strict(makePrefix+"AR", "${config.MacArPath}")
		ctx.Strict(makePrefix+"NM", "${config.MacToolPath}/nm")
		ctx.Strict(makePrefix+"OTOOL", "${config.MacToolPath}/otool")
		ctx.Strict(makePrefix+"STRIP", "${config.MacStripPath}")
	} else {
		ctx.Strict(makePrefix+"AR", "${config.ClangBin}/llvm-ar")
		ctx.Strict(makePrefix+"READELF", gccCmd(toolchain, "readelf"))
		ctx.Strict(makePrefix+"NM", gccCmd(toolchain, "nm"))
		ctx.Strict(makePrefix+"STRIP", gccCmd(toolchain, "strip"))
	}

	if target.Os.Class == android.Device {
		ctx.Strict(makePrefix+"OBJCOPY", gccCmd(toolchain, "objcopy"))
		ctx.Strict(makePrefix+"LD", gccCmd(toolchain, "ld"))
		ctx.Strict(makePrefix+"GCC_VERSION", toolchain.GccVersion())
		ctx.Strict(makePrefix+"NDK_TRIPLE", config.NDKTriple(toolchain))
		ctx.Strict(makePrefix+"TOOLS_PREFIX", gccCmd(toolchain, ""))
	}

	if target.Os.Class == android.Host {
		ctx.Strict(makePrefix+"AVAILABLE_LIBRARIES", strings.Join(toolchain.AvailableLibraries(), " "))
	}

	ctx.Strict(makePrefix+"SHLIB_SUFFIX", toolchain.ShlibSuffix())
	ctx.Strict(makePrefix+"EXECUTABLE_SUFFIX", toolchain.ExecutableSuffix())
}

func splitSystemIncludes(ctx android.MakeVarsContext, val string) (includes, systemIncludes []string) {
	flags, err := ctx.Eval(val)
	if err != nil {
		panic(err)
	}

	extract := func(flags string, dirs []string, prefix string) (string, []string, bool) {
		if strings.HasPrefix(flags, prefix) {
			flags = strings.TrimPrefix(flags, prefix)
			flags = strings.TrimLeft(flags, " ")
			s := strings.SplitN(flags, " ", 2)
			dirs = append(dirs, s[0])
			if len(s) > 1 {
				return strings.TrimLeft(s[1], " "), dirs, true
			}
			return "", dirs, true
		} else {
			return flags, dirs, false
		}
	}

	flags = strings.TrimLeft(flags, " ")
	for flags != "" {
		found := false
		flags, includes, found = extract(flags, includes, "-I")
		if !found {
			flags, systemIncludes, found = extract(flags, systemIncludes, "-isystem ")
		}
		if !found {
			panic(fmt.Errorf("Unexpected flag in %q", flags))
		}
	}

	return includes, systemIncludes
}

func joinLocalTidyChecks(checks []config.PathBasedTidyCheck) string {
	rets := make([]string, len(checks))
	for i, check := range config.DefaultLocalTidyChecks {
		rets[i] = check.PathPrefix + ":" + check.Checks
	}
	return strings.Join(rets, " ")
}
