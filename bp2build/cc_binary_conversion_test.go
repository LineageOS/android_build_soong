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
	"fmt"
	"strings"
	"testing"

	"android/soong/android"
	"android/soong/cc"
	"android/soong/genrule"
)

const (
	ccBinaryTypePlaceHolder = "{rule_name}"
)

type testBazelTarget struct {
	typ   string
	name  string
	attrs attrNameToString
}

func generateBazelTargetsForTest(targets []testBazelTarget) []string {
	ret := make([]string, 0, len(targets))
	for _, t := range targets {
		ret = append(ret, makeBazelTarget(t.typ, t.name, t.attrs))
	}
	return ret
}

type ccBinaryBp2buildTestCase struct {
	description string
	blueprint   string
	targets     []testBazelTarget
}

func registerCcBinaryModuleTypes(ctx android.RegistrationContext) {
	cc.RegisterCCBuildComponents(ctx)
	ctx.RegisterModuleType("filegroup", android.FileGroupFactory)
	ctx.RegisterModuleType("cc_library_static", cc.LibraryStaticFactory)
	ctx.RegisterModuleType("cc_library", cc.LibraryFactory)
	ctx.RegisterModuleType("genrule", genrule.GenRuleFactory)
}

var binaryReplacer = strings.NewReplacer(ccBinaryTypePlaceHolder, "cc_binary")
var hostBinaryReplacer = strings.NewReplacer(ccBinaryTypePlaceHolder, "cc_binary_host")

func runCcBinaryTests(t *testing.T, tc ccBinaryBp2buildTestCase) {
	t.Helper()
	runCcBinaryTestCase(t, tc)
	runCcHostBinaryTestCase(t, tc)
}

func runCcBinaryTestCase(t *testing.T, tc ccBinaryBp2buildTestCase) {
	t.Helper()
	moduleTypeUnderTest := "cc_binary"
	testCase := bp2buildTestCase{
		expectedBazelTargets:       generateBazelTargetsForTest(tc.targets),
		moduleTypeUnderTest:        moduleTypeUnderTest,
		moduleTypeUnderTestFactory: cc.BinaryFactory,
		description:                fmt.Sprintf("%s %s", moduleTypeUnderTest, tc.description),
		blueprint:                  binaryReplacer.Replace(tc.blueprint),
	}
	t.Run(testCase.description, func(t *testing.T) {
		t.Helper()
		runBp2BuildTestCase(t, registerCcBinaryModuleTypes, testCase)
	})
}

func runCcHostBinaryTestCase(t *testing.T, tc ccBinaryBp2buildTestCase) {
	t.Helper()
	testCase := tc
	for i, tar := range testCase.targets {
		switch tar.typ {
		case "cc_binary", "proto_library", "cc_lite_proto_library":
			tar.attrs["target_compatible_with"] = `select({
        "//build/bazel/platforms/os:android": ["@platforms//:incompatible"],
        "//conditions:default": [],
    })`
		}
		testCase.targets[i] = tar
	}
	moduleTypeUnderTest := "cc_binary_host"
	t.Run(testCase.description, func(t *testing.T) {
		runBp2BuildTestCase(t, registerCcBinaryModuleTypes, bp2buildTestCase{
			expectedBazelTargets:       generateBazelTargetsForTest(testCase.targets),
			moduleTypeUnderTest:        moduleTypeUnderTest,
			moduleTypeUnderTestFactory: cc.BinaryHostFactory,
			description:                fmt.Sprintf("%s %s", moduleTypeUnderTest, tc.description),
			blueprint:                  hostBinaryReplacer.Replace(testCase.blueprint),
		})
	})
}

