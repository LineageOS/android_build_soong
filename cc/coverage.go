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
	"strconv"

	"github.com/google/blueprint"

	"android/soong/android"
)

const profileInstrFlag = "-fprofile-instr-generate=/data/misc/trace/clang-%p-%m.profraw"

type CoverageProperties struct {
	Native_coverage *bool

	NeedCoverageVariant bool `blueprint:"mutated"`
	NeedCoverageBuild   bool `blueprint:"mutated"`

	CoverageEnabled   bool `blueprint:"mutated"`
	IsCoverageVariant bool `blueprint:"mutated"`
}

type coverage struct {
	Properties CoverageProperties

	// Whether binaries containing this module need --coverage added to their ldflags
	linkCoverage bool
}

func (cov *coverage) props() []interface{} {
	return []interface{}{&cov.Properties}
}

func getGcovProfileLibraryName(ctx ModuleContextIntf) string {
	// This function should only ever be called for a cc.Module, so the
	// following statement should always succeed.
	// LINT.IfChange
	if ctx.useSdk() {
		return "libprofile-extras_ndk"
	} else {
		return "libprofile-extras"
	}
}

func getClangProfileLibraryName(ctx ModuleContextIntf) string {
	if ctx.useSdk() {
		return "libprofile-clang-extras_ndk"
	} else if ctx.isCfiAssemblySupportEnabled() {
		return "libprofile-clang-extras_cfi_support"
	} else {
		return "libprofile-clang-extras"
	}
	// LINT.ThenChange(library.go)
}

func (cov *coverage) deps(ctx DepsContext, deps Deps) Deps {
	if cov.Properties.NeedCoverageVariant {
		ctx.AddVariationDependencies([]blueprint.Variation{
			{Mutator: "link", Variation: "static"},
		}, CoverageDepTag, getGcovProfileLibraryName(ctx))
		ctx.AddVariationDependencies([]blueprint.Variation{
			{Mutator: "link", Variation: "static"},
		}, CoverageDepTag, getClangProfileLibraryName(ctx))
	}
	return deps
}

func EnableContinuousCoverage(ctx android.BaseModuleContext) bool {
	return ctx.DeviceConfig().ClangCoverageContinuousMode()
}

func (cov *coverage) flags(ctx ModuleContext, flags Flags, deps PathDeps) (Flags, PathDeps) {
	clangCoverage := ctx.DeviceConfig().ClangCoverageEnabled()
	gcovCoverage := ctx.DeviceConfig().GcovCoverageEnabled()

	if !gcovCoverage && !clangCoverage {
		return flags, deps
	}

	if cov.Properties.CoverageEnabled {
		cov.linkCoverage = true

		if gcovCoverage {
			flags.GcovCoverage = true
			flags.Local.CommonFlags = append(flags.Local.CommonFlags, "--coverage", "-O0")

			// Override -Wframe-larger-than and non-default optimization
			// flags that the module may use.
			flags.Local.CFlags = append(flags.Local.CFlags, "-Wno-frame-larger-than=", "-O0")
		} else if clangCoverage {
			flags.Local.CommonFlags = append(flags.Local.CommonFlags, profileInstrFlag,
				"-fcoverage-mapping", "-Wno-pass-failed", "-D__ANDROID_CLANG_COVERAGE__")
			// Override -Wframe-larger-than.  We can expect frame size increase after
			// coverage instrumentation.
			flags.Local.CFlags = append(flags.Local.CFlags, "-Wno-frame-larger-than=")
			if EnableContinuousCoverage(ctx) {
				flags.Local.CommonFlags = append(flags.Local.CommonFlags, "-mllvm", "-runtime-counter-relocation")
			}

			// http://b/248022906, http://b/247941801  enabling coverage and hwasan-globals
			// instrumentation together causes duplicate-symbol errors for __llvm_profile_filename.
			if c, ok := ctx.Module().(*Module); ok && c.sanitize.isSanitizerEnabled(Hwasan) {
				flags.Local.CommonFlags = append(flags.Local.CommonFlags, "-mllvm", "-hwasan-globals=0")
			}
		}
	}

	// Even if we don't have coverage enabled, if any of our object files were compiled
	// with coverage, then we need to add --coverage to our ldflags.
	if !cov.linkCoverage {
		if ctx.static() && !ctx.staticBinary() {
			// For static libraries, the only thing that changes our object files
			// are included whole static libraries, so check to see if any of
			// those have coverage enabled.
			ctx.VisitDirectDeps(func(m android.Module) {
				if depTag, ok := ctx.OtherModuleDependencyTag(m).(libraryDependencyTag); ok {
					if depTag.static() && depTag.wholeStatic {
						if cc, ok := m.(*Module); ok && cc.coverage != nil {
							if cc.coverage.linkCoverage {
								cov.linkCoverage = true
							}
						}
					}
				}
			})
		} else {
			// For executables and shared libraries, we need to check all of
			// our static dependencies.
			ctx.VisitDirectDeps(func(m android.Module) {
				cc, ok := m.(*Module)
				if !ok || cc.coverage == nil {
					return
				}

				if static, ok := cc.linker.(libraryInterface); !ok || !static.static() {
					return
				}

				if cc.coverage.linkCoverage {
					cov.linkCoverage = true
				}
			})
		}
	}

	if cov.linkCoverage {
		if gcovCoverage {
			flags.Local.LdFlags = append(flags.Local.LdFlags, "--coverage")

			coverage := ctx.GetDirectDepWithTag(getGcovProfileLibraryName(ctx), CoverageDepTag).(*Module)
			deps.WholeStaticLibs = append(deps.WholeStaticLibs, coverage.OutputFile().Path())

			flags.Local.LdFlags = append(flags.Local.LdFlags, "-Wl,--wrap,getenv")
		} else if clangCoverage {
			flags.Local.LdFlags = append(flags.Local.LdFlags, profileInstrFlag)
			if EnableContinuousCoverage(ctx) {
				flags.Local.LdFlags = append(flags.Local.LdFlags, "-Wl,-mllvm=-runtime-counter-relocation")
			}

			coverage := ctx.GetDirectDepWithTag(getClangProfileLibraryName(ctx), CoverageDepTag).(*Module)
			deps.WholeStaticLibs = append(deps.WholeStaticLibs, coverage.OutputFile().Path())
			flags.Local.LdFlags = append(flags.Local.LdFlags, "-Wl,--wrap,open")
		}
	}

	return flags, deps
}

