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
	"runtime"
	"strings"

	"android/soong/android"
	"android/soong/remoteexec"
)

var (
	pctx         = android.NewPackageContext("android/soong/cc/config")
	exportedVars = android.NewExportedVariables(pctx)

	// Flags used by lots of devices.  Putting them in package static variables
	// will save bytes in build.ninja so they aren't repeated for every file
	commonGlobalCflags = []string{
		// Enable some optimization by default.
		"-O2",

		// Warnings enabled by default. Reference:
		// https://clang.llvm.org/docs/DiagnosticsReference.html
		"-Wall",
		"-Wextra",
		"-Winit-self",
		"-Wpointer-arith",
		"-Wunguarded-availability",

		// Warnings treated as errors by default.
		// See also noOverrideGlobalCflags for errors that cannot be disabled
		// from Android.bp files.

		// Using __DATE__/__TIME__ causes build nondeterminism.
		"-Werror=date-time",
		// Detects forgotten */& that usually cause a crash
		"-Werror=int-conversion",
		// Detects unterminated alignment modification pragmas, which often lead
		// to ABI mismatch between modules and hard-to-debug crashes.
		"-Werror=pragma-pack",
		// Same as above, but detects alignment pragmas around a header
		// inclusion.
		"-Werror=pragma-pack-suspicious-include",
		// Detects dividing an array size by itself, which is a common typo that
		// leads to bugs.
		"-Werror=sizeof-array-div",
		// Detects a typo that cuts off a prefix from a string literal.
		"-Werror=string-plus-int",
		// Detects for loops that will never execute more than once (for example
		// due to unconditional break), but have a non-empty loop increment
		// clause. Often a mistake/bug.
		"-Werror=unreachable-code-loop-increment",

		// Warnings that should not be errors even for modules with -Werror.

		// Making deprecated usages an error causes extreme pain when trying to
		// deprecate anything.
		"-Wno-error=deprecated-declarations",

		// Warnings disabled by default.

		// Designated initializer syntax is recommended by the Google C++ style
		// and is OK to use even if not formally supported by the chosen C++
		// version.
		"-Wno-c99-designator",
		// Detects uses of a GNU C extension equivalent to a limited form of
		// constexpr. Enabling this would require replacing many constants with
		// macros, which is not a good trade-off.
		"-Wno-gnu-folding-constant",
		// AIDL generated code redeclares pure virtual methods in each
		// subsequent version of an interface, so this warning is currently
		// infeasible to enable.
		"-Wno-inconsistent-missing-override",
		// Detects designated initializers that are in a different order than
		// the fields in the initialized type, which causes the side effects
		// of initializers to occur out of order with the source code.
		// In practice, this warning has extremely poor signal to noise ratio,
		// because it is triggered even for initializers with no side effects.
		// Individual modules can still opt into it via cflags.
		"-Wno-error=reorder-init-list",
		"-Wno-reorder-init-list",
		// Incompatible with the Google C++ style guidance to use 'int' for loop
		// indices; poor signal to noise ratio.
		"-Wno-sign-compare",
		// Poor signal to noise ratio.
		"-Wno-unused",

		// Global preprocessor constants.

		"-DANDROID",
		"-DNDEBUG",
		"-UDEBUG",
		"-D__compiler_offsetof=__builtin_offsetof",
		// Allows the bionic versioning.h to indirectly determine whether the
		// option -Wunguarded-availability is on or not.
		"-D__ANDROID_UNAVAILABLE_SYMBOLS_ARE_WEAK__",

		// -f and -g options.

		// Emit address-significance table which allows linker to perform safe ICF. Clang does
		// not emit the table by default on Android since NDK still uses GNU binutils.
		"-faddrsig",

		// Emit debugging data in a modern format (DWARF v5).
		"-fdebug-default-version=5",

		// Force clang to always output color diagnostics. Ninja will strip the ANSI
		// color codes if it is not running in a terminal.
		"-fcolor-diagnostics",

		// Turn off FMA which got enabled by default in clang-r445002 (http://b/218805949)
		"-ffp-contract=off",

		// Google C++ style does not allow exceptions, turn them off by default.
		"-fno-exceptions",

		// Disable optimizations based on strict aliasing by default.
		// The performance benefit of enabling them currently does not outweigh
		// the risk of hard-to-reproduce bugs.
		"-fno-strict-aliasing",

		// Disable line wrapping for error messages - it interferes with
		// displaying logs in web browsers.
		"-fmessage-length=0",

		// Using simple template names reduces the size of debug builds.
		"-gsimple-template-names",

		// Use zstd to compress debug data.
		"-gz=zstd",

		// Make paths in deps files relative.
		"-no-canonical-prefixes",
	}

	commonGlobalConlyflags = []string{}

	commonGlobalAsflags = []string{
		"-D__ASSEMBLY__",
		// TODO(b/235105792): override global -fdebug-default-version=5, it is causing $TMPDIR to
		// end up in the dwarf data for crtend_so.S.
		"-fdebug-default-version=4",
	}

	// Compilation flags for device code; not applied to host code.
	deviceGlobalCflags = []string{
		"-ffunction-sections",
		"-fdata-sections",
		"-fno-short-enums",
		"-funwind-tables",
		"-fstack-protector-strong",
		"-Wa,--noexecstack",
		"-D_FORTIFY_SOURCE=2",

		"-Wstrict-aliasing=2",

		"-Werror=return-type",
		"-Werror=non-virtual-dtor",
		"-Werror=address",
		"-Werror=sequence-point",
		"-Werror=format-security",
		"-nostdlibinc",

		// Emit additional debug info for AutoFDO
		"-fdebug-info-for-profiling",
	}

	commonGlobalLldflags = []string{
		"-fuse-ld=lld",
		"-Wl,--icf=safe",
		"-Wl,--no-demangle",
	}

	deviceGlobalCppflags = []string{
		"-fvisibility-inlines-hidden",
	}

	// Linking flags for device code; not applied to host binaries.
	deviceGlobalLdflags = []string{
		"-Wl,-z,noexecstack",
		"-Wl,-z,relro",
		"-Wl,-z,now",
		"-Wl,--build-id=md5",
		"-Wl,--fatal-warnings",
		"-Wl,--no-undefined-version",
		// TODO: Eventually we should link against a libunwind.a with hidden symbols, and then these
		// --exclude-libs arguments can be removed.
		"-Wl,--exclude-libs,libgcc.a",
		"-Wl,--exclude-libs,libgcc_stripped.a",
		"-Wl,--exclude-libs,libunwind_llvm.a",
		"-Wl,--exclude-libs,libunwind.a",
	}

	deviceGlobalLldflags = append(append(deviceGlobalLdflags, commonGlobalLldflags...),
		"-Wl,--compress-debug-sections=zstd",
	)

	hostGlobalCflags = []string{}

	hostGlobalCppflags = []string{}

	hostGlobalLdflags = []string{}

	hostGlobalLldflags = commonGlobalLldflags

	commonGlobalCppflags = []string{
		// -Wimplicit-fallthrough is not enabled by -Wall.
		"-Wimplicit-fallthrough",

		// Enable clang's thread-safety annotations in libcxx.
		"-D_LIBCPP_ENABLE_THREAD_SAFETY_ANNOTATIONS",

		// libc++'s math.h has an #include_next outside of system_headers.
		"-Wno-gnu-include-next",
	}

	// These flags are appended after the module's cflags, so they cannot be
	// overridden from Android.bp files.
	//
	// NOTE: if you need to disable a warning to unblock a compiler upgrade
	// and it is only triggered by third party code, add it to
	// extraExternalCflags (if possible) or noOverrideExternalGlobalCflags
	// (if the former doesn't work). If the new warning also occurs in first
	// party code, try adding it to commonGlobalCflags first. Adding it here
	// should be the last resort, because it prevents all code in Android from
	// opting into the warning.
	noOverrideGlobalCflags = []string{
		"-Werror=bool-operation",
		"-Werror=format-insufficient-args",
		"-Werror=implicit-int-float-conversion",
		"-Werror=int-in-bool-context",
		"-Werror=int-to-pointer-cast",
		"-Werror=pointer-to-int-cast",
		"-Werror=xor-used-as-pow",
		// http://b/161386391 for -Wno-void-pointer-to-enum-cast
		"-Wno-void-pointer-to-enum-cast",
		// http://b/161386391 for -Wno-void-pointer-to-int-cast
		"-Wno-void-pointer-to-int-cast",
		// http://b/161386391 for -Wno-pointer-to-int-cast
		"-Wno-pointer-to-int-cast",
		"-Werror=fortify-source",

		"-Werror=address-of-temporary",
		"-Werror=incompatible-function-pointer-types",
		"-Werror=null-dereference",
		"-Werror=return-type",

		// http://b/72331526 Disable -Wtautological-* until the instances detected by these
		// new warnings are fixed.
		"-Wno-tautological-constant-compare",
		"-Wno-tautological-type-limit-compare",
		// http://b/145211066
		"-Wno-implicit-int-float-conversion",
		// New warnings to be fixed after clang-r377782.
		"-Wno-tautological-overlap-compare", // http://b/148815696
		// New warnings to be fixed after clang-r383902.
		"-Wno-deprecated-copy",                      // http://b/153746672
		"-Wno-range-loop-construct",                 // http://b/153747076
		"-Wno-zero-as-null-pointer-constant",        // http://b/68236239
		"-Wno-deprecated-anon-enum-enum-conversion", // http://b/153746485
		"-Wno-pessimizing-move",                     // http://b/154270751
		// New warnings to be fixed after clang-r399163
		"-Wno-non-c-typedef-for-linkage", // http://b/161304145
		// New warnings to be fixed after clang-r428724
		"-Wno-align-mismatch", // http://b/193679946
		// New warnings to be fixed after clang-r433403
		"-Wno-error=unused-but-set-variable",  // http://b/197240255
		"-Wno-error=unused-but-set-parameter", // http://b/197240255
		// New warnings to be fixed after clang-r468909
		"-Wno-error=deprecated-builtins", // http://b/241601211
		"-Wno-error=deprecated",          // in external/googletest/googletest
		// New warnings to be fixed after clang-r475365
		"-Wno-error=single-bit-bitfield-constant-conversion", // http://b/243965903
		"-Wno-error=enum-constexpr-conversion",               // http://b/243964282
	}

	noOverride64GlobalCflags = []string{}

	// Extra cflags applied to third-party code (anything for which
	// IsThirdPartyPath() in build/soong/android/paths.go returns true;
	// includes external/, most of vendor/ and most of hardware/)
	extraExternalCflags = []string{
		"-Wno-enum-compare",
		"-Wno-enum-compare-switch",

		// http://b/72331524 Allow null pointer arithmetic until the instances detected by
		// this new warning are fixed.
		"-Wno-null-pointer-arithmetic",

		// Bug: http://b/29823425 Disable -Wnull-dereference until the
		// new instances detected by this warning are fixed.
		"-Wno-null-dereference",

		// http://b/145211477
		"-Wno-pointer-compare",
		"-Wno-final-dtor-non-final-class",

		// http://b/165945989
		"-Wno-psabi",

		// http://b/199369603
		"-Wno-null-pointer-subtraction",

		// http://b/175068488
		"-Wno-string-concatenation",

		// http://b/239661264
		"-Wno-deprecated-non-prototype",
	}

	// Similar to noOverrideGlobalCflags, but applies only to third-party code
	// (see extraExternalCflags).
	// This section can unblock compiler upgrades when a third party module that
	// enables -Werror and some group of warnings explicitly triggers newly
	// added warnings.
	noOverrideExternalGlobalCflags = []string{
		// http://b/151457797
		"-fcommon",
		// http://b/191699019
		"-Wno-format-insufficient-args",
		// http://b/296321508
		// Introduced in response to a critical security vulnerability and
		// should be a hard error - it requires only whitespace changes to fix.
		"-Wno-misleading-indentation",
		// Triggered by old LLVM code in external/llvm. Likely not worth
		// enabling since it's a cosmetic issue.
		"-Wno-bitwise-instead-of-logical",

		"-Wno-unused-but-set-variable",
		"-Wno-unused-but-set-parameter",
		"-Wno-unqualified-std-cast-call",
		"-Wno-array-parameter",
		"-Wno-gnu-offsetof-extensions",
	}

	llvmNextExtraCommonGlobalCflags = []string{
		// Do not report warnings when testing with the top of trunk LLVM.
		"-Wno-everything",
	}

	// Flags that must not appear in any command line.
	IllegalFlags = []string{
		"-w",
	}

	CStdVersion               = "gnu17"
	CppStdVersion             = "gnu++20"
	ExperimentalCStdVersion   = "gnu2x"
	ExperimentalCppStdVersion = "gnu++2b"

	// prebuilts/clang default settings.
	ClangDefaultBase         = "prebuilts/clang/host"
	ClangDefaultVersion      = "clang-r498229b"
	ClangDefaultShortVersion = "17"

	// Directories with warnings from Android.bp files.
	WarningAllowedProjects = []string{
		"device/",
		"vendor/",
	}

	VersionScriptFlagPrefix = "-Wl,--version-script,"

	VisibilityHiddenFlag  = "-fvisibility=hidden"
	VisibilityDefaultFlag = "-fvisibility=default"
)

