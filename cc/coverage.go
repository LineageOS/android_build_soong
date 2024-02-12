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

var (
 	clangCoverageHostLdFlags = []string{
 		"-Wl,--no-as-needed",
 		"-Wl,--wrap,open",
 	}
 	clangContinuousCoverageFlags = []string{
 		"-mllvm",
 		"-runtime-counter-relocation",
 	}
 	clangCoverageCFlags = []string{
 		"-Wno-frame-larger-than=",
 	}
 	clangCoverageCommonFlags = []string{
 		"-fcoverage-mapping",
 		"-Wno-pass-failed",
 		"-D__ANDROID_CLANG_COVERAGE__",
 	}
 	clangCoverageHWASanFlags = []string{
 		"-mllvm",
 		"-hwasan-globals=0",
 	}
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
	if cov.Properties.NeedCoverageVariant && ctx.Device() {
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
			flags.Local.CommonFlags = append(flags.Local.CommonFlags, profileInstrFlag)
			flags.Local.CommonFlags = append(flags.Local.CommonFlags, clangCoverageCommonFlags...)
			// Override -Wframe-larger-than.  We can expect frame size increase after
			// coverage instrumentation.
			flags.Local.CFlags = append(flags.Local.CFlags, clangCoverageCFlags...)
			if EnableContinuousCoverage(ctx) {
				flags.Local.CommonFlags = append(flags.Local.CommonFlags, clangContinuousCoverageFlags...)
			}

			// http://b/248022906, http://b/247941801  enabling coverage and hwasan-globals
			// instrumentation together causes duplicate-symbol errors for __llvm_profile_filename.
			if c, ok := ctx.Module().(*Module); ok && c.sanitize.isSanitizerEnabled(Hwasan) {
				flags.Local.CommonFlags = append(flags.Local.CommonFlags, clangCoverageHWASanFlags...)
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

			if ctx.Device() {
				coverage := ctx.GetDirectDepWithTag(getGcovProfileLibraryName(ctx), CoverageDepTag).(*Module)
				deps.WholeStaticLibs = append(deps.WholeStaticLibs, coverage.OutputFile().Path())
				flags.Local.LdFlags = append(flags.Local.LdFlags, "-Wl,--wrap,getenv")
			}
		} else if clangCoverage {
			flags.Local.LdFlags = append(flags.Local.LdFlags, profileInstrFlag)
			if EnableContinuousCoverage(ctx) {
				flags.Local.LdFlags = append(flags.Local.LdFlags, "-Wl,-mllvm=-runtime-counter-relocation")
			}

			if ctx.Device() {
				coverage := ctx.GetDirectDepWithTag(getClangProfileLibraryName(ctx), CoverageDepTag).(*Module)
				deps.WholeStaticLibs = append(deps.WholeStaticLibs, coverage.OutputFile().Path())
				flags.Local.LdFlags = append(flags.Local.LdFlags, "-Wl,--wrap,open")
			}
		}
	}

	return flags, deps
}

func (cov *coverage) begin(ctx BaseModuleContext) {
	if ctx.Host() && !ctx.Os().Linux() {
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
	IsNativeCoverageNeeded(ctx android.IncomingTransitionContext) bool
}

// Coverage is an interface for non-CC modules to implement to be mutated for coverage
type Coverage interface {
	UseCoverage
	SetPreventInstall()
	HideFromMake()
	MarkAsCoverageVariant(bool)
	EnableCoverageIfNeeded()
}

type coverageTransitionMutator struct{}

var _ android.TransitionMutator = (*coverageTransitionMutator)(nil)

func (c coverageTransitionMutator) Split(ctx android.BaseModuleContext) []string {
	if c, ok := ctx.Module().(*Module); ok && c.coverage != nil {
		if c.coverage.Properties.NeedCoverageVariant {
			return []string{"", "cov"}
		}
	} else if cov, ok := ctx.Module().(Coverage); ok && cov.IsNativeCoverageNeeded(ctx) {
		// APEX and Rust modules fall here

		// Note: variant "" is also created because an APEX can be depended on by another
		// module which are split into "" and "cov" variants. e.g. when cc_test refers
		// to an APEX via 'data' property.
		return []string{"", "cov"}
	} else if cov, ok := ctx.Module().(UseCoverage); ok && cov.IsNativeCoverageNeeded(ctx) {
		// Module itself doesn't have to have "cov" variant, but it should use "cov" variants of
		// deps.
		return []string{"cov"}
	}

	return []string{""}
}

func (c coverageTransitionMutator) OutgoingTransition(ctx android.OutgoingTransitionContext, sourceVariation string) string {
	return sourceVariation
}

func (c coverageTransitionMutator) IncomingTransition(ctx android.IncomingTransitionContext, incomingVariation string) string {
	if c, ok := ctx.Module().(*Module); ok && c.coverage != nil {
		if !c.coverage.Properties.NeedCoverageVariant {
			return ""
		}
	} else if cov, ok := ctx.Module().(Coverage); ok {
		if !cov.IsNativeCoverageNeeded(ctx) {
			return ""
		}
	} else if cov, ok := ctx.Module().(UseCoverage); ok && cov.IsNativeCoverageNeeded(ctx) {
		// Module only has a "cov" variation, so all incoming variations should use "cov".
		return "cov"
	} else {
		return ""
	}

	return incomingVariation
}

func (c coverageTransitionMutator) Mutate(ctx android.BottomUpMutatorContext, variation string) {
	if c, ok := ctx.Module().(*Module); ok && c.coverage != nil {
		if variation == "" && c.coverage.Properties.NeedCoverageVariant {
			// Setup the non-coverage version and set HideFromMake and
			// PreventInstall to true.
			c.coverage.Properties.CoverageEnabled = false
			c.coverage.Properties.IsCoverageVariant = false
			c.Properties.HideFromMake = true
			c.Properties.PreventInstall = true
		} else if variation == "cov" {
			// The coverage-enabled version inherits HideFromMake,
			// PreventInstall from the original module.
			c.coverage.Properties.CoverageEnabled = c.coverage.Properties.NeedCoverageBuild
			c.coverage.Properties.IsCoverageVariant = true
		}
	} else if cov, ok := ctx.Module().(Coverage); ok && cov.IsNativeCoverageNeeded(ctx) {
		// APEX and Rust modules fall here

		// Note: variant "" is also created because an APEX can be depended on by another
		// module which are split into "" and "cov" variants. e.g. when cc_test refers
		// to an APEX via 'data' property.
		if variation == "" {
			cov.MarkAsCoverageVariant(false)
			cov.SetPreventInstall()
			cov.HideFromMake()
		} else if variation == "cov" {
			cov.MarkAsCoverageVariant(true)
			cov.EnableCoverageIfNeeded()
		}
	} else if cov, ok := ctx.Module().(UseCoverage); ok && cov.IsNativeCoverageNeeded(ctx) {
		// Module itself doesn't have to have "cov" variant, but it should use "cov" variants of
		// deps.
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
