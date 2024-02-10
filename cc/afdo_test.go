// Copyright 2022 Google Inc. All rights reserved.
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
	"runtime"
	"strings"
	"testing"

	"android/soong/android"

	"github.com/google/blueprint"
)

type visitDirectDepsInterface interface {
	VisitDirectDeps(blueprint.Module, func(dep blueprint.Module))
}

func hasDirectDep(ctx visitDirectDepsInterface, m android.Module, wantDep android.Module) bool {
	var found bool
	ctx.VisitDirectDeps(m, func(dep blueprint.Module) {
		if dep == wantDep {
			found = true
		}
	})
	return found
}

func TestAfdoDeps(t *testing.T) {
	t.Parallel()
	bp := `
	cc_library_shared {
		name: "libTest",
		host_supported: true,
		srcs: ["test.c"],
		static_libs: ["libFoo"],
		afdo: true,
		lto: {
			thin: true,
		},
	}

	cc_library_static {
		name: "libFoo",
		host_supported: true,
		srcs: ["foo.c"],
		static_libs: ["libBar"],
	}

	cc_library_static {
		name: "libBar",
		host_supported: true,
		srcs: ["bar.c"],
	}
	`

	result := android.GroupFixturePreparers(
		PrepareForTestWithFdoProfile,
		prepareForCcTest,
		android.FixtureAddTextFile("afdo_profiles_package/libTest.afdo", ""),
		android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
			variables.AfdoProfiles = []string{
				"libTest://afdo_profiles_package:libTest_afdo",
			}
		}),
		android.MockFS{
			"afdo_profiles_package/Android.bp": []byte(`
				fdo_profile {
					name: "libTest_afdo",
					arch: {
						arm64: {
							profile: "libTest.afdo",
						},
					},
				}
			`),
		}.AddToFixture(),
	).RunTestWithBp(t, bp)

	profileSampleCFlag := "-fprofile-sample-use=afdo_profiles_package/libTest.afdo"
	uniqueInternalLinkageNamesCFlag := "-funique-internal-linkage-names"
	afdoLtoLdFlag := "-Wl,-plugin-opt,-import-instr-limit=40"
	noAfdoLtoLdFlag := "-Wl,-plugin-opt,-import-instr-limit=5"

	libTest := result.ModuleForTests("libTest", "android_arm64_armv8-a_shared")
	libFooAfdoVariant := result.ModuleForTests("libFoo", "android_arm64_armv8-a_static_afdo-libTest")
	libBarAfdoVariant := result.ModuleForTests("libBar", "android_arm64_armv8-a_static_afdo-libTest")

	// Check cFlags of afdo-enabled module and the afdo-variant of its static deps
	cFlags := libTest.Rule("cc").Args["cFlags"]
	if !strings.Contains(cFlags, profileSampleCFlag) {
		t.Errorf("Expected 'libTest' to enable afdo profile, but did not find %q in cflags %q", profileSampleCFlag, cFlags)
	}
	if !strings.Contains(cFlags, uniqueInternalLinkageNamesCFlag) {
		t.Errorf("Expected 'libTest' to enable afdo, but did not find %q in cflags %q", profileSampleCFlag, cFlags)
	}

	ldFlags := libTest.Rule("ld").Args["ldFlags"]
	if !strings.Contains(ldFlags, afdoLtoLdFlag) {
		t.Errorf("Expected 'libTest' to enable afdo, but did not find %q in ldflags %q", afdoLtoLdFlag, ldFlags)
	}

	cFlags = libFooAfdoVariant.Rule("cc").Args["cFlags"]
	if !strings.Contains(cFlags, profileSampleCFlag) {
		t.Errorf("Expected 'libFooAfdoVariant' to enable afdo profile, but did not find %q in cflags %q", profileSampleCFlag, cFlags)
	}
	if !strings.Contains(cFlags, uniqueInternalLinkageNamesCFlag) {
		t.Errorf("Expected 'libFooAfdoVariant' to enable afdo, but did not find %q in cflags %q", profileSampleCFlag, cFlags)
	}

	cFlags = libBarAfdoVariant.Rule("cc").Args["cFlags"]
	if !strings.Contains(cFlags, profileSampleCFlag) {
		t.Errorf("Expected 'libBarAfdoVariant' to enable afdo profile, but did not find %q in cflags %q", profileSampleCFlag, cFlags)
	}
	if !strings.Contains(cFlags, uniqueInternalLinkageNamesCFlag) {
		t.Errorf("Expected 'libBarAfdoVariant' to enable afdo, but did not find %q in cflags %q", profileSampleCFlag, cFlags)
	}

	// Check dependency edge from afdo-enabled module to static deps
	if !hasDirectDep(result, libTest.Module(), libFooAfdoVariant.Module()) {
		t.Errorf("libTest missing dependency on afdo variant of libFoo")
	}

	if !hasDirectDep(result, libFooAfdoVariant.Module(), libBarAfdoVariant.Module()) {
		t.Errorf("libTest missing dependency on afdo variant of libBar")
	}

	// Verify non-afdo variant exists and doesn't contain afdo
	libFoo := result.ModuleForTests("libFoo", "android_arm64_armv8-a_static")
	libBar := result.ModuleForTests("libBar", "android_arm64_armv8-a_static")

	cFlags = libFoo.Rule("cc").Args["cFlags"]
	if strings.Contains(cFlags, profileSampleCFlag) {
		t.Errorf("Expected 'libFoo' to not enable afdo profile, but found %q in cflags %q", profileSampleCFlag, cFlags)
	}
	if strings.Contains(cFlags, uniqueInternalLinkageNamesCFlag) {
		t.Errorf("Expected 'libFoo' to not enable afdo, but found %q in cflags %q", profileSampleCFlag, cFlags)
	}
	cFlags = libBar.Rule("cc").Args["cFlags"]
	if strings.Contains(cFlags, profileSampleCFlag) {
		t.Errorf("Expected 'libBar' to not enable afdo profile, but found %q in cflags %q", profileSampleCFlag, cFlags)
	}
	if strings.Contains(cFlags, uniqueInternalLinkageNamesCFlag) {
		t.Errorf("Expected 'libBar' to not enable afdo, but found %q in cflags %q", profileSampleCFlag, cFlags)
	}

	// Check dependency edges of static deps
	if hasDirectDep(result, libTest.Module(), libFoo.Module()) {
		t.Errorf("libTest should not depend on non-afdo variant of libFoo")
	}

	if !hasDirectDep(result, libFoo.Module(), libBar.Module()) {
		t.Errorf("libFoo missing dependency on non-afdo variant of libBar")
	}

	// Verify that the arm variant does not have FDO since the fdo_profile module only has a profile for arm64
	libTest32 := result.ModuleForTests("libTest", "android_arm_armv7-a-neon_shared")
	libFooAfdoVariant32 := result.ModuleForTests("libFoo", "android_arm_armv7-a-neon_static_afdo-libTest_lto-thin")
	libBarAfdoVariant32 := result.ModuleForTests("libBar", "android_arm_armv7-a-neon_static_afdo-libTest_lto-thin")

	cFlags = libTest32.Rule("cc").Args["cFlags"]
	if strings.Contains(cFlags, profileSampleCFlag) {
		t.Errorf("Expected arm32 'libTest' not to enable afdo, but found %q in cflags %q", profileSampleCFlag, cFlags)
	}

	// TODO(b/324141705): when the fdo_profile module doesn't provide a source file the dependencies don't get
	//  -funique-internal-linkage-names but the module does.
	if !strings.Contains(cFlags, uniqueInternalLinkageNamesCFlag) {
		t.Errorf("Expected arm32 'libTest' to enable -funique-internal-linkage-names but did not find %q in cflags %q",
			uniqueInternalLinkageNamesCFlag, cFlags)
	}

	ldFlags = libTest32.Rule("ld").Args["ldFlags"]
	if !strings.Contains(ldFlags, noAfdoLtoLdFlag) {
		t.Errorf("Expected arm32 'libTest' to not enable afdo, but did not find %q in ldflags %q", noAfdoLtoLdFlag, ldFlags)
	}
	if strings.Contains(ldFlags, afdoLtoLdFlag) {
		t.Errorf("Expected arm32 'libTest' to not enable afdo, but found %q in ldflags %q", afdoLtoLdFlag, ldFlags)
	}

	// Check dependency edge from afdo-enabled module to static deps
	if !hasDirectDep(result, libTest32.Module(), libFooAfdoVariant32.Module()) {
		t.Errorf("arm32 libTest missing dependency on afdo variant of libFoo")
	}

	if !hasDirectDep(result, libFooAfdoVariant32.Module(), libBarAfdoVariant32.Module()) {
		t.Errorf("arm32 libTest missing dependency on afdo variant of libBar")
	}

	cFlags = libFooAfdoVariant32.Rule("cc").Args["cFlags"]
	if strings.Contains(cFlags, profileSampleCFlag) {
		t.Errorf("Expected arm32 'libFoo' to not enable afdo profile, but found %q in cflags %q", uniqueInternalLinkageNamesCFlag, cFlags)
	}
	if !strings.Contains(cFlags, uniqueInternalLinkageNamesCFlag) {
		t.Errorf("Expected arm32 'libFoo' to enable afdo, but did not find %q in cflags %q", uniqueInternalLinkageNamesCFlag, cFlags)
	}
	cFlags = libBarAfdoVariant32.Rule("cc").Args["cFlags"]
	if strings.Contains(cFlags, profileSampleCFlag) {
		t.Errorf("Expected arm32 'libBar' to not enable afdo profile, but found %q in cflags %q", uniqueInternalLinkageNamesCFlag, cFlags)
	}
	if !strings.Contains(cFlags, uniqueInternalLinkageNamesCFlag) {
		t.Errorf("Expected arm32 'libBar' to enable afdo, but did not find %q in cflags %q", uniqueInternalLinkageNamesCFlag, cFlags)
	}

	// Verify that the host variants don't enable afdo
	libTestHost := result.ModuleForTests("libTest", result.Config.BuildOSTarget.String()+"_shared")
	libFooHost := result.ModuleForTests("libFoo", result.Config.BuildOSTarget.String()+"_static_lto-thin")
	libBarHost := result.ModuleForTests("libBar", result.Config.BuildOSTarget.String()+"_static_lto-thin")

	cFlags = libTestHost.Rule("cc").Args["cFlags"]
	if strings.Contains(cFlags, profileSampleCFlag) {
		t.Errorf("Expected host 'libTest' to not enable afdo profile, but found %q in cflags %q", profileSampleCFlag, cFlags)
	}

	if strings.Contains(cFlags, uniqueInternalLinkageNamesCFlag) {
		t.Errorf("Expected host 'libTest' to not enable afdo but found %q in cflags %q",
			uniqueInternalLinkageNamesCFlag, cFlags)
	}

	if runtime.GOOS != "darwin" {
		ldFlags := libTestHost.Rule("ld").Args["ldFlags"]
		if !strings.Contains(ldFlags, noAfdoLtoLdFlag) {
			t.Errorf("Expected host 'libTest' to not enable afdo, but did not find %q in ldflags %q", noAfdoLtoLdFlag, ldFlags)
		}
		if strings.Contains(ldFlags, afdoLtoLdFlag) {
			t.Errorf("Expected host 'libTest' to not enable afdo, but found %q in ldflags %q", afdoLtoLdFlag, ldFlags)
		}
	}

	// Check dependency edge from afdo-enabled module to static deps
	if !hasDirectDep(result, libTestHost.Module(), libFooHost.Module()) {
		t.Errorf("host libTest missing dependency on non-afdo variant of libFoo")
	}

	if !hasDirectDep(result, libFooHost.Module(), libBarHost.Module()) {
		t.Errorf("host libTest missing dependency on non-afdo variant of libBar")
	}

	cFlags = libFooHost.Rule("cc").Args["cFlags"]
	if strings.Contains(cFlags, profileSampleCFlag) {
		t.Errorf("Expected host 'libFoo' to not enable afdo profile, but found %q in cflags %q", uniqueInternalLinkageNamesCFlag, cFlags)
	}
	if strings.Contains(cFlags, uniqueInternalLinkageNamesCFlag) {
		t.Errorf("Expected host 'libFoo' to not enable afdo, but found %q in cflags %q", uniqueInternalLinkageNamesCFlag, cFlags)
	}
	cFlags = libBarHost.Rule("cc").Args["cFlags"]
	if strings.Contains(cFlags, profileSampleCFlag) {
		t.Errorf("Expected host 'libBar' to not enable afdo profile, but found %q in cflags %q", uniqueInternalLinkageNamesCFlag, cFlags)
	}
	if strings.Contains(cFlags, uniqueInternalLinkageNamesCFlag) {
		t.Errorf("Expected host 'libBar' to not enable afdo, but found %q in cflags %q", uniqueInternalLinkageNamesCFlag, cFlags)
	}
}

