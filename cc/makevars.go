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

const (
	modulesAddedWall          = "ModulesAddedWall"
	modulesUsingWnoError      = "ModulesUsingWnoError"
	modulesMissingProfileFile = "ModulesMissingProfileFile"
)

func init() {
	android.RegisterMakeVarsProvider(pctx, makeVarsProvider)
}

func getNamedMapForConfig(config android.Config, name string) *sync.Map {
	return config.Once(name, func() interface{} {
		return &sync.Map{}
	}).(*sync.Map)
}

func makeStringOfKeys(ctx android.MakeVarsContext, setName string) string {
	set := getNamedMapForConfig(ctx.Config(), setName)
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
	ctx.Strict("PATH_TO_CLANG_TIDY", "${config.ClangBin}/clang-tidy")
	ctx.StrictSorted("CLANG_CONFIG_UNKNOWN_CFLAGS", strings.Join(config.ClangUnknownCflags, " "))

	ctx.Strict("RS_LLVM_PREBUILTS_VERSION", "${config.RSClangVersion}")
	ctx.Strict("RS_LLVM_PREBUILTS_BASE", "${config.RSClangBase}")
	ctx.Strict("RS_LLVM_PREBUILTS_PATH", "${config.RSLLVMPrebuiltsPath}")
	ctx.Strict("RS_LLVM_INCLUDES", "${config.RSIncludePath}")
	ctx.Strict("RS_CLANG", "${config.RSLLVMPrebuiltsPath}/clang")
	ctx.Strict("RS_LLVM_AS", "${config.RSLLVMPrebuiltsPath}/llvm-as")
	ctx.Strict("RS_LLVM_LINK", "${config.RSLLVMPrebuiltsPath}/llvm-link")

	ctx.Strict("GLOBAL_CFLAGS_NO_OVERRIDE", "${config.NoOverrideGlobalCflags}")
	ctx.Strict("GLOBAL_CLANG_CFLAGS_NO_OVERRIDE", "${config.ClangExtraNoOverrideCflags}")
	ctx.Strict("GLOBAL_CPPFLAGS_NO_OVERRIDE", "")
	ctx.Strict("GLOBAL_CLANG_CPPFLAGS_NO_OVERRIDE", "")
	ctx.Strict("NDK_PREBUILT_SHARED_LIBRARIES", strings.Join(ndkPrebuiltSharedLibs, " "))

	ctx.Strict("BOARD_VNDK_VERSION", ctx.DeviceConfig().VndkVersion())

	ctx.Strict("VNDK_CORE_LIBRARIES", strings.Join(vndkCoreLibraries, " "))
	ctx.Strict("VNDK_SAMEPROCESS_LIBRARIES", strings.Join(vndkSpLibraries, " "))
	ctx.Strict("LLNDK_LIBRARIES", strings.Join(llndkLibraries, " "))
	ctx.Strict("VNDK_PRIVATE_LIBRARIES", strings.Join(vndkPrivateLibraries, " "))

	// Filter vendor_public_library that are exported to make
	exportedVendorPublicLibraries := []string{}
	ctx.SingletonContext().VisitAllModules(func(module android.Module) {
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
	ctx.Strict("SOONG_MODULES_ADDED_WALL", makeStringOfKeys(ctx, modulesAddedWall))
	ctx.Strict("SOONG_MODULES_USING_WNO_ERROR", makeStringOfKeys(ctx, modulesUsingWnoError))
	ctx.Strict("SOONG_MODULES_MISSING_PGO_PROFILE_FILE", makeStringOfKeys(ctx, modulesMissingProfileFile))

	ctx.Strict("ADDRESS_SANITIZER_CONFIG_EXTRA_CFLAGS", strings.Join(asanCflags, " "))
	ctx.Strict("ADDRESS_SANITIZER_CONFIG_EXTRA_LDFLAGS", strings.Join(asanLdflags, " "))
	ctx.Strict("ADDRESS_SANITIZER_CONFIG_EXTRA_STATIC_LIBRARIES", strings.Join(asanLibs, " "))

	ctx.Strict("CFI_EXTRA_CFLAGS", strings.Join(cfiCflags, " "))
	ctx.Strict("CFI_EXTRA_LDFLAGS", strings.Join(cfiLdflags, " "))

	ctx.Strict("INTEGER_OVERFLOW_EXTRA_CFLAGS", strings.Join(intOverflowCflags, " "))

	ctx.Strict("DEFAULT_C_STD_VERSION", config.CStdVersion)
	ctx.Strict("DEFAULT_CPP_STD_VERSION", config.CppStdVersion)
	ctx.Strict("DEFAULT_GCC_CPP_STD_VERSION", config.GccCppStdVersion)
	ctx.Strict("EXPERIMENTAL_C_STD_VERSION", config.ExperimentalCStdVersion)
	ctx.Strict("EXPERIMENTAL_CPP_STD_VERSION", config.ExperimentalCppStdVersion)

	ctx.Strict("DEFAULT_GLOBAL_TIDY_CHECKS", "${config.TidyDefaultGlobalChecks}")
	ctx.Strict("DEFAULT_LOCAL_TIDY_CHECKS", joinLocalTidyChecks(config.DefaultLocalTidyChecks))
	ctx.Strict("DEFAULT_TIDY_HEADER_DIRS", "${config.TidyDefaultHeaderDirs}")

	ctx.Strict("AIDL_CPP", "${aidlCmd}")

	ctx.Strict("RS_GLOBAL_INCLUDES", "${config.RsGlobalIncludes}")

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

	hostTargets := ctx.Config().Targets[android.Host]
	makeVarsToolchain(ctx, "", hostTargets[0])
	if len(hostTargets) > 1 {
		makeVarsToolchain(ctx, "2ND_", hostTargets[1])
	}

	crossTargets := ctx.Config().Targets[android.HostCross]
	if len(crossTargets) > 0 {
		makeVarsToolchain(ctx, "", crossTargets[0])
		if len(crossTargets) > 1 {
			makeVarsToolchain(ctx, "2ND_", crossTargets[1])
		}
	}

	deviceTargets := ctx.Config().Targets[android.Device]
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
	case android.HostCross:
		typePrefix = "HOST_CROSS_"
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

	ctx.Strict(makePrefix+"GLOBAL_CFLAGS", strings.Join([]string{
		toolchain.Cflags(),
		"${config.CommonGlobalCflags}",
		fmt.Sprintf("${config.%sGlobalCflags}", hod),
		toolchain.ToolchainCflags(),
		productExtraCflags,
	}, " "))
	ctx.Strict(makePrefix+"GLOBAL_CONLYFLAGS", strings.Join([]string{
		"${config.CommonGlobalConlyflags}",
	}, " "))
	ctx.Strict(makePrefix+"GLOBAL_CPPFLAGS", strings.Join([]string{
		"${config.CommonGlobalCppflags}",
		fmt.Sprintf("${config.%sGlobalCppflags}", hod),
		toolchain.Cppflags(),
	}, " "))
	ctx.Strict(makePrefix+"GLOBAL_LDFLAGS", strings.Join([]string{
		fmt.Sprintf("${config.%sGlobalLdflags}", hod),
		toolchain.Ldflags(),
		toolchain.ToolchainLdflags(),
		productExtraLdflags,
	}, " "))

	includeFlags, err := ctx.Eval(toolchain.IncludeFlags())
	if err != nil {
		panic(err)
	}
	includes, systemIncludes := splitSystemIncludes(ctx, includeFlags)
	ctx.StrictRaw(makePrefix+"C_INCLUDES", strings.Join(includes, " "))
	ctx.StrictRaw(makePrefix+"C_SYSTEM_INCLUDES", strings.Join(systemIncludes, " "))

	if target.Arch.ArchType == android.Arm {
		flags, err := toolchain.InstructionSetFlags("arm")
		if err != nil {
			panic(err)
		}
		ctx.Strict(makePrefix+"arm_CFLAGS", flags)

		flags, err = toolchain.InstructionSetFlags("thumb")
		if err != nil {
			panic(err)
		}
		ctx.Strict(makePrefix+"thumb_CFLAGS", flags)
	}

	if toolchain.ClangSupported() {
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

		if target.Os.Class == android.Device {
			ctx.Strict(secondPrefix+"ADDRESS_SANITIZER_RUNTIME_LIBRARY", strings.TrimSuffix(config.AddressSanitizerRuntimeLibrary(toolchain), ".so"))
			ctx.Strict(secondPrefix+"UBSAN_RUNTIME_LIBRARY", strings.TrimSuffix(config.UndefinedBehaviorSanitizerRuntimeLibrary(toolchain), ".so"))
			ctx.Strict(secondPrefix+"UBSAN_MINIMAL_RUNTIME_LIBRARY", strings.TrimSuffix(config.UndefinedBehaviorSanitizerMinimalRuntimeLibrary(toolchain), ".a"))
			ctx.Strict(secondPrefix+"TSAN_RUNTIME_LIBRARY", strings.TrimSuffix(config.ThreadSanitizerRuntimeLibrary(toolchain), ".so"))
		}

		// This is used by external/gentoo/...
		ctx.Strict("CLANG_CONFIG_"+target.Arch.ArchType.Name+"_"+typePrefix+"TRIPLE",
			toolchain.ClangTriple())

		ctx.Strict(makePrefix+"CLANG_SUPPORTED", "true")
	} else {
		ctx.Strict(makePrefix+"CLANG_SUPPORTED", "")
	}

	ctx.Strict(makePrefix+"CC", gccCmd(toolchain, "gcc"))
	ctx.Strict(makePrefix+"CXX", gccCmd(toolchain, "g++"))

	if target.Os == android.Darwin {
		ctx.Strict(makePrefix+"AR", "${config.MacArPath}")
	} else {
		ctx.Strict(makePrefix+"AR", "${config.ClangBin}/llvm-ar")
		ctx.Strict(makePrefix+"READELF", gccCmd(toolchain, "readelf"))
		ctx.Strict(makePrefix+"NM", gccCmd(toolchain, "nm"))
	}

	if target.Os == android.Windows {
		ctx.Strict(makePrefix+"OBJDUMP", gccCmd(toolchain, "objdump"))
	}

	if target.Os.Class == android.Device {
		ctx.Strict(makePrefix+"OBJCOPY", gccCmd(toolchain, "objcopy"))
		ctx.Strict(makePrefix+"LD", gccCmd(toolchain, "ld"))
		ctx.Strict(makePrefix+"STRIP", gccCmd(toolchain, "strip"))
		ctx.Strict(makePrefix+"GCC_VERSION", toolchain.GccVersion())
		ctx.Strict(makePrefix+"NDK_GCC_VERSION", toolchain.GccVersion())
		ctx.Strict(makePrefix+"NDK_TRIPLE", config.NDKTriple(toolchain))
	}

	if target.Os.Class == android.Host || target.Os.Class == android.HostCross {
		ctx.Strict(makePrefix+"AVAILABLE_LIBRARIES", strings.Join(toolchain.AvailableLibraries(), " "))
	}

	ctx.Strict(makePrefix+"TOOLCHAIN_ROOT", toolchain.GccRoot())
	ctx.Strict(makePrefix+"TOOLS_PREFIX", gccCmd(toolchain, ""))
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
