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

package config

import (
	"strings"

	"android/soong/android"
)

// Flags used by lots of devices.  Putting them in package static variables will save bytes in
// build.ninja so they aren't repeated for every file
var (
	commonGlobalCflags = []string{
		"-DANDROID",
		"-fmessage-length=0",
		"-W",
		"-Wall",
		"-Wno-unused",
		"-Winit-self",
		"-Wpointer-arith",

		// COMMON_RELEASE_CFLAGS
		"-DNDEBUG",
		"-UDEBUG",
	}

	commonGlobalConlyflags = []string{}

	deviceGlobalCflags = []string{
		"-fdiagnostics-color",

		// TARGET_ERROR_FLAGS
		"-Werror=return-type",
		"-Werror=non-virtual-dtor",
		"-Werror=address",
		"-Werror=sequence-point",
		"-Werror=date-time",
	}

	hostGlobalCflags = []string{}

	commonGlobalCppflags = []string{
		"-Wsign-promo",
	}

	noOverrideGlobalCflags = []string{
		"-Werror=int-to-pointer-cast",
		"-Werror=pointer-to-int-cast",
	}

	IllegalFlags = []string{
		"-w",
	}

	CStdVersion      = "gnu99"
	CppStdVersion    = "gnu++14"
	GccCppStdVersion = "gnu++11"
)

var pctx = android.NewPackageContext("android/soong/cc/config")

func init() {
	if android.BuildOs == android.Linux {
		commonGlobalCflags = append(commonGlobalCflags, "-fdebug-prefix-map=/proc/self/cwd=")
	}

	pctx.StaticVariable("CommonGlobalCflags", strings.Join(commonGlobalCflags, " "))
	pctx.StaticVariable("CommonGlobalConlyflags", strings.Join(commonGlobalConlyflags, " "))
	pctx.StaticVariable("DeviceGlobalCflags", strings.Join(deviceGlobalCflags, " "))
	pctx.StaticVariable("HostGlobalCflags", strings.Join(hostGlobalCflags, " "))
	pctx.StaticVariable("NoOverrideGlobalCflags", strings.Join(noOverrideGlobalCflags, " "))

	pctx.StaticVariable("CommonGlobalCppflags", strings.Join(commonGlobalCppflags, " "))

	pctx.StaticVariable("CommonClangGlobalCflags",
		strings.Join(append(ClangFilterUnknownCflags(commonGlobalCflags), "${ClangExtraCflags}"), " "))
	pctx.StaticVariable("DeviceClangGlobalCflags",
		strings.Join(append(ClangFilterUnknownCflags(deviceGlobalCflags), "${ClangExtraTargetCflags}"), " "))
	pctx.StaticVariable("HostClangGlobalCflags",
		strings.Join(ClangFilterUnknownCflags(hostGlobalCflags), " "))
	pctx.StaticVariable("NoOverrideClangGlobalCflags",
		strings.Join(append(ClangFilterUnknownCflags(noOverrideGlobalCflags), "${ClangExtraNoOverrideCflags}"), " "))

	pctx.StaticVariable("CommonClangGlobalCppflags",
		strings.Join(append(ClangFilterUnknownCflags(commonGlobalCppflags), "${ClangExtraCppflags}"), " "))

	// Everything in these lists is a crime against abstraction and dependency tracking.
	// Do not add anything to this list.
	pctx.PrefixedPathsForOptionalSourceVariable("CommonGlobalIncludes", "-I",
		[]string{
			"system/core/include",
			"system/media/audio/include",
			"hardware/libhardware/include",
			"frameworks/native/include",
		})
	pctx.PrefixedPathsForOptionalSourceVariable("CommonGlobalSystemIncludes", "-isystem ",
		[]string{
			"hardware/libhardware_legacy/include",
			"hardware/ril/include",
			"libnativehelper/include",
			"frameworks/native/opengl/include",
			"frameworks/av/include",
			"frameworks/base/include",
		})
	// This is used by non-NDK modules to get jni.h. export_include_dirs doesn't help
	// with this, since there is no associated library.
	pctx.PrefixedPathsForOptionalSourceVariable("CommonNativehelperInclude", "-I",
		[]string{"libnativehelper/include/nativehelper"})

	pctx.SourcePathVariable("ClangDefaultBase", "prebuilts/clang/host")
	pctx.VariableFunc("ClangBase", func(config interface{}) (string, error) {
		if override := config.(android.Config).Getenv("LLVM_PREBUILTS_BASE"); override != "" {
			return override, nil
		}
		return "${ClangDefaultBase}", nil
	})
	pctx.VariableFunc("ClangVersion", func(config interface{}) (string, error) {
		if override := config.(android.Config).Getenv("LLVM_PREBUILTS_VERSION"); override != "" {
			return override, nil
		}
		return "clang-3289846", nil
	})
	pctx.StaticVariable("ClangPath", "${ClangBase}/${HostPrebuiltTag}/${ClangVersion}")
	pctx.StaticVariable("ClangBin", "${ClangPath}/bin")

	pctx.StaticVariable("ClangAsanLibDir", "${ClangPath}/lib64/clang/3.8/lib/linux")

	pctx.VariableFunc("CcWrapper", func(config interface{}) (string, error) {
		if override := config.(android.Config).Getenv("CC_WRAPPER"); override != "" {
			return override + " ", nil
		}
		return "", nil
	})
}

var HostPrebuiltTag = pctx.VariableConfigMethod("HostPrebuiltTag", android.Config.PrebuiltOS)

func bionicHeaders(bionicArch, kernelArch string) string {
	return strings.Join([]string{
		"-isystem bionic/libc/arch-" + bionicArch + "/include",
		"-isystem bionic/libc/include",
		"-isystem bionic/libc/kernel/uapi",
		"-isystem bionic/libc/kernel/uapi/asm-" + kernelArch,
		"-isystem bionic/libc/kernel/android/uapi",
	}, " ")
}
