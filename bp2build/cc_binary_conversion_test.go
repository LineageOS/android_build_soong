// Copyright 2021 Google Inc. All rights reserved.
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

package bp2build

import (
	"android/soong/android"
	"android/soong/cc"
	"fmt"
	"strings"
	"testing"
)

const (
	ccBinaryTypePlaceHolder   = "{rule_name}"
	compatibleWithPlaceHolder = "{target_compatible_with}"
)

func registerCcBinaryModuleTypes(ctx android.RegistrationContext) {
	cc.RegisterCCBuildComponents(ctx)
	ctx.RegisterModuleType("filegroup", android.FileGroupFactory)
	ctx.RegisterModuleType("cc_library_static", cc.LibraryStaticFactory)
	ctx.RegisterModuleType("cc_library", cc.LibraryFactory)
}

var binaryReplacer = strings.NewReplacer(ccBinaryTypePlaceHolder, "cc_binary", compatibleWithPlaceHolder, "")
var hostBinaryReplacer = strings.NewReplacer(ccBinaryTypePlaceHolder, "cc_binary_host", compatibleWithPlaceHolder, `
    target_compatible_with = select({
        "//build/bazel/platforms/os:android": ["@platforms//:incompatible"],
        "//conditions:default": [],
    }),`)

func runCcBinaryTests(t *testing.T, tc bp2buildTestCase) {
	t.Helper()
	runCcBinaryTestCase(t, tc)
	runCcHostBinaryTestCase(t, tc)
}

func runCcBinaryTestCase(t *testing.T, tc bp2buildTestCase) {
	t.Helper()
	testCase := tc
	testCase.expectedBazelTargets = append([]string{}, tc.expectedBazelTargets...)
	testCase.moduleTypeUnderTest = "cc_binary"
	testCase.moduleTypeUnderTestFactory = cc.BinaryFactory
	testCase.moduleTypeUnderTestBp2BuildMutator = cc.BinaryBp2build
	testCase.description = fmt.Sprintf("%s %s", testCase.moduleTypeUnderTest, testCase.description)
	testCase.blueprint = binaryReplacer.Replace(testCase.blueprint)
	for i, et := range testCase.expectedBazelTargets {
		testCase.expectedBazelTargets[i] = binaryReplacer.Replace(et)
	}
	t.Run(testCase.description, func(t *testing.T) {
		runBp2BuildTestCase(t, registerCcBinaryModuleTypes, testCase)
	})
}

func runCcHostBinaryTestCase(t *testing.T, tc bp2buildTestCase) {
	t.Helper()
	testCase := tc
	testCase.expectedBazelTargets = append([]string{}, tc.expectedBazelTargets...)
	testCase.moduleTypeUnderTest = "cc_binary_host"
	testCase.moduleTypeUnderTestFactory = cc.BinaryHostFactory
	testCase.moduleTypeUnderTestBp2BuildMutator = cc.BinaryHostBp2build
	testCase.description = fmt.Sprintf("%s %s", testCase.moduleTypeUnderTest, testCase.description)
	testCase.blueprint = hostBinaryReplacer.Replace(testCase.blueprint)
	for i, et := range testCase.expectedBazelTargets {
		testCase.expectedBazelTargets[i] = hostBinaryReplacer.Replace(et)
	}
	t.Run(testCase.description, func(t *testing.T) {
		runBp2BuildTestCase(t, registerCcBinaryModuleTypes, testCase)
	})
}

func TestBasicCcBinary(t *testing.T) {
	runCcBinaryTests(t, bp2buildTestCase{
		description: "basic -- properties -> attrs with little/no transformation",
		blueprint: `
{rule_name} {
    name: "foo",
    srcs: ["a.cc"],
    local_include_dirs: ["dir"],
    include_dirs: ["absolute_dir"],
    cflags: ["-Dcopt"],
    cppflags: ["-Dcppflag"],
    conlyflags: ["-Dconlyflag"],
    asflags: ["-Dasflag"],
    ldflags: ["ld-flag"],
    rtti: true,
    strip: {
        all: true,
        keep_symbols: true,
        keep_symbols_and_debug_frame: true,
        keep_symbols_list: ["symbol"],
        none: true,
    },
}
`,
		expectedBazelTargets: []string{`cc_binary(
    name = "foo",
    absolute_includes = ["absolute_dir"],
    asflags = ["-Dasflag"],
    conlyflags = ["-Dconlyflag"],
    copts = ["-Dcopt"],
    cppflags = ["-Dcppflag"],
    linkopts = ["ld-flag"],
    local_includes = [
        "dir",
        ".",
    ],
    rtti = True,
    srcs = ["a.cc"],
    strip = {
        "all": True,
        "keep_symbols": True,
        "keep_symbols_and_debug_frame": True,
        "keep_symbols_list": ["symbol"],
        "none": True,
    },{target_compatible_with}
)`},
	})
}

