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
	modulesWarningsAllowedKey    = android.NewOnceKey("ModulesWarningsAllowed")
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
	sort.Strings(allProjects)
	// Makefile rules use pattern "path/%" to match module paths.
	if len(allProjects) > 0 {
		return strings.Join(allProjects, "% ") + "%"
	} else {
		return ""
	}
}

type notOnHostContext struct {
}

func (c *notOnHostContext) Host() bool {
	return false
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
	ctx.StrictSorted("CLANG_CONFIG_UNKNOWN_CFLAGS", strings.Join(config.ClangUnknownCflags, " "))

	ctx.Strict("RS_LLVM_PREBUILTS_VERSION", "${config.RSClangVersion}")
	ctx.Strict("RS_LLVM_PREBUILTS_BASE", "${config.RSClangBase}")
	ctx.Strict("RS_LLVM_PREBUILTS_PATH", "${config.RSLLVMPrebuiltsPath}")
	ctx.Strict("RS_LLVM_INCLUDES", "${config.RSIncludePath}")
	ctx.Strict("RS_CLANG", "${config.RSLLVMPrebuiltsPath}/clang")
	ctx.Strict("RS_LLVM_AS", "${config.RSLLVMPrebuiltsPath}/llvm-as")
	ctx.Strict("RS_LLVM_LINK", "${config.RSLLVMPrebuiltsPath}/llvm-link")

	ctx.Strict("CLANG_EXTERNAL_CFLAGS", "${config.ExternalCflags}")
	ctx.Strict("GLOBAL_CLANG_CFLAGS_NO_OVERRIDE", "${config.NoOverrideGlobalCflags}")
	ctx.Strict("GLOBAL_CLANG_CFLAGS_64_NO_OVERRIDE", "${config.NoOverride64GlobalCflags}")
	ctx.Strict("GLOBAL_CLANG_CPPFLAGS_NO_OVERRIDE", "")
	ctx.Strict("GLOBAL_CLANG_EXTERNAL_CFLAGS_NO_OVERRIDE", "${config.NoOverrideExternalGlobalCflags}")

	ctx.Strict("BOARD_VNDK_VERSION", ctx.DeviceConfig().VndkVersion())
	ctx.Strict("RECOVERY_SNAPSHOT_VERSION", ctx.DeviceConfig().RecoverySnapshotVersion())

	// Filter vendor_public_library that are exported to make
	exportedVendorPublicLibraries := []string{}
	ctx.VisitAllModules(func(module android.Module) {
		if ccModule, ok := module.(*Module); ok {
			baseName := ccModule.BaseModuleName()
			if ccModule.IsVendorPublicLibrary() && module.ExportedToMake() {
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
	ctx.Strict("SOONG_MODULES_WARNINGS_ALLOWED", makeStringOfKeys(ctx, modulesWarningsAllowedKey))
	ctx.Strict("SOONG_MODULES_USING_WNO_ERROR", makeStringOfKeys(ctx, modulesUsingWnoErrorKey))
	ctx.Strict("SOONG_MODULES_MISSING_PGO_PROFILE_FILE", makeStringOfKeys(ctx, modulesMissingProfileFileKey))

 	ctx.Strict("CLANG_COVERAGE_CONFIG_CFLAGS", strings.Join(clangCoverageCFlags, " "))
 	ctx.Strict("CLANG_COVERAGE_CONFIG_COMMFLAGS", strings.Join(clangCoverageCommonFlags, " "))
 	ctx.Strict("CLANG_COVERAGE_HOST_LDFLAGS", strings.Join(clangCoverageHostLdFlags, " "))
 	ctx.Strict("CLANG_COVERAGE_INSTR_PROFILE", profileInstrFlag)
 	ctx.Strict("CLANG_COVERAGE_CONTINUOUS_FLAGS", strings.Join(clangContinuousCoverageFlags, " "))
 	ctx.Strict("CLANG_COVERAGE_HWASAN_FLAGS", strings.Join(clangCoverageHWASanFlags, " "))

	ctx.Strict("ADDRESS_SANITIZER_CONFIG_EXTRA_CFLAGS", strings.Join(asanCflags, " "))
	ctx.Strict("ADDRESS_SANITIZER_CONFIG_EXTRA_LDFLAGS", strings.Join(asanLdflags, " "))

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
	ctx.Strict("ALLOWED_MANUAL_INTERFACE_PATHS", strings.Join(allowedManualInterfacePaths, " "))

	ctx.Strict("RS_GLOBAL_INCLUDES", "${config.RsGlobalIncludes}")

	ctx.Strict("SOONG_STRIP_PATH", "${stripPath}")
	ctx.Strict("XZ", "${xzCmd}")
	ctx.Strict("CREATE_MINIDEBUGINFO", "${createMiniDebugInfo}")

	includeFlags, err := ctx.Eval("${config.CommonGlobalIncludes}")
	if err != nil {
		panic(err)
	}
	includes, systemIncludes := splitSystemIncludes(ctx, includeFlags)
	ctx.StrictRaw("SRC_HEADERS", strings.Join(includes, " "))
	ctx.StrictRaw("SRC_SYSTEM_HEADERS", strings.Join(systemIncludes, " "))

	ndkKnownLibs := *getNDKKnownLibs(ctx.Config())
	sort.Strings(ndkKnownLibs)
	ctx.Strict("NDK_KNOWN_LIBS", strings.Join(ndkKnownLibs, " "))

	hostTargets := ctx.Config().Targets[ctx.Config().BuildOS]
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

	clangPrefix := secondPrefix + "CLANG_" + typePrefix

	ctx.Strict(clangPrefix+"TRIPLE", toolchain.ClangTriple())
	ctx.Strict(clangPrefix+"GLOBAL_CFLAGS", strings.Join([]string{
		toolchain.Cflags(),
		"${config.CommonGlobalCflags}",
		fmt.Sprintf("${config.%sGlobalCflags}", hod),
		toolchain.ToolchainCflags(),
		productExtraCflags,
	}, " "))
	ctx.Strict(clangPrefix+"GLOBAL_CPPFLAGS", strings.Join([]string{
		"${config.CommonGlobalCppflags}",
		fmt.Sprintf("${config.%sGlobalCppflags}", hod),
		toolchain.Cppflags(),
	}, " "))
	ctx.Strict(clangPrefix+"GLOBAL_LDFLAGS", strings.Join([]string{
		fmt.Sprintf("${config.%sGlobalLdflags}", hod),
		toolchain.Ldflags(),
		toolchain.ToolchainLdflags(),
		productExtraLdflags,
	}, " "))
	ctx.Strict(clangPrefix+"GLOBAL_LLDFLAGS", strings.Join([]string{
		fmt.Sprintf("${config.%sGlobalLldflags}", hod),
		toolchain.Lldflags(),
		toolchain.ToolchainLdflags(),
		productExtraLdflags,
	}, " "))

	if target.Os.Class == android.Device {
		sanitizerVariables := map[string]string{
			"ADDRESS_SANITIZER_RUNTIME_LIBRARY":   config.AddressSanitizerRuntimeLibrary(toolchain),
			"HWADDRESS_SANITIZER_RUNTIME_LIBRARY": config.HWAddressSanitizerRuntimeLibrary(toolchain),
			"HWADDRESS_SANITIZER_STATIC_LIBRARY":  config.HWAddressSanitizerStaticLibrary(toolchain),
			"UBSAN_RUNTIME_LIBRARY":               config.UndefinedBehaviorSanitizerRuntimeLibrary(toolchain),
			"UBSAN_MINIMAL_RUNTIME_LIBRARY":       config.UndefinedBehaviorSanitizerMinimalRuntimeLibrary(toolchain),
			"TSAN_RUNTIME_LIBRARY":                config.ThreadSanitizerRuntimeLibrary(toolchain),
			"SCUDO_RUNTIME_LIBRARY":               config.ScudoRuntimeLibrary(toolchain),
			"SCUDO_MINIMAL_RUNTIME_LIBRARY":       config.ScudoMinimalRuntimeLibrary(toolchain),
		}

		for variable, value := range sanitizerVariables {
			ctx.Strict(secondPrefix+variable, value)
		}

		sanitizerLibs := android.SortedStringValues(sanitizerVariables)
		var sanitizerLibStems []string
		ctx.VisitAllModules(func(m android.Module) {
			if !m.Enabled() {
				return
			}

			ccModule, _ := m.(*Module)
			if ccModule == nil || ccModule.library == nil || !ccModule.library.shared() {
				return
			}

			if android.InList(strings.TrimPrefix(ctx.ModuleName(m), "prebuilt_"), sanitizerLibs) &&
				m.Target().Os == target.Os && m.Target().Arch.ArchType == target.Arch.ArchType {
				outputFile := ccModule.outputFile
				if outputFile.Valid() {
					sanitizerLibStems = append(sanitizerLibStems, outputFile.Path().Base())
				}
			}
		})
		sanitizerLibStems = android.SortedUniqueStrings(sanitizerLibStems)
		ctx.Strict(secondPrefix+"SANITIZER_STEMS", strings.Join(sanitizerLibStems, " "))
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
		ctx.Strict(makePrefix+"READELF", "${config.ClangBin}/llvm-readelf")
		ctx.Strict(makePrefix+"NM", "${config.ClangBin}/llvm-nm")
		ctx.Strict(makePrefix+"STRIP", "${config.ClangBin}/llvm-strip")
	}

	if target.Os.Class == android.Device {
		ctx.Strict(makePrefix+"OBJCOPY", "${config.ClangBin}/llvm-objcopy")
		ctx.Strict(makePrefix+"LD", "${config.ClangBin}/lld")
		ctx.Strict(makePrefix+"NDK_TRIPLE", config.NDKTriple(toolchain))
		ctx.Strict(makePrefix+"TOOLS_PREFIX", "${config.ClangBin}/llvm-")
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
