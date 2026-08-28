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
	"slices"
	"strconv"
	"strings"

	"android/soong/android"
	"android/soong/remoteexec"
)

var (
	pctx = android.NewPackageContext("android/soong/cc/config")

	// Flags used by lots of devices.  Putting them in package static variables
	// will save bytes in build.ninja so they aren't repeated for every file
	commonGlobalCflags = []string{
		// Enable some optimization by default.
		"-O2",

		// Warnings enabled by default. Reference:
		// https://clang.llvm.org/docs/DiagnosticsReference.html
		"-Wall",
		"-Wextra",
		"-Wpointer-arith",
		"-Wunguarded-availability",

		// Warnings treated as errors by default.
		// See also noOverrideGlobalCflags for errors that cannot be disabled
		// from Android.bp files.

		// Detects usage of bitwise operators on Boolean values.
		"-Werror=bool-operation",
		// Using __DATE__/__TIME__ causes build nondeterminism.
		"-Werror=date-time",
		// Bans format strings in printf-like functions that are not known at
		// compile time and cannot be checked against their arguments.
		"-Werror=format-security",
		// Detects forgotten */& that usually cause a crash
		"-Werror=int-conversion",
		// Detects multi-character constants such as 'abcd', which have
		// implementation-defined values. Usually typos and may cause bugs.
		"-Werror=multichar",
		// Detects unterminated alignment modification pragmas, which often lead
		// to ABI mismatch between modules and hard-to-debug crashes.
		"-Werror=pragma-pack",
		// Same as above, but detects alignment pragmas around a header
		// inclusion.
		"-Werror=pragma-pack-suspicious-include",
		// Detects dividing an array size by itself, which is a common typo that
		// leads to bugs.
		"-Werror=sizeof-array-div",
		// Detects code like 'memcpy(dest, src, sizeof(src))' where 'src' is a pointer.
		// This is nearly always a bug that may cause memory corruption.
		"-Werror=sizeof-pointer-memaccess",
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

		// We should encourage use of C23 features even when the whole project
		// isn't C23-ready.
		"-Wno-c23-extensions",
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
		"-Wa,--noexecstack",
		"-D_FORTIFY_SOURCE=3",

		// Add stack canaries on every frame, checked on return.
		// -fstack-clash-protection isn't supported in clang for ILP32,
		// so that's in each of the LP64 architectures' configurations instead.
		"-fstack-protector-strong",

		// Bans classes that have virtual functions and a public non-virtual destructor.
		// This potentially allows the class to be partially destroyed, causing memory
		// corruption.
		"-Werror=non-virtual-dtor",
		// Detects invalid comparisons involving pointers that always give the same
		// result. This is almost always a bug.
		"-Werror=address",
		// Detects stuff like 'x++ = x++ * x++;', which has undefined effects.
		"-Werror=sequence-point",
	}

	commonGlobalLdflags = []string{
		"-fuse-ld=lld",
		"-Wl,--icf=safe",
		"-Wl,--no-demangle",
	}

	deviceGlobalCppflags = []string{
		"-fvisibility-inlines-hidden",
	}

	// Linking flags for device code; not applied to host binaries.
	deviceGlobalLdflags = slices.Concat([]string{
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
		"-Wl,--compress-debug-sections=zstd",
	}, commonGlobalLdflags)

	hostGlobalCflags = []string{}

	hostGlobalCppflags = []string{}

	hostGlobalLdflags = commonGlobalLdflags

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
	// NOTE: Some warnings are disabled in this section (and
	// noOverrideExternalGlobalCflags below). If they were disabled in
	// commonGlobalCflags (and externalCflags below), these disabled flags will
	// be overridden when modules that opt into`-Wall`, `-Wextra` in their
	// `cflags`.  (TODO(b/444266638) to remove and forbid these flags from Android.bp.
	//
	// If you need to disable a warning to unblock a compiler upgrade
	// and it is only triggered by third party code, add it to
	// extraExternalCflags (if possible) or noOverrideExternalGlobalCflags
	// (if the former doesn't work). If the new warning also occurs in first
	// party code, try adding it to commonGlobalCflags first. Adding it here
	// should be the last resort, because it prevents all code in Android from
	// opting into the warning.
	noOverrideGlobalCflags = []string{
		// Force treating some high severity warnings as errors.

		// Detects things like 'int* x = &(a + b)'. This is an error by default,
		// but we don't want anyone to disable it from Android.bp.
		"-Werror=address-of-temporary",
		// Bundle of warnings that detects expressions which create dangling
		// pointers. They are always bugs with high risk of memory corruption.
		"-Werror=dangling",
		// Detects printf-like functions with fewer arguments than required by
		// the format string. Such calls usually print stack garbage and may crash.
		"-Werror=format-insufficient-args",
		// Detects some buffer overflow bugs involving C standard library functions.
		"-Werror=fortify-source",
		// Detects assignments between function pointers of incompatible types,
		// which are allowed by the standard, but are almost always bugs with
		// memory corruption potential.
		"-Werror=incompatible-function-pointer-types",
		// Detects suspicious uses of integers and arithmetic expressions in Boolean
		// contexts, such as if statements, while loops, the ternary operator ?:, etc.
		"-Werror=int-in-bool-context",
		// Detects casts from integers smaller than a pointer to pointers, which
		// usually indicates address truncation in code that's not 64-bit compatible.
		"-Werror=int-to-pointer-cast",
		// Self-explanatory, this is always undefined behavior.
		"-Werror=null-dereference",
		// Detects missing or inconsistent return statements in functions.
		// The function will return garbage, which can lead to crashes and memory
		// corruption.
		"-Werror=return-type",
		// Detects the typos 2^x and 10^x, where x is a decimal constant.
		"-Werror=xor-used-as-pow",

		// Disable some warnings to unblock compiler upgrades. All of the flags below
		// should eventually be removed or moved to other sections.

		// Disabled because it produces many false positives. http://b/323050926
		"-Wno-missing-field-initializers",
		// http://b/323050889
		"-Wno-packed-non-pod",

		// http://b/72331526 Disable -Wtautological-* until the instances detected by these
		// new warnings are fixed.
		"-Wno-error=tautological-constant-compare",
		// http://b/145211066
		"-Wno-implicit-int-float-conversion",
		// New warnings to be fixed after clang-r377782.
		"-Wno-tautological-overlap-compare", // http://b/148815696
		// New warnings to be fixed after clang-r383902.
		"-Wno-deprecated-copy",                      // http://b/153746672
		"-Wno-zero-as-null-pointer-constant",        // http://b/68236239
		"-Wno-deprecated-anon-enum-enum-conversion", // http://b/153746485
		"-Wno-deprecated-enum-enum-conversion",
		"-Wno-error=pessimizing-move", // http://b/154270751
		// New warnings to be fixed after clang-r399163
		"-Wno-non-c-typedef-for-linkage", // http://b/161304145
		// New warnings to be fixed after clang-r428724
		"-Wno-align-mismatch", // http://b/193679946
		// New warnings to be fixed after clang-r433403
		"-Wno-error=unused-but-set-parameter", // http://b/197240255
		// New warnings to be fixed after clang-r468909
		"-Wno-error=deprecated", // in external/googletest/googletest
		// New warnings to be fixed after clang-r522817
		"-Wno-error=invalid-offsetof",
		// New warnings to be fixed after clang-r563880
		"-Wno-nontrivial-memcall",
		"-Wno-invalid-specialization",
		// New warnings to be fixed after clang-r574158
		"-Wno-unterminated-string-initialization",
		"-Wno-implicit-int-conversion-on-negation",
		"-Wno-default-const-init-field-unsafe",
		"-Wno-default-const-init-var-unsafe",
		"-Wno-preferred-type-bitfield-enum-conversion",
		"-Wno-implicit-enum-enum-cast",
		// New warnings to be fixed after clang-r584948
		"-Wno-character-conversion", // http://b/452740154
		// TODO: Disable this warning in external projects after switching clang.
		"-Wno-error=uninitialized-const-pointer", // http://b/458489157
		// New warnings to be fixed after clang-r596125
		"-Wno-incompatible-pointer-types", // http://b/490481169
		"-Wno-c2y-extensions",             // http://b/493691159

		// Allow using VLA CXX extension.
		"-Wno-vla-cxx-extension",
		"-Wno-cast-function-type-mismatch",
	}

	noOverride64GlobalCflags = []string{}

	// Extra cflags applied to tests code.
	extraTestsCflags = []string{
		"-Wno-error=unused-but-set-variable",
	}

	// This is similar to noOverrideGlobalCflags, but applies only to tests
	// code. This section can unblock compiler upgrades when a test module
	// that enables -Wall, -Wextra, or a particular warnings explicitly triggers
	// newly added warnings. See note above noOverrideGlobalCflags.
	noOverrideTestsGlobalCflags = []string{
		"-Wno-unused-variable",
	}

	// Extra cflags applied to third-party code (anything for which
	// IsThirdPartyPath() in build/soong/android/paths.go returns true;
	// includes external/, most of vendor/ and most of hardware/)
	extraExternalCflags = []string{
		"-Wno-enum-compare",
		"-Wno-enum-compare-switch",

		// http://b/72331524 Allow null pointer arithmetic until the instances detected by
		// this new warning are fixed.
		"-Wno-null-pointer-arithmetic",

		// http://b/199369603
		"-Wno-null-pointer-subtraction",

		// http://b/175068488
		"-Wno-string-concatenation",

		// http://b/239661264
		"-Wno-deprecated-non-prototype",

		"-Wno-unused",
		"-Wno-unused-but-set-variable",
		"-Wno-deprecated",
		"-Wno-tautological-constant-compare",
	}

	// This is similar to noOverrideGlobalCflags, but applies only to third-party
	// code. This section can unblock compiler upgrades when a third party module
	// that enables -Wall, -Wextra, or a particular warnings explicitly triggers
	// newly added warnings. See note above noOverrideGlobalCflags.
	noOverrideExternalGlobalCflags = []string{
		// http://b/191699019
		"-Wno-format-insufficient-args",
		// http://b/296321508
		// Introduced in response to a critical security vulnerability and
		// should be a hard error - it requires only whitespace changes to fix.
		"-Wno-misleading-indentation",

		"-Wno-unused",
		"-Wno-unused-parameter",
		"-Wno-unused-but-set-parameter",
		"-Wno-unused-variable",
		"-Wno-unqualified-std-cast-call",
		"-Wno-array-parameter",
		"-Wno-gnu-offsetof-extensions",
		"-Wno-pessimizing-move",

		// http://b/161386391 adding -Werror=pointer-to-int-cast, which
		// also controls -Wvoid-pointer-to-int-cast, -Wpointer-to-enum-cast
		// and -Wvoid-pointer-to-enum-cast
		"-Wno-pointer-to-int-cast",
	}

	llvmNextExtraCommonGlobalCflags = []string{
		// Do not report warnings when testing with the top of trunk LLVM.
		"-Wno-everything",
	}

	// Flags that must not appear in any command line.
	IllegalFlags = []string{
		"-w",
		"-pedantic",
		"-pedantic-errors",
		"-Werror=pedantic",
		"-Wno-all",
		"-Wno-everything",
	}

	CStdVersion               = "gnu23"
	CppDefaultStdVersion      = "gnu++20"
	ExperimentalCStdVersion   = "gnu2y"
	ExperimentalCppStdVersion = "gnu++2b"

	// prebuilts/clang default settings.
	ClangDefaultBase = "prebuilts/clang/host"
	// The Clang version used in the trunk branch.
	// NOTE: This is deprecated and will be removed in a future version, use the getter function instead.
	ClangDefaultVersion = "clang-r596125"

	RsGlobalIncludes = []string{
		"external/clang/lib/Headers",
		"frameworks/rs/script_api/include",
	}

	// Directories with warnings from Android.bp files.
	WarningAllowedProjects = []string{
		"device/",
		"vendor/",
	}

	VersionScriptFlagPrefix = "-Wl,--version-script,"

	VisibilityHiddenFlag  = "-fvisibility=hidden"
	VisibilityDefaultFlag = "-fvisibility=default"
)

