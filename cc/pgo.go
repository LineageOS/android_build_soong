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
	"fmt"
	"path/filepath"
	"strings"

	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/cc/config"
)

var (
	// Add flags to ignore warnings that profiles are old or missing for
	// some functions.
	profileUseOtherFlags = []string{
		"-Wno-backend-plugin",
	}

	globalPgoProfileProjects = []string{
		"toolchain/pgo-profiles",
		"vendor/google_data/pgo_profile",
	}
)

var pgoProfileProjectsConfigKey = android.NewOnceKey("PgoProfileProjects")

const profileInstrumentFlag = "-fprofile-generate=/data/local/tmp"
const profileSamplingFlag = "-gmlt -fdebug-info-for-profiling"
const profileUseInstrumentFormat = "-fprofile-use=%s"
const profileUseSamplingFormat = "-fprofile-sample-accurate -fprofile-sample-use=%s"

func getPgoProfileProjects(config android.DeviceConfig) []string {
	return config.OnceStringSlice(pgoProfileProjectsConfigKey, func() []string {
		return append(globalPgoProfileProjects, config.PgoAdditionalProfileDirs()...)
	})
}

func recordMissingProfileFile(ctx BaseModuleContext, missing string) {
	getNamedMapForConfig(ctx.Config(), modulesMissingProfileFileKey).Store(missing, true)
}

type PgoProperties struct {
	Pgo struct {
		Instrumentation    *bool
		Sampling           *bool
		Profile_file       *string `android:"arch_variant"`
		Benchmarks         []string
		Enable_profile_use *bool `android:"arch_variant"`
		// Additional compiler flags to use when building this module
		// for profiling (either instrumentation or sampling).
		Cflags []string `android:"arch_variant"`
	} `android:"arch_variant"`

	PgoPresent          bool `blueprint:"mutated"`
	ShouldProfileModule bool `blueprint:"mutated"`
	PgoCompile          bool `blueprint:"mutated"`
}

type pgo struct {
	Properties PgoProperties
}

func (props *PgoProperties) isInstrumentation() bool {
	return props.Pgo.Instrumentation != nil && *props.Pgo.Instrumentation == true
}

func (props *PgoProperties) isSampling() bool {
	return props.Pgo.Sampling != nil && *props.Pgo.Sampling == true
}

func (pgo *pgo) props() []interface{} {
	return []interface{}{&pgo.Properties}
}

func (props *PgoProperties) addInstrumentationProfileGatherFlags(ctx ModuleContext, flags Flags) Flags {
	flags.Local.CFlags = append(flags.Local.CFlags, props.Pgo.Cflags...)

	flags.Local.CFlags = append(flags.Local.CFlags, profileInstrumentFlag)
	// The profile runtime is added below in deps().  Add the below
	// flag, which is the only other link-time action performed by
	// the Clang driver during link.
	flags.Local.LdFlags = append(flags.Local.LdFlags, "-u__llvm_profile_runtime")
	return flags
}
func (props *PgoProperties) addSamplingProfileGatherFlags(ctx ModuleContext, flags Flags) Flags {
	flags.Local.CFlags = append(flags.Local.CFlags, props.Pgo.Cflags...)

	flags.Local.CFlags = append(flags.Local.CFlags, profileSamplingFlag)
	flags.Local.LdFlags = append(flags.Local.LdFlags, profileSamplingFlag)
	return flags
}

func (props *PgoProperties) getPgoProfileFile(ctx BaseModuleContext) android.OptionalPath {
	profile_file := *props.Pgo.Profile_file

	// Test if the profile_file is present in any of the PGO profile projects
	for _, profileProject := range getPgoProfileProjects(ctx.DeviceConfig()) {
		// Bug: http://b/74395273 If the profile_file is unavailable,
		// use a versioned file named
		// <profile_file>.<arbitrary-version> when available.  This
		// works around an issue where ccache serves stale cache
		// entries when the profile file has changed.
		globPattern := filepath.Join(profileProject, profile_file+".*")
		versioned_profiles, err := ctx.GlobWithDeps(globPattern, nil)
		if err != nil {
			ctx.ModuleErrorf("glob: %s", err.Error())
		}

		path := android.ExistentPathForSource(ctx, profileProject, profile_file)
		if path.Valid() {
			if len(versioned_profiles) != 0 {
				ctx.PropertyErrorf("pgo.profile_file", "Profile_file has multiple versions: "+filepath.Join(profileProject, profile_file)+", "+strings.Join(versioned_profiles, ", "))
			}
			return path
		}

		if len(versioned_profiles) > 1 {
			ctx.PropertyErrorf("pgo.profile_file", "Profile_file has multiple versions: "+strings.Join(versioned_profiles, ", "))
		} else if len(versioned_profiles) == 1 {
			return android.OptionalPathForPath(android.PathForSource(ctx, versioned_profiles[0]))
		}
	}

	// Record that this module's profile file is absent
	missing := *props.Pgo.Profile_file + ":" + ctx.ModuleDir() + "/Android.bp:" + ctx.ModuleName()
	recordMissingProfileFile(ctx, missing)

	return android.OptionalPathForPath(nil)
}

func (props *PgoProperties) profileUseFlag(ctx ModuleContext, file string) string {
	if props.isInstrumentation() {
		return fmt.Sprintf(profileUseInstrumentFormat, file)
	}
	if props.isSampling() {
		return fmt.Sprintf(profileUseSamplingFormat, file)
	}
	return ""
}