func ExportStringList(name string, value []string) {
	exportedVars.ExportStringList(name, value)
}

func init() {
	if runtime.GOOS == "linux" {
		commonGlobalCflags = append(commonGlobalCflags, "-fdebug-prefix-map=/proc/self/cwd=")
	}

	exportedVars.ExportStringListStaticVariable("CommonGlobalConlyflags", commonGlobalConlyflags)
	exportedVars.ExportStringListStaticVariable("CommonGlobalAsflags", commonGlobalAsflags)
	exportedVars.ExportStringListStaticVariable("DeviceGlobalCppflags", deviceGlobalCppflags)
	exportedVars.ExportStringListStaticVariable("DeviceGlobalLdflags", deviceGlobalLdflags)
	exportedVars.ExportStringListStaticVariable("DeviceGlobalLldflags", deviceGlobalLldflags)
	exportedVars.ExportStringListStaticVariable("HostGlobalCppflags", hostGlobalCppflags)
	exportedVars.ExportStringListStaticVariable("HostGlobalLdflags", hostGlobalLdflags)
	exportedVars.ExportStringListStaticVariable("HostGlobalLldflags", hostGlobalLldflags)

	// Export the static default CommonGlobalCflags to Bazel.
	exportedVars.ExportStringList("CommonGlobalCflags", commonGlobalCflags)

	pctx.VariableFunc("CommonGlobalCflags", func(ctx android.PackageVarContext) string {
		flags := commonGlobalCflags

		// http://b/131390872
		// Automatically initialize any uninitialized stack variables.
		// Prefer zero-init if multiple options are set.
		if ctx.Config().IsEnvTrue("AUTO_ZERO_INITIALIZE") {
			flags = append(flags, "-ftrivial-auto-var-init=zero")
		} else if ctx.Config().IsEnvTrue("AUTO_PATTERN_INITIALIZE") {
			flags = append(flags, "-ftrivial-auto-var-init=pattern")
		} else if ctx.Config().IsEnvTrue("AUTO_UNINITIALIZE") {
			flags = append(flags, "-ftrivial-auto-var-init=uninitialized")
		} else {
			// Default to zero initialization.
			flags = append(flags, "-ftrivial-auto-var-init=zero")
		}

		// Workaround for ccache with clang.
		// See http://petereisentraut.blogspot.com/2011/05/ccache-and-clang.html.
		if ctx.Config().IsEnvTrue("USE_CCACHE") {
			flags = append(flags, "-Wno-unused-command-line-argument")
		}

		if ctx.Config().IsEnvTrue("ALLOW_UNKNOWN_WARNING_OPTION") {
			flags = append(flags, "-Wno-error=unknown-warning-option")
		}

		switch ctx.Config().Getenv("CLANG_DEFAULT_DEBUG_LEVEL") {
		case "debug_level_0":
			flags = append(flags, "-g0")
		case "debug_level_1":
			flags = append(flags, "-g1")
		case "debug_level_2":
			flags = append(flags, "-g2")
		case "debug_level_3":
			flags = append(flags, "-g3")
		case "debug_level_g":
			flags = append(flags, "-g")
		default:
			flags = append(flags, "-g")
		}

		return strings.Join(flags, " ")
	})

	// Export the static default DeviceGlobalCflags to Bazel.
	// TODO(187086342): handle cflags that are set in VariableFuncs.
	exportedVars.ExportStringList("DeviceGlobalCflags", deviceGlobalCflags)

	pctx.VariableFunc("DeviceGlobalCflags", func(ctx android.PackageVarContext) string {
		return strings.Join(deviceGlobalCflags, " ")
	})

	// Export the static default NoOverrideGlobalCflags to Bazel.
	exportedVars.ExportStringList("NoOverrideGlobalCflags", noOverrideGlobalCflags)
	pctx.VariableFunc("NoOverrideGlobalCflags", func(ctx android.PackageVarContext) string {
		flags := noOverrideGlobalCflags
		if ctx.Config().IsEnvTrue("LLVM_NEXT") {
			flags = append(noOverrideGlobalCflags, llvmNextExtraCommonGlobalCflags...)
			IllegalFlags = []string{} // Don't fail build while testing a new compiler.
		}
		return strings.Join(flags, " ")
	})

	exportedVars.ExportStringListStaticVariable("NoOverride64GlobalCflags", noOverride64GlobalCflags)
	exportedVars.ExportStringListStaticVariable("HostGlobalCflags", hostGlobalCflags)
	exportedVars.ExportStringListStaticVariable("NoOverrideExternalGlobalCflags", noOverrideExternalGlobalCflags)
	exportedVars.ExportStringListStaticVariable("CommonGlobalCppflags", commonGlobalCppflags)
	exportedVars.ExportStringListStaticVariable("ExternalCflags", extraExternalCflags)

	exportedVars.ExportString("CStdVersion", CStdVersion)
	exportedVars.ExportString("CppStdVersion", CppStdVersion)
	exportedVars.ExportString("ExperimentalCStdVersion", ExperimentalCStdVersion)
	exportedVars.ExportString("ExperimentalCppStdVersion", ExperimentalCppStdVersion)

	exportedVars.ExportString("VersionScriptFlagPrefix", VersionScriptFlagPrefix)

	exportedVars.ExportString("VisibilityHiddenFlag", VisibilityHiddenFlag)
	exportedVars.ExportString("VisibilityDefaultFlag", VisibilityDefaultFlag)

	// Everything in these lists is a crime against abstraction and dependency tracking.
	// Do not add anything to this list.
	commonGlobalIncludes := []string{
		"system/core/include",
		"system/logging/liblog/include",
		"system/media/audio/include",
		"hardware/libhardware/include",
		"hardware/libhardware_legacy/include",
		"hardware/ril/include",
		"frameworks/native/include",
		"frameworks/native/opengl/include",
		"frameworks/av/include",
	}
	exportedVars.ExportStringList("CommonGlobalIncludes", commonGlobalIncludes)
	pctx.PrefixedExistentPathsForSourcesVariable("CommonGlobalIncludes", "-I", commonGlobalIncludes)

	exportedVars.ExportStringStaticVariable("CLANG_DEFAULT_VERSION", ClangDefaultVersion)
	exportedVars.ExportStringStaticVariable("CLANG_DEFAULT_SHORT_VERSION", ClangDefaultShortVersion)

	pctx.StaticVariableWithEnvOverride("ClangBase", "LLVM_PREBUILTS_BASE", ClangDefaultBase)
	pctx.StaticVariableWithEnvOverride("ClangVersion", "LLVM_PREBUILTS_VERSION", ClangDefaultVersion)
	pctx.StaticVariable("ClangPath", "${ClangBase}/${HostPrebuiltTag}/${ClangVersion}")
	pctx.StaticVariable("ClangBin", "${ClangPath}/bin")

	pctx.StaticVariableWithEnvOverride("ClangShortVersion", "LLVM_RELEASE_VERSION", ClangDefaultShortVersion)
	pctx.StaticVariable("ClangAsanLibDir", "${ClangBase}/linux-x86/${ClangVersion}/lib/clang/${ClangShortVersion}/lib/linux")

	exportedVars.ExportStringListStaticVariable("WarningAllowedProjects", WarningAllowedProjects)

	// These are tied to the version of LLVM directly in external/llvm, so they might trail the host prebuilts
	// being used for the rest of the build process.
	pctx.SourcePathVariable("RSClangBase", "prebuilts/clang/host")
	pctx.SourcePathVariable("RSClangVersion", "clang-3289846")
	pctx.SourcePathVariable("RSReleaseVersion", "3.8")
	pctx.StaticVariable("RSLLVMPrebuiltsPath", "${RSClangBase}/${HostPrebuiltTag}/${RSClangVersion}/bin")
	pctx.StaticVariable("RSIncludePath", "${RSLLVMPrebuiltsPath}/../lib64/clang/${RSReleaseVersion}/include")

	rsGlobalIncludes := []string{
		"external/clang/lib/Headers",
		"frameworks/rs/script_api/include",
	}
	pctx.PrefixedExistentPathsForSourcesVariable("RsGlobalIncludes", "-I", rsGlobalIncludes)
	exportedVars.ExportStringList("RsGlobalIncludes", rsGlobalIncludes)

	pctx.VariableFunc("CcWrapper", func(ctx android.PackageVarContext) string {
		if override := ctx.Config().Getenv("CC_WRAPPER"); override != "" {
			return override + " "
		}
		return ""
	})

	pctx.StaticVariableWithEnvOverride("RECXXPool", "RBE_CXX_POOL", remoteexec.DefaultPool)
	pctx.StaticVariableWithEnvOverride("RECXXLinksPool", "RBE_CXX_LINKS_POOL", remoteexec.DefaultPool)
	pctx.StaticVariableWithEnvOverride("REClangTidyPool", "RBE_CLANG_TIDY_POOL", remoteexec.DefaultPool)
	pctx.StaticVariableWithEnvOverride("RECXXLinksExecStrategy", "RBE_CXX_LINKS_EXEC_STRATEGY", remoteexec.LocalExecStrategy)
	pctx.StaticVariableWithEnvOverride("REClangTidyExecStrategy", "RBE_CLANG_TIDY_EXEC_STRATEGY", remoteexec.LocalExecStrategy)
	pctx.StaticVariableWithEnvOverride("REAbiDumperExecStrategy", "RBE_ABI_DUMPER_EXEC_STRATEGY", remoteexec.LocalExecStrategy)
	pctx.StaticVariableWithEnvOverride("REAbiLinkerExecStrategy", "RBE_ABI_LINKER_EXEC_STRATEGY", remoteexec.LocalExecStrategy)
}

var HostPrebuiltTag = exportedVars.ExportVariableConfigMethod("HostPrebuiltTag", android.Config.PrebuiltOS)

func ClangPath(ctx android.PathContext, file string) android.SourcePath {
	type clangToolKey string

	key := android.NewCustomOnceKey(clangToolKey(file))

	return ctx.Config().OnceSourcePath(key, func() android.SourcePath {
		return clangPath(ctx).Join(ctx, file)
	})
}

var clangPathKey = android.NewOnceKey("clangPath")

func clangPath(ctx android.PathContext) android.SourcePath {
	return ctx.Config().OnceSourcePath(clangPathKey, func() android.SourcePath {
		clangBase := ClangDefaultBase
		if override := ctx.Config().Getenv("LLVM_PREBUILTS_BASE"); override != "" {
			clangBase = override
		}
		clangVersion := ClangDefaultVersion
		if override := ctx.Config().Getenv("LLVM_PREBUILTS_VERSION"); override != "" {
			clangVersion = override
		}
		return android.PathForSource(ctx, clangBase, ctx.Config().PrebuiltOS(), clangVersion)
	})
}