func (cov *coverage) begin(ctx BaseModuleContext) {
	if ctx.Host() {
		// TODO(dwillemsen): because of -nodefaultlibs, we must depend on libclang_rt.profile-*.a
		// Just turn off for now.
	} else {
		cov.Properties = SetCoverageProperties(ctx, cov.Properties, ctx.nativeCoverage(), ctx.useSdk(), ctx.sdkVersion())
	}
}

func SetCoverageProperties(ctx android.BaseModuleContext, properties CoverageProperties, moduleTypeHasCoverage bool,
	useSdk bool, sdkVersion string) CoverageProperties {
	// Coverage is disabled globally
	if !ctx.DeviceConfig().NativeCoverageEnabled() {
		return properties
	}

	var needCoverageVariant bool
	var needCoverageBuild bool

	if moduleTypeHasCoverage {
		// Check if Native_coverage is set to false.  This property defaults to true.
		needCoverageVariant = BoolDefault(properties.Native_coverage, true)
		if useSdk && sdkVersion != "current" {
			// Native coverage is not supported for SDK versions < 23
			if fromApi, err := strconv.Atoi(sdkVersion); err == nil && fromApi < 23 {
				needCoverageVariant = false
			}
		}

		if needCoverageVariant {
			// Coverage variant is actually built with coverage if enabled for its module path
			needCoverageBuild = ctx.DeviceConfig().NativeCoverageEnabledForPath(ctx.ModuleDir())
		}
	}

	properties.NeedCoverageBuild = needCoverageBuild
	properties.NeedCoverageVariant = needCoverageVariant

	return properties
}

type UseCoverage interface {
	android.Module
	IsNativeCoverageNeeded(ctx android.BaseModuleContext) bool
}

// Coverage is an interface for non-CC modules to implement to be mutated for coverage
type Coverage interface {
	UseCoverage
	SetPreventInstall()
	HideFromMake()
	MarkAsCoverageVariant(bool)
	EnableCoverageIfNeeded()
}

func coverageMutator(mctx android.BottomUpMutatorContext) {
	if c, ok := mctx.Module().(*Module); ok && c.coverage != nil {
		needCoverageVariant := c.coverage.Properties.NeedCoverageVariant
		needCoverageBuild := c.coverage.Properties.NeedCoverageBuild
		if needCoverageVariant {
			m := mctx.CreateVariations("", "cov")

			// Setup the non-coverage version and set HideFromMake and
			// PreventInstall to true.
			m[0].(*Module).coverage.Properties.CoverageEnabled = false
			m[0].(*Module).coverage.Properties.IsCoverageVariant = false
			m[0].(*Module).Properties.HideFromMake = true
			m[0].(*Module).Properties.PreventInstall = true

			// The coverage-enabled version inherits HideFromMake,
			// PreventInstall from the original module.
			m[1].(*Module).coverage.Properties.CoverageEnabled = needCoverageBuild
			m[1].(*Module).coverage.Properties.IsCoverageVariant = true
		}
	} else if cov, ok := mctx.Module().(Coverage); ok && cov.IsNativeCoverageNeeded(mctx) {
		// APEX and Rust modules fall here

		// Note: variant "" is also created because an APEX can be depended on by another
		// module which are split into "" and "cov" variants. e.g. when cc_test refers
		// to an APEX via 'data' property.
		m := mctx.CreateVariations("", "cov")
		m[0].(Coverage).MarkAsCoverageVariant(false)
		m[0].(Coverage).SetPreventInstall()
		m[0].(Coverage).HideFromMake()

		m[1].(Coverage).MarkAsCoverageVariant(true)
		m[1].(Coverage).EnableCoverageIfNeeded()
	} else if cov, ok := mctx.Module().(UseCoverage); ok && cov.IsNativeCoverageNeeded(mctx) {
		// Module itself doesn't have to have "cov" variant, but it should use "cov" variants of
		// deps.
		mctx.CreateVariations("cov")
		mctx.AliasVariation("cov")
	}
}

func parseSymbolFileForAPICoverage(ctx ModuleContext, symbolFile string) android.ModuleOutPath {
	apiLevelsJson := android.GetApiLevelsJson(ctx)
	symbolFilePath := android.PathForModuleSrc(ctx, symbolFile)
	outputFile := ctx.baseModuleName() + ".xml"
	parsedApiCoveragePath := android.PathForModuleOut(ctx, outputFile)
	rule := android.NewRuleBuilder(pctx, ctx)
	rule.Command().
		BuiltTool("ndk_api_coverage_parser").
		Input(symbolFilePath).
		Output(parsedApiCoveragePath).
		Implicit(apiLevelsJson).
		FlagWithArg("--api-map ", apiLevelsJson.String())
	rule.Build("native_library_api_list", "Generate native API list based on symbol files for coverage measurement")
	return parsedApiCoveragePath
}