func (props *PgoProperties) profileUseFlags(ctx ModuleContext, file string) []string {
	flags := []string{props.profileUseFlag(ctx, file)}
	flags = append(flags, profileUseOtherFlags...)
	return flags
}

func (props *PgoProperties) addProfileUseFlags(ctx ModuleContext, flags Flags) Flags {
	// Return if 'pgo' property is not present in this module.
	if !props.PgoPresent {
		return flags
	}

	if props.PgoCompile {
		profileFile := props.getPgoProfileFile(ctx)
		profileFilePath := profileFile.Path()
		profileUseFlags := props.profileUseFlags(ctx, profileFilePath.String())

		flags.Local.CFlags = append(flags.Local.CFlags, profileUseFlags...)
		flags.Local.LdFlags = append(flags.Local.LdFlags, profileUseFlags...)

		// Update CFlagsDeps and LdFlagsDeps so the module is rebuilt
		// if profileFile gets updated
		flags.CFlagsDeps = append(flags.CFlagsDeps, profileFilePath)
		flags.LdFlagsDeps = append(flags.LdFlagsDeps, profileFilePath)

		if props.isSampling() {
			flags.Local.LdFlags = append(flags.Local.LdFlags, "-Wl,-mllvm,-no-warn-sample-unused=true")
		}
	}
	return flags
}

func (props *PgoProperties) isPGO(ctx BaseModuleContext) bool {
	isInstrumentation := props.isInstrumentation()
	isSampling := props.isSampling()

	profileKindPresent := isInstrumentation || isSampling
	filePresent := props.Pgo.Profile_file != nil
	benchmarksPresent := len(props.Pgo.Benchmarks) > 0

	// If all three properties are absent, PGO is OFF for this module
	if !profileKindPresent && !filePresent && !benchmarksPresent {
		return false
	}

	// If at least one property exists, validate that all properties exist
	if !profileKindPresent || !filePresent || !benchmarksPresent {
		var missing []string
		if !profileKindPresent {
			missing = append(missing, "profile kind (either \"instrumentation\" or \"sampling\" property)")
		}
		if !filePresent {
			missing = append(missing, "profile_file property")
		}
		if !benchmarksPresent {
			missing = append(missing, "non-empty benchmarks property")
		}
		missingProps := strings.Join(missing, ", ")
		ctx.ModuleErrorf("PGO specification is missing properties: " + missingProps)
	}

	if isSampling && isInstrumentation {
		ctx.PropertyErrorf("pgo", "Exactly one of \"instrumentation\" and \"sampling\" properties must be set")
	}

	return true
}

func (pgo *pgo) begin(ctx BaseModuleContext) {
	// TODO Evaluate if we need to support PGO for host modules
	if ctx.Host() {
		return
	}

	// Check if PGO is needed for this module
	pgo.Properties.PgoPresent = pgo.Properties.isPGO(ctx)

	if !pgo.Properties.PgoPresent {
		return
	}

	// This module should be instrumented if ANDROID_PGO_INSTRUMENT is set
	// and includes 'all', 'ALL' or a benchmark listed for this module.
	//
	// TODO Validate that each benchmark instruments at least one module
	pgo.Properties.ShouldProfileModule = false
	pgoBenchmarks := ctx.Config().Getenv("ANDROID_PGO_INSTRUMENT")
	pgoBenchmarksMap := make(map[string]bool)
	for _, b := range strings.Split(pgoBenchmarks, ",") {
		pgoBenchmarksMap[b] = true
	}

	if pgoBenchmarksMap["all"] == true || pgoBenchmarksMap["ALL"] == true {
		pgo.Properties.ShouldProfileModule = true
	} else {
		for _, b := range pgo.Properties.Pgo.Benchmarks {
			if pgoBenchmarksMap[b] == true {
				pgo.Properties.ShouldProfileModule = true
				break
			}
		}
	}

	// PGO profile use is not feasible for a Clang coverage build because
	// -fprofile-use and -fprofile-instr-generate are incompatible.
	if ctx.DeviceConfig().ClangCoverageEnabled() {
		return
	}

	if !ctx.Config().IsEnvTrue("ANDROID_PGO_NO_PROFILE_USE") &&
		proptools.BoolDefault(pgo.Properties.Pgo.Enable_profile_use, true) {
		if profileFile := pgo.Properties.getPgoProfileFile(ctx); profileFile.Valid() {
			pgo.Properties.PgoCompile = true
		}
	}
}

func (pgo *pgo) deps(ctx BaseModuleContext, deps Deps) Deps {
	if pgo.Properties.ShouldProfileModule {
		runtimeLibrary := config.ProfileRuntimeLibrary(ctx.toolchain())
		deps.LateStaticLibs = append(deps.LateStaticLibs, runtimeLibrary)
	}
	return deps
}

func (pgo *pgo) flags(ctx ModuleContext, flags Flags) Flags {
	if ctx.Host() {
		return flags
	}

	props := pgo.Properties

	// Add flags to profile this module based on its profile_kind
	if props.ShouldProfileModule && props.isInstrumentation() {
		return props.addInstrumentationProfileGatherFlags(ctx, flags)
	} else if props.ShouldProfileModule && props.isSampling() {
		return props.addSamplingProfileGatherFlags(ctx, flags)
	} else if ctx.DeviceConfig().SamplingPGO() {
		return props.addSamplingProfileGatherFlags(ctx, flags)
	}

	if !ctx.Config().IsEnvTrue("ANDROID_PGO_NO_PROFILE_USE") {
		return props.addProfileUseFlags(ctx, flags)
	}

	return flags
}
