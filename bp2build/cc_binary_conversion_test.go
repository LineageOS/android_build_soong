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
	attrs AttrNameToString
}

func generateBazelTargetsForTest(targets []testBazelTarget, hod android.HostOrDeviceSupported) []string {
	ret := make([]string, 0, len(targets))
	for _, t := range targets {
		attrs := t.attrs.clone()
		ret = append(ret, makeBazelTargetHostOrDevice(t.typ, t.name, attrs, hod))
	}
	return ret
}

type ccBinaryBp2buildTestCase struct {
	description string
	filesystem  map[string]string
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

func runCcBinaryTestCase(t *testing.T, testCase ccBinaryBp2buildTestCase) {
	t.Helper()
	moduleTypeUnderTest := "cc_binary"

	description := fmt.Sprintf("%s %s", moduleTypeUnderTest, testCase.description)
	t.Run(description, func(t *testing.T) {
		t.Helper()
		RunBp2BuildTestCase(t, registerCcBinaryModuleTypes, Bp2buildTestCase{
			ExpectedBazelTargets:       generateBazelTargetsForTest(testCase.targets, android.DeviceSupported),
			ModuleTypeUnderTest:        moduleTypeUnderTest,
			ModuleTypeUnderTestFactory: cc.BinaryFactory,
			Description:                description,
			Blueprint:                  binaryReplacer.Replace(testCase.blueprint),
			Filesystem:                 testCase.filesystem,
		})
	})
}

func runCcHostBinaryTestCase(t *testing.T, testCase ccBinaryBp2buildTestCase) {
	t.Helper()
	moduleTypeUnderTest := "cc_binary_host"
	description := fmt.Sprintf("%s %s", moduleTypeUnderTest, testCase.description)
	t.Run(description, func(t *testing.T) {
		RunBp2BuildTestCase(t, registerCcBinaryModuleTypes, Bp2buildTestCase{
			ExpectedBazelTargets:       generateBazelTargetsForTest(testCase.targets, android.HostSupported),
			ModuleTypeUnderTest:        moduleTypeUnderTest,
			ModuleTypeUnderTestFactory: cc.BinaryHostFactory,
			Description:                description,
			Blueprint:                  hostBinaryReplacer.Replace(testCase.blueprint),
			Filesystem:                 testCase.filesystem,
		})
	})
}

func TestBasicCcBinary(t *testing.T) {
	runCcBinaryTests(t, ccBinaryBp2buildTestCase{
		description: "basic -- properties -> attrs with little/no transformation",
		filesystem: map[string]string{
			soongCcVersionLibBpPath: soongCcVersionLibBp,
		},
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
    use_version_lib: true,
}
`,
		targets: []testBazelTarget{
			{"cc_binary", "foo", AttrNameToString{
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
				"sdk_version":        `"current"`,
				"min_sdk_version":    `"29"`,
				"use_version_lib":    `True`,
				"whole_archive_deps": `["//build/soong/cc/libbuildversion:libbuildversion"]`,
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
			{"cc_binary", "foo", AttrNameToString{
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
			{"cc_binary", "foo", AttrNameToString{
				"linkshared": `False`,
			},
			},
		},
	})
}

func TestCcBinaryVersionScriptAndDynamicList(t *testing.T) {
	runCcBinaryTests(t, ccBinaryBp2buildTestCase{
		description: `version script and dynamic list`,
		blueprint: `
{rule_name} {
    name: "foo",
    include_build_directory: false,
    version_script: "vs",
    dynamic_list: "dynamic.list",
}
`,
		targets: []testBazelTarget{
			{"cc_binary", "foo", AttrNameToString{
				"additional_linker_inputs": `[
        "vs",
        "dynamic.list",
    ]`,
				"linkopts": `[
        "-Wl,--version-script,$(location vs)",
        "-Wl,--dynamic-list,$(location dynamic.list)",
    ]`,
			},
			},
		},
	})
}

func TestCcBinaryLdflagsSplitBySpaceExceptSoongAdded(t *testing.T) {
	runCcBinaryTests(t, ccBinaryBp2buildTestCase{
		description: "ldflags are split by spaces except for the ones added by soong (version script and dynamic list)",
		blueprint: `
{rule_name} {
    name: "foo",
		ldflags: [
			"--nospace_flag",
			"-z spaceflag",
		],
		version_script: "version_script",
		dynamic_list: "dynamic.list",
    include_build_directory: false,
}
`,
		targets: []testBazelTarget{
			{"cc_binary", "foo", AttrNameToString{
				"additional_linker_inputs": `[
        "version_script",
        "dynamic.list",
    ]`,
				"linkopts": `[
        "--nospace_flag",
        "-z",
        "spaceflag",
        "-Wl,--version-script,$(location version_script)",
        "-Wl,--dynamic-list,$(location dynamic.list)",
    ]`,
			}}},
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
			{"cc_binary", "foo", AttrNameToString{
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
			{"cc_binary", "foo", AttrNameToString{
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
		bazelAttr     AttrNameToString
	}{
		{
			description:   "nocrt: true",
			soongProperty: `nocrt: true,`,
			bazelAttr:     AttrNameToString{"link_crt": `False`},
		},
		{
			description:   "nocrt: false",
			soongProperty: `nocrt: false,`,
			bazelAttr:     AttrNameToString{},
		},
		{
			description: "nocrt: not set",
			bazelAttr:   AttrNameToString{},
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
		bazelAttr     AttrNameToString
	}{
		{
			description:   "no_libcrt: true",
			soongProperty: `no_libcrt: true,`,
			bazelAttr:     AttrNameToString{"use_libcrt": `False`},
		},
		{
			description:   "no_libcrt: false",
			soongProperty: `no_libcrt: false,`,
			bazelAttr:     AttrNameToString{"use_libcrt": `True`},
		},
		{
			description: "no_libcrt: not set",
			bazelAttr:   AttrNameToString{},
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
		bazelAttr     AttrNameToString
	}{
		{
			description:   "pack_relocation: true",
			soongProperty: `pack_relocations: true,`,
			bazelAttr:     AttrNameToString{},
		},
		{
			description:   "pack_relocations: false",
			soongProperty: `pack_relocations: false,`,
			bazelAttr:     AttrNameToString{"features": `["disable_pack_relocations"]`},
		},
		{
			description: "pack_relocations: not set",
			bazelAttr:   AttrNameToString{},
		},
		{
			description:   "pack_relocation: true",
			soongProperty: `allow_undefined_symbols: true,`,
			bazelAttr:     AttrNameToString{"features": `["-no_undefined_symbols"]`},
		},
		{
			description:   "allow_undefined_symbols: false",
			soongProperty: `allow_undefined_symbols: false,`,
			bazelAttr:     AttrNameToString{},
		},
		{
			description: "allow_undefined_symbols: not set",
			bazelAttr:   AttrNameToString{},
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
			{"proto_library", "foo_proto", AttrNameToString{
				"srcs": `["foo.proto"]`,
			}}, {"cc_lite_proto_library", "foo_cc_proto_lite", AttrNameToString{
				"deps": `[":foo_proto"]`,
			}}, {"cc_binary", "foo", AttrNameToString{
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
			{"proto_library", "foo_proto", AttrNameToString{
				"srcs": `["foo.proto"]`,
			}}, {"cc_lite_proto_library", "foo_cc_proto_lite", AttrNameToString{
				"deps": `[":foo_proto"]`,
			}}, {"cc_binary", "foo", AttrNameToString{
				"deps":               `[":libprotobuf-cpp-lite"]`,
				"whole_archive_deps": `[":foo_cc_proto_lite"]`,
				"linkshared":         `False`,
			}},
		},
	})
}

func TestCcBinaryConvertLex(t *testing.T) {
	runCcBinaryTests(t, ccBinaryBp2buildTestCase{
		description: `.l and .ll sources converted to .c and .cc`,
		blueprint: `
{rule_name} {
    name: "foo",
		srcs: ["foo.c", "bar.cc", "foo1.l", "foo2.l", "bar1.ll", "bar2.ll"],
		lex: { flags: ["--foo_opt", "--bar_opt"] },
		include_build_directory: false,
}
`,
		targets: []testBazelTarget{
			{"genlex", "foo_genlex_l", AttrNameToString{
				"srcs": `[
        "foo1.l",
        "foo2.l",
    ]`,
				"lexopts": `[
        "--foo_opt",
        "--bar_opt",
    ]`,
			}},
			{"genlex", "foo_genlex_ll", AttrNameToString{
				"srcs": `[
        "bar1.ll",
        "bar2.ll",
    ]`,
				"lexopts": `[
        "--foo_opt",
        "--bar_opt",
    ]`,
			}},
			{"cc_binary", "foo", AttrNameToString{
				"srcs": `[
        "bar.cc",
        ":foo_genlex_ll",
    ]`,
				"srcs_c": `[
        "foo.c",
        ":foo_genlex_l",
    ]`,
			}},
		},
	})
}

func TestCcBinaryRuntimeLibs(t *testing.T) {
	runCcBinaryTests(t, ccBinaryBp2buildTestCase{
		description: "cc_binary with runtime libs",
		blueprint: `
cc_library {
    name: "bar",
    srcs: ["b.cc"],
}

{rule_name} {
    name: "foo",
    srcs: ["a.cc"],
    runtime_libs: ["bar"],
}
`,
		targets: []testBazelTarget{
			{"cc_library_static", "bar_bp2build_cc_library_static", AttrNameToString{
				"local_includes":         `["."]`,
				"srcs":                   `["b.cc"]`,
				"target_compatible_with": `["//build/bazel/platforms/os:android"]`,
			},
			},
			{"cc_library_shared", "bar", AttrNameToString{
				"local_includes":         `["."]`,
				"srcs":                   `["b.cc"]`,
				"target_compatible_with": `["//build/bazel/platforms/os:android"]`,
			},
			},
			{"cc_binary", "foo", AttrNameToString{
				"local_includes": `["."]`,
				"srcs":           `["a.cc"]`,
				"runtime_deps":   `[":bar"]`,
			},
			},
		},
	})
}

func TestCcBinaryWithInstructionSet(t *testing.T) {
	runCcBinaryTests(t, ccBinaryBp2buildTestCase{
		description: "instruction set",
		blueprint: `
{rule_name} {
    name: "foo",
    arch: {
      arm: {
        instruction_set: "arm",
      }
    }
}
`,
		targets: []testBazelTarget{
			{"cc_binary", "foo", AttrNameToString{
				"features": `select({
        "//build/bazel/platforms/arch:arm": [
            "arm_isa_arm",
            "-arm_isa_thumb",
        ],
        "//conditions:default": [],
    })`,
				"local_includes": `["."]`,
			}},
		},
	})
}

func TestCcBinaryEmptySuffix(t *testing.T) {
	runCcBinaryTests(t, ccBinaryBp2buildTestCase{
		description: "binary with empty suffix",
		blueprint: `
{rule_name} {
    name: "foo",
    suffix: "",
}`,
		targets: []testBazelTarget{
			{"cc_binary", "foo", AttrNameToString{
				"local_includes": `["."]`,
				"suffix":         `""`,
			}},
		},
	})
}

func TestCcBinarySuffix(t *testing.T) {
	runCcBinaryTests(t, ccBinaryBp2buildTestCase{
		description: "binary with suffix",
		blueprint: `
{rule_name} {
    name: "foo",
    suffix: "-suf",
}
`,
		targets: []testBazelTarget{
			{"cc_binary", "foo", AttrNameToString{
				"local_includes": `["."]`,
				"suffix":         `"-suf"`,
			}},
		},
	})
}

func TestCcArchVariantBinarySuffix(t *testing.T) {
	runCcBinaryTests(t, ccBinaryBp2buildTestCase{
		description: "binary with suffix",
		blueprint: `
{rule_name} {
    name: "foo",
    arch: {
        arm64: { suffix: "-64" },
        arm:   { suffix: "-32" },
		},
}
`,
		targets: []testBazelTarget{
			{"cc_binary", "foo", AttrNameToString{
				"local_includes": `["."]`,
				"suffix": `select({
        "//build/bazel/platforms/arch:arm": "-32",
        "//build/bazel/platforms/arch:arm64": "-64",
        "//conditions:default": None,
    })`,
			}},
		},
	})
}

func TestCcBinaryWithSyspropSrcs(t *testing.T) {
	runCcBinaryTestCase(t, ccBinaryBp2buildTestCase{
		description: "cc_binary with sysprop sources",
		blueprint: `
{rule_name} {
	name: "foo",
	srcs: [
		"bar.sysprop",
		"baz.sysprop",
		"blah.cpp",
	],
	min_sdk_version: "5",
}`,
		targets: []testBazelTarget{
			{"sysprop_library", "foo_sysprop_library", AttrNameToString{
				"srcs": `[
        "bar.sysprop",
        "baz.sysprop",
    ]`,
			}},
			{"cc_sysprop_library_static", "foo_cc_sysprop_library_static", AttrNameToString{
				"dep":             `":foo_sysprop_library"`,
				"min_sdk_version": `"5"`,
			}},
			{"cc_binary", "foo", AttrNameToString{
				"srcs":               `["blah.cpp"]`,
				"local_includes":     `["."]`,
				"min_sdk_version":    `"5"`,
				"whole_archive_deps": `[":foo_cc_sysprop_library_static"]`,
			}},
		},
	})
}

func TestCcBinaryWithSyspropSrcsSomeConfigs(t *testing.T) {
	runCcBinaryTestCase(t, ccBinaryBp2buildTestCase{
		description: "cc_binary with sysprop sources in some configs but not others",
		blueprint: `
{rule_name} {
	name: "foo",
	srcs: [
		"blah.cpp",
	],
	target: {
		android: {
			srcs: ["bar.sysprop"],
		},
	},
	min_sdk_version: "5",
}`,
		targets: []testBazelTarget{
			{"sysprop_library", "foo_sysprop_library", AttrNameToString{
				"srcs": `select({
        "//build/bazel/platforms/os:android": ["bar.sysprop"],
        "//conditions:default": [],
    })`,
			}},
			{"cc_sysprop_library_static", "foo_cc_sysprop_library_static", AttrNameToString{
				"dep":             `":foo_sysprop_library"`,
				"min_sdk_version": `"5"`,
			}},
			{"cc_binary", "foo", AttrNameToString{
				"srcs":            `["blah.cpp"]`,
				"local_includes":  `["."]`,
				"min_sdk_version": `"5"`,
				"whole_archive_deps": `select({
        "//build/bazel/platforms/os:android": [":foo_cc_sysprop_library_static"],
        "//conditions:default": [],
    })`,
			}},
		},
	})
}

func TestCcBinaryWithIntegerOverflowProperty(t *testing.T) {
	runCcBinaryTestCase(t, ccBinaryBp2buildTestCase{
		description: "cc_binary with integer overflow property specified",
		blueprint: `
{rule_name} {
	name: "foo",
	sanitize: {
		integer_overflow: true,
	},
}`,
		targets: []testBazelTarget{
			{"cc_binary", "foo", AttrNameToString{
				"local_includes": `["."]`,
				"features":       `["ubsan_integer_overflow"]`,
			}},
		},
	})
}

func TestCcBinaryWithMiscUndefinedProperty(t *testing.T) {
	runCcBinaryTestCase(t, ccBinaryBp2buildTestCase{
		description: "cc_binary with miscellaneous properties specified",
		blueprint: `
{rule_name} {
	name: "foo",
	sanitize: {
		misc_undefined: ["undefined", "nullability"],
	},
}`,
		targets: []testBazelTarget{
			{"cc_binary", "foo", AttrNameToString{
				"local_includes": `["."]`,
				"features": `[
        "ubsan_undefined",
        "ubsan_nullability",
    ]`,
			}},
		},
	})
}

func TestCcBinaryWithUBSanPropertiesArchSpecific(t *testing.T) {
	runCcBinaryTestCase(t, ccBinaryBp2buildTestCase{
		description: "cc_binary has correct feature select when UBSan props are specified in arch specific blocks",
		blueprint: `
{rule_name} {
	name: "foo",
	sanitize: {
		misc_undefined: ["undefined", "nullability"],
	},
	target: {
			android: {
					sanitize: {
							misc_undefined: ["alignment"],
					},
			},
			linux_glibc: {
					sanitize: {
							integer_overflow: true,
					},
			},
	},
}`,
		targets: []testBazelTarget{
			{"cc_binary", "foo", AttrNameToString{
				"local_includes": `["."]`,
				"features": `[
        "ubsan_undefined",
        "ubsan_nullability",
    ] + select({
        "//build/bazel/platforms/os:android": ["ubsan_alignment"],
        "//build/bazel/platforms/os:linux": ["ubsan_integer_overflow"],
        "//conditions:default": [],
    })`,
			}},
		},
	})
}