func TestAfdoEnabledOnStaticDepNoAfdo(t *testing.T) {
	t.Parallel()
	bp := `
	cc_library_shared {
		name: "libTest",
		srcs: ["foo.c"],
		static_libs: ["libFoo"],
	}

	cc_library_static {
		name: "libFoo",
		srcs: ["foo.c"],
		static_libs: ["libBar"],
		afdo: true, // TODO(b/256670524): remove support for enabling afdo from static only libraries, this can only propagate from shared libraries/binaries
	}

	cc_library_static {
		name: "libBar",
	}
	`

	result := android.GroupFixturePreparers(
		prepareForCcTest,
		PrepareForTestWithFdoProfile,
		android.FixtureAddTextFile("toolchain/pgo-profiles/sampling/libFoo.afdo", ""),
		android.MockFS{
			"afdo_profiles_package/Android.bp": []byte(`
				soong_namespace {
				}
				fdo_profile {
					name: "libFoo_afdo",
					profile: "libFoo.afdo",
				}
			`),
		}.AddToFixture(),
	).RunTestWithBp(t, bp)

	libTest := result.ModuleForTests("libTest", "android_arm64_armv8-a_shared").Module()
	libFoo := result.ModuleForTests("libFoo", "android_arm64_armv8-a_static")
	libBar := result.ModuleForTests("libBar", "android_arm64_armv8-a_static").Module()

	if !hasDirectDep(result, libTest, libFoo.Module()) {
		t.Errorf("libTest missing dependency on non-afdo variant of libFoo")
	}

	if !hasDirectDep(result, libFoo.Module(), libBar) {
		t.Errorf("libFoo missing dependency on non-afdo variant of libBar")
	}

	fooVariants := result.ModuleVariantsForTests("foo")
	for _, v := range fooVariants {
		if strings.Contains(v, "afdo-") {
			t.Errorf("Expected no afdo variant of 'foo', got %q", v)
		}
	}

	cFlags := libFoo.Rule("cc").Args["cFlags"]
	if w := "-fprofile-sample-accurate"; strings.Contains(cFlags, w) {
		t.Errorf("Expected 'foo' to not enable afdo, but found %q in cflags %q", w, cFlags)
	}

	barVariants := result.ModuleVariantsForTests("bar")
	for _, v := range barVariants {
		if strings.Contains(v, "afdo-") {
			t.Errorf("Expected no afdo variant of 'bar', got %q", v)
		}
	}
}