func TestCcBinaryWithSharedLdflagDisableFeature(t *testing.T) {
	runCcBinaryTests(t, bp2buildTestCase{
		description: `ldflag "-shared" disables static_flag feature`,
		blueprint: `
{rule_name} {
    name: "foo",
    ldflags: ["-shared"],
    include_build_directory: false,
}
`,
		expectedBazelTargets: []string{`cc_binary(
    name = "foo",
    features = ["-static_flag"],
    linkopts = ["-shared"],{target_compatible_with}
)`},
	})
}

func TestCcBinaryWithLinkStatic(t *testing.T) {
	runCcBinaryTests(t, bp2buildTestCase{
		description: "link static",
		blueprint: `
{rule_name} {
    name: "foo",
    static_executable: true,
    include_build_directory: false,
}
`,
		expectedBazelTargets: []string{`cc_binary(
    name = "foo",
    linkshared = False,{target_compatible_with}
)`},
	})
}

func TestCcBinaryVersionScript(t *testing.T) {
	runCcBinaryTests(t, bp2buildTestCase{
		description: `version script`,
		blueprint: `
{rule_name} {
    name: "foo",
    include_build_directory: false,
    version_script: "vs",
}
`,
		expectedBazelTargets: []string{`cc_binary(
    name = "foo",
    additional_linker_inputs = ["vs"],
    linkopts = ["-Wl,--version-script,$(location vs)"],{target_compatible_with}
)`},
	})
}

func TestCcBinarySplitSrcsByLang(t *testing.T) {
	runCcHostBinaryTestCase(t, bp2buildTestCase{
		description: "split srcs by lang",
		blueprint: `
{rule_name} {
    name: "foo",
    srcs: [
        "asonly.S",
        "conly.c",
        "cpponly.cpp",
        ":fg_foo",
    ],
    include_build_directory: false,
}
` + simpleModuleDoNotConvertBp2build("filegroup", "fg_foo"),
		expectedBazelTargets: []string{`cc_binary(
    name = "foo",
    srcs = [
        "cpponly.cpp",
        ":fg_foo_cpp_srcs",
    ],
    srcs_as = [
        "asonly.S",
        ":fg_foo_as_srcs",
    ],
    srcs_c = [
        "conly.c",
        ":fg_foo_c_srcs",
    ],{target_compatible_with}
)`},
	})
}

func TestCcBinaryDoNotDistinguishBetweenDepsAndImplementationDeps(t *testing.T) {
	runCcBinaryTestCase(t, bp2buildTestCase{
		description: "no implementation deps",
		blueprint: `
{rule_name} {
    name: "foo",
    shared_libs: ["implementation_shared_dep", "shared_dep"],
    export_shared_lib_headers: ["shared_dep"],
    static_libs: ["implementation_static_dep", "static_dep"],
    export_static_lib_headers: ["static_dep", "whole_static_dep"],
    whole_static_libs: ["not_explicitly_exported_whole_static_dep", "whole_static_dep"],
    include_build_directory: false,
}
` +
			simpleModuleDoNotConvertBp2build("cc_library_static", "static_dep") +
			simpleModuleDoNotConvertBp2build("cc_library_static", "implementation_static_dep") +
			simpleModuleDoNotConvertBp2build("cc_library_static", "whole_static_dep") +
			simpleModuleDoNotConvertBp2build("cc_library_static", "not_explicitly_exported_whole_static_dep") +
			simpleModuleDoNotConvertBp2build("cc_library", "shared_dep") +
			simpleModuleDoNotConvertBp2build("cc_library", "implementation_shared_dep"),
		expectedBazelTargets: []string{`cc_binary(
    name = "foo",
    deps = [
        ":implementation_static_dep",
        ":static_dep",
    ],
    dynamic_deps = [
        ":implementation_shared_dep",
        ":shared_dep",
    ],{target_compatible_with}
    whole_archive_deps = [
        ":not_explicitly_exported_whole_static_dep",
        ":whole_static_dep",
    ],
)`},
	})
}

