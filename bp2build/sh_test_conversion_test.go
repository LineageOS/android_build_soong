// Copyright 2023 Google Inc. All rights reserved.
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
	"testing"

	"android/soong/android"
	"android/soong/sh"
)

func TestShTestSimple(t *testing.T) {
	RunBp2BuildTestCase(t, func(ctx android.RegistrationContext) {
		ctx.RegisterModuleType("filegroup", android.FileGroupFactory)
	}, Bp2buildTestCase{
		Description:         "sh_test test",
		ModuleTypeUnderTest: "sh_test",
		StubbedBuildDefinitions: []string{"android.hardware.bluetooth@1.1-service.sim",
			"android.hardware.bluetooth@1.1-impl-sim", "libc++", "libcrypto"},
		ModuleTypeUnderTestFactory: sh.ShTestFactory,
		Blueprint: simpleModule("filegroup", "android.hardware.bluetooth@1.1-service.sim") +
			simpleModule("filegroup", "android.hardware.bluetooth@1.1-impl-sim") +
			simpleModule("filegroup", "libc++") +
			simpleModule("filegroup", "libcrypto") + `sh_test{
    name: "sts-rootcanal-sidebins",
    src: "empty.sh",
    test_suites: [
        "sts",
        "sts-lite",
    ],
    data_bins: [
        "android.hardware.bluetooth@1.1-service.sim",
        "android.hardware.bluetooth@1.1-impl-sim"
    ],
    data: ["android.hardware.bluetooth@1.1-service.sim.rc"],
    data_libs: ["libc++","libcrypto"],
		test_config: "art-gtests-target-install-apex.xml",
		test_config_template: ":art-run-test-target-template",
		auto_gen_config: false,
    test_options:{tags: ["no-remote"],
	},
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("sh_test", "sts-rootcanal-sidebins", AttrNameToString{
				"srcs":    `["empty.sh"]`,
				"runs_on": `["device"]`,
				"data": `[
        "android.hardware.bluetooth@1.1-service.sim.rc",
        ":android.hardware.bluetooth@1.1-service.sim",
        ":android.hardware.bluetooth@1.1-impl-sim",
        ":libc++",
        ":libcrypto",
    ]`,
				"data_bins": `[
        ":android.hardware.bluetooth@1.1-service.sim",
        ":android.hardware.bluetooth@1.1-impl-sim",
    ]`,
				"tags": `["no-remote"]`,
			})},
	})
}

func TestShTestHostSimple(t *testing.T) {
	RunBp2BuildTestCase(t, func(ctx android.RegistrationContext) {
		ctx.RegisterModuleType("filegroup", android.FileGroupFactory)
	}, Bp2buildTestCase{
		Description:         "sh_test_host test",
		ModuleTypeUnderTest: "sh_test_host",
		StubbedBuildDefinitions: []string{"android.hardware.bluetooth@1.1-service.sim",
			"android.hardware.bluetooth@1.1-impl-sim", "libc++", "libcrypto"},
		ModuleTypeUnderTestFactory: sh.ShTestHostFactory,
		Blueprint: simpleModule("filegroup", "android.hardware.bluetooth@1.1-service.sim") +
			simpleModule("filegroup", "android.hardware.bluetooth@1.1-impl-sim") +
			simpleModule("filegroup", "libc++") +
			simpleModule("filegroup", "libcrypto") + `sh_test_host{
    name: "sts-rootcanal-sidebins",
    src: "empty.sh",
    test_suites: [
        "sts",
        "sts-lite",
    ],
    data_bins: [
        "android.hardware.bluetooth@1.1-service.sim",
        "android.hardware.bluetooth@1.1-impl-sim"
    ],
    data: ["android.hardware.bluetooth@1.1-service.sim.rc"],
    data_libs: ["libc++","libcrypto"],
		test_config: "art-gtests-target-install-apex.xml",
		test_config_template: ":art-run-test-target-template",
		auto_gen_config: false,
    test_options:{tags: ["no-remote"],
	},
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("sh_test", "sts-rootcanal-sidebins", AttrNameToString{
				"srcs":    `["empty.sh"]`,
				"runs_on": `["host_without_device"]`,
				"data": `[
        "android.hardware.bluetooth@1.1-service.sim.rc",
        ":android.hardware.bluetooth@1.1-service.sim",
        ":android.hardware.bluetooth@1.1-impl-sim",
        ":libc++",
        ":libcrypto",
    ]`,
				"data_bins": `[
        ":android.hardware.bluetooth@1.1-service.sim",
        ":android.hardware.bluetooth@1.1-impl-sim",
    ]`,
				"tags": `["no-remote"]`,
				"target_compatible_with": `select({
        "//build/bazel_common_rules/platforms/os:android": ["@platforms//:incompatible"],
        "//conditions:default": [],
    })`,
			})},
	})
}