func TestAfdoEnabledWithRuntimeDepNoAfdo(t *testing.T) {
	bp := `
	cc_library {
		name: "libTest",
		srcs: ["foo.c"],
		runtime_libs: ["libFoo"],
		afdo: true,
	}

	cc_library {
		name: "libFoo",
	}
	`

	result := android.GroupFixturePreparers(
		prepareForCcTest,
		PrepareForTestWithFdoProfile,
		android.FixtureAddTextFile("afdo_profiles_package/libTest.afdo", ""),
		android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
			variables.AfdoProfiles = []string{
				"libTest://afdo_profiles_package:libTest_afdo",
			}
		}),
		android.MockFS{
			"afdo_profiles_package/Android.bp": []byte(`
				fdo_profile {
					name: "libTest_afdo",
					profile: "libTest.afdo",
				}
			`),
		}.AddToFixture(),
	).RunTestWithBp(t, bp)

	libFooVariants := result.ModuleVariantsForTests("libFoo")
	for _, v := range libFooVariants {
		if strings.Contains(v, "afdo-") {
			t.Errorf("Expected no afdo variant of 'foo', got %q", v)
		}
	}
}

func TestAfdoEnabledWithMultiArchs(t *testing.T) {
	bp := `
	cc_library_shared {
		name: "foo",
		srcs: ["test.c"],
		afdo: true,
		compile_multilib: "both",
	}
`
	result := android.GroupFixturePreparers(
		PrepareForTestWithFdoProfile,
		prepareForCcTest,
		android.FixtureAddTextFile("afdo_profiles_package/foo_arm.afdo", ""),
		android.FixtureAddTextFile("afdo_profiles_package/foo_arm64.afdo", ""),
		android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
			variables.AfdoProfiles = []string{
				"foo://afdo_profiles_package:foo_afdo",
			}
		}),
		android.MockFS{
			"afdo_profiles_package/Android.bp": []byte(`
				soong_namespace {
				}
				fdo_profile {
					name: "foo_afdo",
					arch: {
						arm: {
							profile: "foo_arm.afdo",
						},
						arm64: {
							profile: "foo_arm64.afdo",
						}
					}
				}
			`),
		}.AddToFixture(),
	).RunTestWithBp(t, bp)

	fooArm := result.ModuleForTests("foo", "android_arm_armv7-a-neon_shared")
	fooArmCFlags := fooArm.Rule("cc").Args["cFlags"]
	if w := "-fprofile-sample-use=afdo_profiles_package/foo_arm.afdo"; !strings.Contains(fooArmCFlags, w) {
		t.Errorf("Expected 'foo' to enable afdo, but did not find %q in cflags %q", w, fooArmCFlags)
	}

	fooArm64 := result.ModuleForTests("foo", "android_arm64_armv8-a_shared")
	fooArm64CFlags := fooArm64.Rule("cc").Args["cFlags"]
	if w := "-fprofile-sample-use=afdo_profiles_package/foo_arm64.afdo"; !strings.Contains(fooArm64CFlags, w) {
		t.Errorf("Expected 'foo' to enable afdo, but did not find %q in cflags %q", w, fooArm64CFlags)
	}
}