func TestBasicCcBinary(t *testing.T) {
	runCcBinaryTests(t, ccBinaryBp2buildTestCase{
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
    sdk_version: "current",
    min_sdk_version: "29",
}
`,
		targets: []testBazelTarget{
			{"cc_binary", "foo", attrNameToString{
				"absolute_includes": `["absolute_dir"]`,
				"asflags":           `["-Dasflag"]`,
				"conlyflags":        `["-Dconlyflag"]`,
				"copts":             `["-Dcopt"]`,
				"cppflags":          `["-Dcppflag"]`,
				"linkopts":          `["ld-flag"]`,
				"local_includes": `[
        "dir",
        ".",
    ]`,
				"rtti": `True`,
				"srcs": `["a.cc"]`,
				"strip": `{
        "all": True,
        "keep_symbols": True,
        "keep_symbols_and_debug_frame": True,
        "keep_symbols_list": ["symbol"],
        "none": True,
    }`,
        "sdk_version": `"current"`,
        "min_sdk_version": `"29"`,
			},
			},
		},
	})
}

func TestCcBinaryWithSharedLdflagDisableFeature(t *testing.T) {
	runCcBinaryTests(t, ccBinaryBp2buildTestCase{
		description: `ldflag "-shared" disables static_flag feature`,
		blueprint: `
{rule_name} {
    name: "foo",
    ldflags: ["-shared"],
    include_build_directory: false,
}
`,
		targets: []testBazelTarget{
			{"cc_binary", "foo", attrNameToString{
				"features": `["-static_flag"]`,
				"linkopts": `["-shared"]`,
			},
			},
		},
	})
}

func TestCcBinaryWithLinkStatic(t *testing.T) {
	runCcBinaryTests(t, ccBinaryBp2buildTestCase{
		description: "link static",
		blueprint: `
{rule_name} {
    name: "foo",
    static_executable: true,
    include_build_directory: false,
}
`,
		targets: []testBazelTarget{
			{"cc_binary", "foo", attrNameToString{
				"linkshared": `False`,
			},
			},
		},
	})
}

func TestCcBinaryVersionScript(t *testing.T) {
	runCcBinaryTests(t, ccBinaryBp2buildTestCase{
		description: `version script`,
		blueprint: `
{rule_name} {
    name: "foo",
    include_build_directory: false,
    version_script: "vs",
}
`,
		targets: []testBazelTarget{
			{"cc_binary", "foo", attrNameToString{
				"additional_linker_inputs": `["vs"]`,
				"linkopts":                 `["-Wl,--version-script,$(location vs)"]`,
			},
			},
		},
	})
}

func TestCcBinarySplitSrcsByLang(t *testing.T) {
	runCcHostBinaryTestCase(t, ccBinaryBp2buildTestCase{
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
		targets: []testBazelTarget{
			{"cc_binary", "foo", attrNameToString{
				"srcs": `[
        "cpponly.cpp",
        ":fg_foo_cpp_srcs",
    ]`,
				"srcs_as": `[
        "asonly.S",
        ":fg_foo_as_srcs",
    ]`,
				"srcs_c": `[
        "conly.c",
        ":fg_foo_c_srcs",
    ]`,
			},
			},
		},
	})
}

func TestCcBinaryDoNotDistinguishBetweenDepsAndImplementationDeps(t *testing.T) {
	runCcBinaryTestCase(t, ccBinaryBp2buildTestCase{
		description: "no implementation deps",
		blueprint: `
genrule {
    name: "generated_hdr",
    cmd: "nothing to see here",
    bazel_module: { bp2build_available: false },
}

genrule {
    name: "export_generated_hdr",
    cmd: "nothing to see here",
    bazel_module: { bp2build_available: false },
}

{rule_name} {
    name: "foo",
    srcs: ["foo.cpp"],
    shared_libs: ["implementation_shared_dep", "shared_dep"],
    export_shared_lib_headers: ["shared_dep"],
    static_libs: ["implementation_static_dep", "static_dep"],
    export_static_lib_headers: ["static_dep", "whole_static_dep"],
    whole_static_libs: ["not_explicitly_exported_whole_static_dep", "whole_static_dep"],
    include_build_directory: false,
    generated_headers: ["generated_hdr", "export_generated_hdr"],
    export_generated_headers: ["export_generated_hdr"],
}
` +
			simpleModuleDoNotConvertBp2build("cc_library_static", "static_dep") +
			simpleModuleDoNotConvertBp2build("cc_library_static", "implementation_static_dep") +
			simpleModuleDoNotConvertBp2build("cc_library_static", "whole_static_dep") +
			simpleModuleDoNotConvertBp2build("cc_library_static", "not_explicitly_exported_whole_static_dep") +
			simpleModuleDoNotConvertBp2build("cc_library", "shared_dep") +
			simpleModuleDoNotConvertBp2build("cc_library", "implementation_shared_dep"),
		targets: []testBazelTarget{
			{"cc_binary", "foo", attrNameToString{
				"deps": `[
        ":implementation_static_dep",
        ":static_dep",
    ]`,
				"dynamic_deps": `[
        ":implementation_shared_dep",
        ":shared_dep",
    ]`,
				"srcs": `[
        "foo.cpp",
        ":generated_hdr",
        ":export_generated_hdr",
    ]`,
				"whole_archive_deps": `[
        ":not_explicitly_exported_whole_static_dep",
        ":whole_static_dep",
    ]`,
				"local_includes": `["."]`,
			},
			},
		},
	})
}

func TestCcBinaryNocrtTests(t *testing.T) {
	baseTestCases := []struct {
		description   string
		soongProperty string
		bazelAttr     attrNameToString
	}{
		{
			description:   "nocrt: true",
			soongProperty: `nocrt: true,`,
			bazelAttr:     attrNameToString{"link_crt": `False`},
		},
		{
			description:   "nocrt: false",
			soongProperty: `nocrt: false,`,
			bazelAttr:     attrNameToString{},
		},
		{
			description: "nocrt: not set",
			bazelAttr:   attrNameToString{},
		},
	}

	baseBlueprint := `{rule_name} {
    name: "foo",%s
    include_build_directory: false,
}
`

	for _, btc := range baseTestCases {
		prop := btc.soongProperty
		if len(prop) > 0 {
			prop = "\n" + prop
		}
		runCcBinaryTests(t, ccBinaryBp2buildTestCase{
			description: btc.description,
			blueprint:   fmt.Sprintf(baseBlueprint, prop),
			targets: []testBazelTarget{
				{"cc_binary", "foo", btc.bazelAttr},
			},
		})
	}
}

func TestCcBinaryNo_libcrtTests(t *testing.T) {
	baseTestCases := []struct {
		description   string
		soongProperty string
		bazelAttr     attrNameToString
	}{
		{
			description:   "no_libcrt: true",
			soongProperty: `no_libcrt: true,`,
			bazelAttr:     attrNameToString{"use_libcrt": `False`},
		},
		{
			description:   "no_libcrt: false",
			soongProperty: `no_libcrt: false,`,
			bazelAttr:     attrNameToString{"use_libcrt": `True`},
		},
		{
			description: "no_libcrt: not set",
			bazelAttr:   attrNameToString{},
		},
	}

	baseBlueprint := `{rule_name} {
    name: "foo",%s
    include_build_directory: false,
}
`

	for _, btc := range baseTestCases {
		prop := btc.soongProperty
		if len(prop) > 0 {
			prop = "\n" + prop
		}
		runCcBinaryTests(t, ccBinaryBp2buildTestCase{
			description: btc.description,
			blueprint:   fmt.Sprintf(baseBlueprint, prop),
			targets: []testBazelTarget{
				{"cc_binary", "foo", btc.bazelAttr},
			},
		})
	}
}

func TestCcBinaryPropertiesToFeatures(t *testing.T) {
	baseTestCases := []struct {
		description   string
		soongProperty string
		bazelAttr     attrNameToString
	}{
		{
			description:   "pack_relocation: true",
			soongProperty: `pack_relocations: true,`,
			bazelAttr:     attrNameToString{},
		},
		{
			description:   "pack_relocations: false",
			soongProperty: `pack_relocations: false,`,
			bazelAttr:     attrNameToString{"features": `["disable_pack_relocations"]`},
		},
		{
			description: "pack_relocations: not set",
			bazelAttr:   attrNameToString{},
		},
		{
			description:   "pack_relocation: true",
			soongProperty: `allow_undefined_symbols: true,`,
			bazelAttr:     attrNameToString{"features": `["-no_undefined_symbols"]`},
		},
		{
			description:   "allow_undefined_symbols: false",
			soongProperty: `allow_undefined_symbols: false,`,
			bazelAttr:     attrNameToString{},
		},
		{
			description: "allow_undefined_symbols: not set",
			bazelAttr:   attrNameToString{},
		},
	}

	baseBlueprint := `{rule_name} {
    name: "foo",%s
    include_build_directory: false,
}
`
	for _, btc := range baseTestCases {
		prop := btc.soongProperty
		if len(prop) > 0 {
			prop = "\n" + prop
		}
		runCcBinaryTests(t, ccBinaryBp2buildTestCase{
			description: btc.description,
			blueprint:   fmt.Sprintf(baseBlueprint, prop),
			targets: []testBazelTarget{
				{"cc_binary", "foo", btc.bazelAttr},
			},
		})
	}
}

func TestCcBinarySharedProto(t *testing.T) {
	runCcBinaryTests(t, ccBinaryBp2buildTestCase{
		blueprint: soongCcProtoLibraries + `{rule_name} {
	name: "foo",
	srcs: ["foo.proto"],
	proto: {
	},
	include_build_directory: false,
}`,
		targets: []testBazelTarget{
			{"proto_library", "foo_proto", attrNameToString{
				"srcs": `["foo.proto"]`,
			}}, {"cc_lite_proto_library", "foo_cc_proto_lite", attrNameToString{
				"deps": `[":foo_proto"]`,
			}}, {"cc_binary", "foo", attrNameToString{
				"dynamic_deps":       `[":libprotobuf-cpp-lite"]`,
				"whole_archive_deps": `[":foo_cc_proto_lite"]`,
			}},
		},
	})
}

func TestCcBinaryStaticProto(t *testing.T) {
	runCcBinaryTests(t, ccBinaryBp2buildTestCase{
		blueprint: soongCcProtoLibraries + `{rule_name} {
	name: "foo",
	srcs: ["foo.proto"],
	static_executable: true,
	proto: {
	},
	include_build_directory: false,
}`,
		targets: []testBazelTarget{
			{"proto_library", "foo_proto", attrNameToString{
				"srcs": `["foo.proto"]`,
			}}, {"cc_lite_proto_library", "foo_cc_proto_lite", attrNameToString{
				"deps": `[":foo_proto"]`,
			}}, {"cc_binary", "foo", attrNameToString{
				"deps":               `[":libprotobuf-cpp-lite"]`,
				"whole_archive_deps": `[":foo_cc_proto_lite"]`,
				"linkshared":         `False`,
			}},
		},
	})
}
