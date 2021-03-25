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
	PgoInstrLink        bool `blueprint:"mutated"`
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
	// Add to C flags iff PGO is explicitly enabled for this module.
	if props.ShouldProfileModule {
		flags.Local.CFlags = append(flags.Local.CFlags, props.Pgo.Cflags...)
		flags.Local.CFlags = append(flags.Local.CFlags, profileInstrumentFlag)
	}
	flags.Local.LdFlags = append(flags.Local.LdFlags, profileInstrumentFlag)
	return flags
}
func (props *PgoProperties) addSamplingProfileGatherFlags(ctx ModuleContext, flags Flags) Flags {
	flags.Local.CFlags = append(flags.Local.CFlags, props.Pgo.Cflags...)
	return flags
}

func (props *PgoProperties) getPgoProfileFile(ctx BaseModuleContext) android.OptionalPath {
	profileFile := *props.Pgo.Profile_file

	// Test if the profile_file is present in any of the PGO profile projects
	for _, profileProject := range getPgoProfileProjects(ctx.DeviceConfig()) {
		// Bug: http://b/74395273 If the profile_file is unavailable,
		// use a versioned file named
		// <profile_file>.<arbitrary-version> when available.  This
		// works around an issue where ccache serves stale cache
		// entries when the profile file has changed.
		globPattern := filepath.Join(profileProject, profileFile+".*")
		versionedProfiles, err := ctx.GlobWithDeps(globPattern, nil)
		if err != nil {
			ctx.ModuleErrorf("glob: %s", err.Error())
		}

		path := android.ExistentPathForSource(ctx, profileProject, profileFile)
		if path.Valid() {
			if len(versionedProfiles) != 0 {
				ctx.PropertyErrorf("pgo.profile_file", "Profile_file has multiple versions: "+filepath.Join(profileProject, profileFile)+", "+strings.Join(versionedProfiles, ", "))
			}
			return path
		}

		if len(versionedProfiles) > 1 {
			ctx.PropertyErrorf("pgo.profile_file", "Profile_file has multiple versions: "+strings.Join(versionedProfiles, ", "))
		} else if len(versionedProfiles) == 1 {
			return android.OptionalPathForPath(android.PathForSource(ctx, versionedProfiles[0]))
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

	// profileKindPresent and filePresent are mandatory properties.
	if !profileKindPresent || !filePresent {
		var missing []string
		if !profileKindPresent {
			missing = append(missing, "profile kind (either \"instrumentation\" or \"sampling\" property)")
		}
		if !filePresent {
			missing = append(missing, "profile_file property")
		}
		missingProps := strings.Join(missing, ", ")
		ctx.ModuleErrorf("PGO specification is missing properties: " + missingProps)
	}

	// Benchmark property is mandatory for instrumentation PGO.
	if isInstrumentation && !benchmarksPresent {
		ctx.ModuleErrorf("Instrumentation PGO specification is missing benchmark property")
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
		pgo.Properties.PgoInstrLink = pgo.Properties.isInstrumentation()
	} else {
		for _, b := range pgo.Properties.Pgo.Benchmarks {
			if pgoBenchmarksMap[b] == true {
				pgo.Properties.ShouldProfileModule = true
				pgo.Properties.PgoInstrLink = pgo.Properties.isInstrumentation()
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

func (pgo *pgo) flags(ctx ModuleContext, flags Flags) Flags {
	if ctx.Host() {
		return flags
	}

	// Deduce PgoInstrLink property i.e. whether this module needs to be
	// linked with profile-generation flags.  Here, we're setting it if any
	// dependency needs PGO instrumentation.  It is initially set in
	// begin() if PGO is directly enabled for this module.
	if ctx.static() && !ctx.staticBinary() {
		// For static libraries, check if any whole_static_libs are
		// linked with profile generation
		ctx.VisitDirectDeps(func(m android.Module) {
			if depTag, ok := ctx.OtherModuleDependencyTag(m).(libraryDependencyTag); ok {
				if depTag.static() && depTag.wholeStatic {
					if cc, ok := m.(*Module); ok {
						if cc.pgo.Properties.PgoInstrLink {
							pgo.Properties.PgoInstrLink = true
						}
					}
				}
			}
		})
	} else {
		// For executables and shared libraries, check all static dependencies.
		ctx.VisitDirectDeps(func(m android.Module) {
			if depTag, ok := ctx.OtherModuleDependencyTag(m).(libraryDependencyTag); ok {
				if depTag.static() {
					if cc, ok := m.(*Module); ok {
						if cc.pgo.Properties.PgoInstrLink {
							pgo.Properties.PgoInstrLink = true
						}
					}
				}
			}
		})
	}

	props := pgo.Properties
	// Add flags to profile this module based on its profile_kind
	if (props.ShouldProfileModule && props.isInstrumentation()) || props.PgoInstrLink {
		// Instrumentation PGO use and gather flags cannot coexist.
		return props.addInstrumentationProfileGatherFlags(ctx, flags)
	} else if props.ShouldProfileModule && props.isSampling() {
		flags = props.addSamplingProfileGatherFlags(ctx, flags)
	} else if ctx.DeviceConfig().SamplingPGO() {
		flags = props.addSamplingProfileGatherFlags(ctx, flags)
	}

	if !ctx.Config().IsEnvTrue("ANDROID_PGO_NO_PROFILE_USE") {
		flags = props.addProfileUseFlags(ctx, flags)
	}

	return flags
}