func TestShTestAutogen(t *testing.T) {
	RunBp2BuildTestCase(t, func(ctx android.RegistrationContext) {
		ctx.RegisterModuleType("filegroup", android.FileGroupFactory)
	}, Bp2buildTestCase{
		Description:         "sh_test test",
		ModuleTypeUnderTest: "sh_test",
		StubbedBuildDefinitions: []string{"android.hardware.bluetooth@1.1-service.sim",
			"android.hardware.bluetooth@1.1-impl-sim", "libc++", "libcrypto", "art-run-test-target-template"},
		ModuleTypeUnderTestFactory: sh.ShTestFactory,
		Blueprint: simpleModule("filegroup", "android.hardware.bluetooth@1.1-service.sim") +
			simpleModule("filegroup", "android.hardware.bluetooth@1.1-impl-sim") +
			simpleModule("filegroup", "libc++") +
			simpleModule("filegroup", "libcrypto") +
			simpleModule("filegroup", "art-run-test-target-template") + `sh_test{
    name: "sts-rootcanal-sidebins",
    src: "empty.sh",
    test_suites: [
        "sts",
        "sts-lite",
    ],
    data_bins: [
        "android.hardware.bluetooth@1.1-service.sim",
        "android.hardware.bluetooth@1.1-impl-sim"
    ],
    data: ["android.hardware.bluetooth@1.1-service.sim.rc"],
    data_libs: ["libc++","libcrypto"],
		test_config: "art-gtests-target-install-apex.xml",
		test_config_template: ":art-run-test-target-template",
		auto_gen_config: true,
    test_options:{tags: ["no-remote"],
	},
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("sh_test", "sts-rootcanal-sidebins", AttrNameToString{
				"srcs":                      `["empty.sh"]`,
				"runs_on":                   `["device"]`,
				"auto_generate_test_config": "True",
				"target_compatible_with":    `["//build/bazel_common_rules/platforms/os:android"]`,
				"template_test_config":      `":art-run-test-target-template"`,
				"data": `[
        "android.hardware.bluetooth@1.1-service.sim.rc",
        ":android.hardware.bluetooth@1.1-service.sim",
        ":android.hardware.bluetooth@1.1-impl-sim",
        ":libc++",
        ":libcrypto",
    ]`,
				"data_bins": `[
        ":android.hardware.bluetooth@1.1-service.sim",
        ":android.hardware.bluetooth@1.1-impl-sim",
    ]`,
				"tags": `["no-remote"]`,
			})},
	})
}

func TestShTestHostAutogen(t *testing.T) {
	RunBp2BuildTestCase(t, func(ctx android.RegistrationContext) {
		ctx.RegisterModuleType("filegroup", android.FileGroupFactory)
	}, Bp2buildTestCase{
		Description:         "sh_test_host test",
		ModuleTypeUnderTest: "sh_test_host",
		StubbedBuildDefinitions: []string{"android.hardware.bluetooth@1.1-service.sim",
			"android.hardware.bluetooth@1.1-impl-sim", "libc++", "libcrypto", "art-run-test-target-template"},
		ModuleTypeUnderTestFactory: sh.ShTestHostFactory,
		Blueprint: simpleModule("filegroup", "android.hardware.bluetooth@1.1-service.sim") +
			simpleModule("filegroup", "android.hardware.bluetooth@1.1-impl-sim") +
			simpleModule("filegroup", "libc++") +
			simpleModule("filegroup", "libcrypto") +
			simpleModule("filegroup", "art-run-test-target-template") + `sh_test_host{
    name: "sts-rootcanal-sidebins",
    src: "empty.sh",
    test_suites: [
        "sts",
        "sts-lite",
    ],
    data_bins: [
        "android.hardware.bluetooth@1.1-service.sim",
        "android.hardware.bluetooth@1.1-impl-sim"
    ],
    data: ["android.hardware.bluetooth@1.1-service.sim.rc"],
    data_libs: ["libc++","libcrypto"],
		test_config: "art-gtests-target-install-apex.xml",
		test_config_template: ":art-run-test-target-template",
		auto_gen_config: true,
    test_options:{tags: ["no-remote"],
	},
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("sh_test", "sts-rootcanal-sidebins", AttrNameToString{
				"srcs":                      `["empty.sh"]`,
				"runs_on":                   `["host_without_device"]`,
				"auto_generate_test_config": "True",
				"target_compatible_with": `select({
        "//build/bazel_common_rules/platforms/os:android": ["@platforms//:incompatible"],
        "//conditions:default": [],
    })`,
				"template_test_config": `":art-run-test-target-template"`,
				"data": `[
        "android.hardware.bluetooth@1.1-service.sim.rc",
        ":android.hardware.bluetooth@1.1-service.sim",
        ":android.hardware.bluetooth@1.1-impl-sim",
        ":libc++",
        ":libcrypto",
    ]`,
				"data_bins": `[
        ":android.hardware.bluetooth@1.1-service.sim",
        ":android.hardware.bluetooth@1.1-impl-sim",
    ]`,
				"tags": `["no-remote"]`,
			})},
	})
}
func TestShTestSimpleUnset(t *testing.T) {
	RunBp2BuildTestCase(t, func(ctx android.RegistrationContext) {
		ctx.RegisterModuleType("filegroup", android.FileGroupFactory)
	}, Bp2buildTestCase{
		Description:         "sh_test test",
		ModuleTypeUnderTest: "sh_test",
		StubbedBuildDefinitions: []string{"android.hardware.bluetooth@1.1-service.sim",
			"android.hardware.bluetooth@1.1-impl-sim", "libc++", "libcrypto"},
		ModuleTypeUnderTestFactory: sh.ShTestFactory,
		Blueprint: simpleModule("filegroup", "android.hardware.bluetooth@1.1-service.sim") +
			simpleModule("filegroup", "android.hardware.bluetooth@1.1-impl-sim") +
			simpleModule("filegroup", "libc++") +
			simpleModule("filegroup", "libcrypto") + `sh_test{
    name: "sts-rootcanal-sidebins",
    src: "empty.sh",
    test_suites: [
        "sts",
        "sts-lite",
    ],
    data_bins: [
        "android.hardware.bluetooth@1.1-service.sim",
        "android.hardware.bluetooth@1.1-impl-sim"
    ],
    data: ["android.hardware.bluetooth@1.1-service.sim.rc"],
    data_libs: ["libc++","libcrypto"],
    test_options:{tags: ["no-remote"],
	},
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("sh_test", "sts-rootcanal-sidebins", AttrNameToString{
				"srcs":    `["empty.sh"]`,
				"runs_on": `["device"]`,
				"data": `[
        "android.hardware.bluetooth@1.1-service.sim.rc",
        ":android.hardware.bluetooth@1.1-service.sim",
        ":android.hardware.bluetooth@1.1-impl-sim",
        ":libc++",
        ":libcrypto",
    ]`,
				"data_bins": `[
        ":android.hardware.bluetooth@1.1-service.sim",
        ":android.hardware.bluetooth@1.1-impl-sim",
    ]`,
				"tags": `["no-remote"]`,
			})},
	})
}

func TestShTestHostSimpleUnset(t *testing.T) {
	RunBp2BuildTestCase(t, func(ctx android.RegistrationContext) {
		ctx.RegisterModuleType("filegroup", android.FileGroupFactory)
	}, Bp2buildTestCase{
		Description:                "sh_test_host test",
		ModuleTypeUnderTest:        "sh_test_host",
		ModuleTypeUnderTestFactory: sh.ShTestHostFactory,
		StubbedBuildDefinitions: []string{"android.hardware.bluetooth@1.1-service.sim",
			"android.hardware.bluetooth@1.1-impl-sim", "libc++", "libcrypto"},
		Blueprint: simpleModule("filegroup", "android.hardware.bluetooth@1.1-service.sim") +
			simpleModule("filegroup", "android.hardware.bluetooth@1.1-impl-sim") +
			simpleModule("filegroup", "libc++") +
			simpleModule("filegroup", "libcrypto") + `sh_test_host{
    name: "sts-rootcanal-sidebins",
    src: "empty.sh",
    test_suites: [
        "sts",
        "sts-lite",
    ],
    data_bins: [
        "android.hardware.bluetooth@1.1-service.sim",
        "android.hardware.bluetooth@1.1-impl-sim"
    ],
    data: ["android.hardware.bluetooth@1.1-service.sim.rc"],
    data_libs: ["libc++","libcrypto"],
    test_options:{tags: ["no-remote"],
	},
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("sh_test", "sts-rootcanal-sidebins", AttrNameToString{
				"srcs":    `["empty.sh"]`,
				"runs_on": `["host_without_device"]`,
				"data": `[
        "android.hardware.bluetooth@1.1-service.sim.rc",
        ":android.hardware.bluetooth@1.1-service.sim",
        ":android.hardware.bluetooth@1.1-impl-sim",
        ":libc++",
        ":libcrypto",
    ]`,
				"data_bins": `[
        ":android.hardware.bluetooth@1.1-service.sim",
        ":android.hardware.bluetooth@1.1-impl-sim",
    ]`,
				"tags": `["no-remote"]`,
				"target_compatible_with": `select({
        "//build/bazel_common_rules/platforms/os:android": ["@platforms//:incompatible"],
        "//conditions:default": [],
    })`,
			})},
	})
}