func TestMultipleAfdoRDeps(t *testing.T) {
	t.Parallel()
	bp := `
	cc_library_shared {
		name: "libTest",
		srcs: ["test.c"],
		static_libs: ["libFoo"],
		afdo: true,
	}

	cc_library_shared {
		name: "libBar",
		srcs: ["bar.c"],
		static_libs: ["libFoo"],
		afdo: true,
	}

	cc_library_static {
		name: "libFoo",
		srcs: ["foo.c"],
	}
	`

	result := android.GroupFixturePreparers(
		PrepareForTestWithFdoProfile,
		prepareForCcTest,
		android.FixtureAddTextFile("afdo_profiles_package/libTest.afdo", ""),
		android.FixtureAddTextFile("afdo_profiles_package/libBar.afdo", ""),
		android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
			variables.AfdoProfiles = []string{
				"libTest://afdo_profiles_package:libTest_afdo",
				"libBar://afdo_profiles_package:libBar_afdo",
			}
		}),
		android.MockFS{
			"afdo_profiles_package/Android.bp": []byte(`
				fdo_profile {
					name: "libTest_afdo",
					profile: "libTest.afdo",
				}
				fdo_profile {
					name: "libBar_afdo",
					profile: "libBar.afdo",
				}
			`),
		}.AddToFixture(),
	).RunTestWithBp(t, bp)

	expectedCFlagLibTest := "-fprofile-sample-use=afdo_profiles_package/libTest.afdo"
	expectedCFlagLibBar := "-fprofile-sample-use=afdo_profiles_package/libBar.afdo"

	libTest := result.ModuleForTests("libTest", "android_arm64_armv8-a_shared")
	libFooAfdoVariantWithLibTest := result.ModuleForTests("libFoo", "android_arm64_armv8-a_static_afdo-libTest")

	libBar := result.ModuleForTests("libBar", "android_arm64_armv8-a_shared")
	libFooAfdoVariantWithLibBar := result.ModuleForTests("libFoo", "android_arm64_armv8-a_static_afdo-libBar")

	// Check cFlags of afdo-enabled module and the afdo-variant of its static deps
	cFlags := libTest.Rule("cc").Args["cFlags"]
	if !strings.Contains(cFlags, expectedCFlagLibTest) {
		t.Errorf("Expected 'libTest' to enable afdo, but did not find %q in cflags %q", expectedCFlagLibTest, cFlags)
	}
	cFlags = libBar.Rule("cc").Args["cFlags"]
	if !strings.Contains(cFlags, expectedCFlagLibBar) {
		t.Errorf("Expected 'libTest' to enable afdo, but did not find %q in cflags %q", expectedCFlagLibBar, cFlags)
	}

	cFlags = libFooAfdoVariantWithLibTest.Rule("cc").Args["cFlags"]
	if !strings.Contains(cFlags, expectedCFlagLibTest) {
		t.Errorf("Expected 'libFooAfdoVariantWithLibTest' to enable afdo, but did not find %q in cflags %q", expectedCFlagLibTest, cFlags)
	}

	cFlags = libFooAfdoVariantWithLibBar.Rule("cc").Args["cFlags"]
	if !strings.Contains(cFlags, expectedCFlagLibBar) {
		t.Errorf("Expected 'libBarAfdoVariant' to enable afdo, but did not find %q in cflags %q", expectedCFlagLibBar, cFlags)
	}

	// Check dependency edges of static deps
	if !hasDirectDep(result, libTest.Module(), libFooAfdoVariantWithLibTest.Module()) {
		t.Errorf("libTest missing dependency on afdo variant of libFoo")
	}

	if !hasDirectDep(result, libBar.Module(), libFooAfdoVariantWithLibBar.Module()) {
		t.Errorf("libFoo missing dependency on non-afdo variant of libBar")
	}
}