func init() {
	if runtime.GOOS == "linux" {
		commonGlobalCflags = append(commonGlobalCflags, "-fdebug-prefix-map=/proc/self/cwd=")
	}
	if runtime.GOOS == "darwin" {
		commonGlobalCflags = append(commonGlobalCflags, "-Wno-unguarded-availability")
	}

	pctx.StaticVariable("CommonGlobalConlyflags", strings.Join(commonGlobalConlyflags, " "))
	pctx.StaticVariable("CommonGlobalAsflags", strings.Join(commonGlobalAsflags, " "))
	pctx.StaticVariable("DeviceGlobalCppflags", strings.Join(deviceGlobalCppflags, " "))
	pctx.StaticVariable("DeviceGlobalLdflags", strings.Join(deviceGlobalLdflags, " "))
	pctx.StaticVariable("HostGlobalCppflags", strings.Join(hostGlobalCppflags, " "))
	pctx.StaticVariable("HostGlobalLdflags", strings.Join(hostGlobalLdflags, " "))

	pctx.VariableFunc("CommonGlobalCflags", func(ctx android.PackageVarContext) string {
		flags := slices.Clone(commonGlobalCflags)

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

	pctx.VariableFunc("DeviceGlobalCflags", func(ctx android.PackageVarContext) string {
		return strings.Join(deviceGlobalCflags, " ")
	})

	pctx.VariableFunc("NoOverrideGlobalCflags", func(ctx android.PackageVarContext) string {
		flags := noOverrideGlobalCflags
		if ctx.Config().IsEnvTrue("LLVM_NEXT") {
			flags = append(noOverrideGlobalCflags, llvmNextExtraCommonGlobalCflags...)
			IllegalFlags = []string{} // Don't fail build while testing a new compiler.
		}
		return strings.Join(flags, " ")
	})

	pctx.StaticVariable("NoOverride64GlobalCflags", strings.Join(noOverride64GlobalCflags, " "))
	pctx.StaticVariable("HostGlobalCflags", strings.Join(hostGlobalCflags, " "))
	pctx.VariableFunc("NoOverrideExternalGlobalCflags", func(ctx android.PackageVarContext) string {
		flags := noOverrideExternalGlobalCflags
		if !ctx.Config().ReleaseUseFnoCommonFor3pCode() {
			flags = append(flags, "-fcommon")
		}
		return strings.Join(flags, " ")
	})
	pctx.StaticVariable("CommonGlobalCppflags", strings.Join(commonGlobalCppflags, " "))
	pctx.StaticVariable("NoOverrideTestsGlobalCflags", strings.Join(noOverrideTestsGlobalCflags, " "))
	pctx.StaticVariable("TestsCflags", strings.Join(extraTestsCflags, " "))
	pctx.StaticVariable("ExternalCflags", strings.Join(extraExternalCflags, " "))

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
	pctx.PrefixedExistentPathsForSourcesVariable("CommonGlobalIncludes", "-I", commonGlobalIncludes)

	pctx.StaticVariableWithEnvOverride("ClangBase", "LLVM_PREBUILTS_BASE", ClangDefaultBase)
	pctx.VariableFunc("ClangVersion", func(ctx android.PackageVarContext) string {
		if override := ctx.Config().Getenv("LLVM_PREBUILTS_VERSION"); override != "" {
			return override
		}
		return ClangVersion(ctx)
	})
	pctx.VariableFunc("CppStdVersion", func(ctx android.PackageVarContext) string {
		return ctx.Config().ReleaseBuildCppStdVersion(CppDefaultStdVersion)
	})

	pctx.StaticVariable("ClangPath", "${ClangBase}/${HostPrebuiltTag}/${ClangVersion}")
	pctx.StaticVariable("ClangBin", "${ClangPath}/bin")

	pctx.StaticVariable("WarningAllowedProjects", strings.Join(WarningAllowedProjects, " "))

	// These are tied to the version of LLVM directly in external/llvm, so they might trail the host prebuilts
	// being used for the rest of the build process.
	pctx.SourcePathVariable("RSClangBase", "prebuilts/clang/host")
	pctx.SourcePathVariable("RSClangVersion", "clang-3289846")
	pctx.SourcePathVariable("RSReleaseVersion", "3.8")
	pctx.StaticVariable("RSLLVMPrebuiltsPath", "${RSClangBase}/${HostPrebuiltTag}/${RSClangVersion}/bin")
	pctx.StaticVariable("RSIncludePath", "${RSLLVMPrebuiltsPath}/../lib64/clang/${RSReleaseVersion}/include")

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

var HostPrebuiltTag = pctx.VariableConfigMethod("HostPrebuiltTag", android.Config.PrebuiltOS)

// ClangPath returns the path to a tool in the LLVM prebuilts directory.
//
// The environment variable configuration is cached via a config.Once(), but the path
// recalculation via PathForSource() is performed every time. This is needed because
// in incremental soong, the PathForSource() calls may add missing dependencies to a
// module, and we don't want it to be nondeterministically whichever module calls this first.
func ClangPath(ctx android.PathContext, file string) android.SourcePath {
	return clangPath(ctx).Join(ctx, file)
}

var clangPathKey = android.NewOnceKey("clangPath")

type clangPathResult struct {
	clangBase    string
	clangVersion string
}

func clangPath(ctx android.PathContext) android.SourcePath {
	res := ctx.Config().Once(clangPathKey, func() interface{} {
		clangBase := ClangDefaultBase
		if override := ctx.Config().Getenv("LLVM_PREBUILTS_BASE"); override != "" {
			clangBase = override
		}
		clangVersion := ClangVersion(ctx)
		if override := ctx.Config().Getenv("LLVM_PREBUILTS_VERSION"); override != "" {
			clangVersion = override
		}
		return clangPathResult{clangBase, clangVersion}
	}).(clangPathResult)
	return android.PathForSource(ctx, res.clangBase, ctx.Config().PrebuiltOS(), res.clangVersion)
}

func ClangVersion(ctx android.PathContext) string {
	return ctx.Config().ReleaseBuildClangVersion(ClangDefaultVersion)
}

func CppStdVersion(ctx android.PathContext) string {
	return ctx.Config().ReleaseBuildCppStdVersion(CppDefaultStdVersion)
}

// Check if the Clang revision is greater or equal to minRev. Returns false if failed to parse.
func ClangVersionAtLeast(ctx android.PathContext, minRev int) bool {
	curRevStr := ClangVersion(ctx)
	// Slice the string to keep only the digits (e.g., "584948")
	if !strings.HasPrefix(curRevStr, "clang-r") {
		return false
	}
	i := 7
	for i < len(curRevStr) && curRevStr[i] >= '0' && curRevStr[i] <= '9' {
		i++
	}
	curRev, err := strconv.Atoi(curRevStr[7:i])
	if err != nil {
		return false
	}
	return curRev >= minRev
}