func TestCcBinaryNocrtTests(t *testing.T) {
	baseTestCases := []struct {
		description   string
		soongProperty string
		bazelAttr     string
	}{
		{
			description:   "nocrt: true",
			soongProperty: `nocrt: true,`,
			bazelAttr:     `    link_crt = False,`,
		},
		{
			description:   "nocrt: false",
			soongProperty: `nocrt: false,`,
		},
		{
			description: "nocrt: not set",
		},
	}

	baseBlueprint := `{rule_name} {
    name: "foo",%s
    include_build_directory: false,
}
`

	baseBazelTarget := `cc_binary(
    name = "foo",%s{target_compatible_with}
)`

	for _, btc := range baseTestCases {
		prop := btc.soongProperty
		if len(prop) > 0 {
			prop = "\n" + prop
		}
		attr := btc.bazelAttr
		if len(attr) > 0 {
			attr = "\n" + attr
		}
		runCcBinaryTests(t, bp2buildTestCase{
			description: btc.description,
			blueprint:   fmt.Sprintf(baseBlueprint, prop),
			expectedBazelTargets: []string{
				fmt.Sprintf(baseBazelTarget, attr),
			},
		})
	}
}

func TestCcBinaryNo_libcrtTests(t *testing.T) {
	baseTestCases := []struct {
		description   string
		soongProperty string
		bazelAttr     string
	}{
		{
			description:   "no_libcrt: true",
			soongProperty: `no_libcrt: true,`,
			bazelAttr:     `    use_libcrt = False,`,
		},
		{
			description:   "no_libcrt: false",
			soongProperty: `no_libcrt: false,`,
			bazelAttr:     `    use_libcrt = True,`,
		},
		{
			description: "no_libcrt: not set",
		},
	}

	baseBlueprint := `{rule_name} {
    name: "foo",%s
    include_build_directory: false,
}
`

	baseBazelTarget := `cc_binary(
    name = "foo",{target_compatible_with}%s
)`

	for _, btc := range baseTestCases {
		prop := btc.soongProperty
		if len(prop) > 0 {
			prop = "\n" + prop
		}
		attr := btc.bazelAttr
		if len(attr) > 0 {
			attr = "\n" + attr
		}
		runCcBinaryTests(t, bp2buildTestCase{
			description: btc.description,
			blueprint:   fmt.Sprintf(baseBlueprint, prop),
			expectedBazelTargets: []string{
				fmt.Sprintf(baseBazelTarget, attr),
			},
		})
	}
}

func TestCcBinaryPropertiesToFeatures(t *testing.T) {
	baseTestCases := []struct {
		description   string
		soongProperty string
		bazelAttr     string
	}{
		{
			description:   "pack_relocation: true",
			soongProperty: `pack_relocations: true,`,
		},
		{
			description:   "pack_relocations: false",
			soongProperty: `pack_relocations: false,`,
			bazelAttr:     `    features = ["disable_pack_relocations"],`,
		},
		{
			description: "pack_relocations: not set",
		},
		{
			description:   "pack_relocation: true",
			soongProperty: `allow_undefined_symbols: true,`,
			bazelAttr:     `    features = ["-no_undefined_symbols"],`,
		},
		{
			description:   "allow_undefined_symbols: false",
			soongProperty: `allow_undefined_symbols: false,`,
		},
		{
			description: "allow_undefined_symbols: not set",
		},
	}

	baseBlueprint := `{rule_name} {
    name: "foo",%s
    include_build_directory: false,
}
`

	baseBazelTarget := `cc_binary(
    name = "foo",%s{target_compatible_with}
)`

	for _, btc := range baseTestCases {
		prop := btc.soongProperty
		if len(prop) > 0 {
			prop = "\n" + prop
		}
		attr := btc.bazelAttr
		if len(attr) > 0 {
			attr = "\n" + attr
		}
		runCcBinaryTests(t, bp2buildTestCase{
			description: btc.description,
			blueprint:   fmt.Sprintf(baseBlueprint, prop),
			expectedBazelTargets: []string{
				fmt.Sprintf(baseBazelTarget, attr),
			},
		})
	}
}