func TestAfdoDepsWithoutProfile(t *testing.T) {
	t.Parallel()
	bp := `
	cc_library_shared {
		name: "libTest",
		srcs: ["test.c"],
		static_libs: ["libFoo"],
		afdo: true,
	}

	cc_library_static {
		name: "libFoo",
		srcs: ["foo.c"],
		static_libs: ["libBar"],
	}

	cc_library_static {
		name: "libBar",
		srcs: ["bar.c"],
	}
	`

	result := android.GroupFixturePreparers(
		PrepareForTestWithFdoProfile,
		prepareForCcTest,
	).RunTestWithBp(t, bp)

	// Even without a profile path, the afdo enabled libraries should be built with
	// -funique-internal-linkage-names.
	expectedCFlag := "-funique-internal-linkage-names"

	libTest := result.ModuleForTests("libTest", "android_arm64_armv8-a_shared")
	libFooAfdoVariant := result.ModuleForTests("libFoo", "android_arm64_armv8-a_static_afdo-libTest")
	libBarAfdoVariant := result.ModuleForTests("libBar", "android_arm64_armv8-a_static_afdo-libTest")

	// Check cFlags of afdo-enabled module and the afdo-variant of its static deps
	cFlags := libTest.Rule("cc").Args["cFlags"]
	if !strings.Contains(cFlags, expectedCFlag) {
		t.Errorf("Expected 'libTest' to enable afdo, but did not find %q in cflags %q", expectedCFlag, cFlags)
	}

	cFlags = libFooAfdoVariant.Rule("cc").Args["cFlags"]
	if !strings.Contains(cFlags, expectedCFlag) {
		t.Errorf("Expected 'libFooAfdoVariant' to enable afdo, but did not find %q in cflags %q", expectedCFlag, cFlags)
	}

	cFlags = libBarAfdoVariant.Rule("cc").Args["cFlags"]
	if !strings.Contains(cFlags, expectedCFlag) {
		t.Errorf("Expected 'libBarAfdoVariant' to enable afdo, but did not find %q in cflags %q", expectedCFlag, cFlags)
	}
	// Check dependency edge from afdo-enabled module to static deps
	if !hasDirectDep(result, libTest.Module(), libFooAfdoVariant.Module()) {
		t.Errorf("libTest missing dependency on afdo variant of libFoo")
	}

	if !hasDirectDep(result, libFooAfdoVariant.Module(), libBarAfdoVariant.Module()) {
		t.Errorf("libTest missing dependency on afdo variant of libBar")
	}

	// Verify non-afdo variant exists and doesn't contain afdo
	libFoo := result.ModuleForTests("libFoo", "android_arm64_armv8-a_static")
	libBar := result.ModuleForTests("libBar", "android_arm64_armv8-a_static")

	cFlags = libFoo.Rule("cc").Args["cFlags"]
	if strings.Contains(cFlags, expectedCFlag) {
		t.Errorf("Expected 'libFoo' to not enable afdo, but found %q in cflags %q", expectedCFlag, cFlags)
	}
	cFlags = libBar.Rule("cc").Args["cFlags"]
	if strings.Contains(cFlags, expectedCFlag) {
		t.Errorf("Expected 'libBar' to not enable afdo, but found %q in cflags %q", expectedCFlag, cFlags)
	}

	// Check dependency edges of static deps
	if hasDirectDep(result, libTest.Module(), libFoo.Module()) {
		t.Errorf("libTest should not depend on non-afdo variant of libFoo")
	}

	if !hasDirectDep(result, libFoo.Module(), libBar.Module()) {
		t.Errorf("libFoo missing dependency on non-afdo variant of libBar")
	}
}
