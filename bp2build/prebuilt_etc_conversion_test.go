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
	"android/soong/etc"

	"testing"
)

func runPrebuiltEtcTestCase(t *testing.T, tc Bp2buildTestCase) {
	t.Helper()
	(&tc).ModuleTypeUnderTest = "prebuilt_etc"
	(&tc).ModuleTypeUnderTestFactory = etc.PrebuiltEtcFactory
	RunBp2BuildTestCase(t, registerPrebuiltEtcModuleTypes, tc)
}

func registerPrebuiltEtcModuleTypes(ctx android.RegistrationContext) {
}

func TestPrebuiltEtcSimple(t *testing.T) {
	runPrebuiltEtcTestCase(t, Bp2buildTestCase{
		Description: "prebuilt_etc - simple example",
		Filesystem:  map[string]string{},
		Blueprint: `
prebuilt_etc {
    name: "apex_tz_version",
    src: "version/tz_version",
    filename: "tz_version",
    sub_dir: "tz",
    installable: false,
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("prebuilt_file", "apex_tz_version", AttrNameToString{
				"filename":    `"tz_version"`,
				"installable": `False`,
				"src":         `"version/tz_version"`,
				"dir":         `"etc/tz"`,
			})}})
}

func TestPrebuiltEtcArchVariant(t *testing.T) {
	runPrebuiltEtcTestCase(t, Bp2buildTestCase{
		Description: "prebuilt_etc - arch variant",
		Filesystem:  map[string]string{},
		Blueprint: `
prebuilt_etc {
    name: "apex_tz_version",
    src: "version/tz_version",
    filename: "tz_version",
    sub_dir: "tz",
    installable: false,
    arch: {
      arm: {
        src: "arm",
      },
      arm64: {
        src: "arm64",
      },
    }
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("prebuilt_file", "apex_tz_version", AttrNameToString{
				"filename":    `"tz_version"`,
				"installable": `False`,
				"src": `select({
        "//build/bazel/platforms/arch:arm": "arm",
        "//build/bazel/platforms/arch:arm64": "arm64",
        "//conditions:default": "version/tz_version",
    })`,
				"dir": `"etc/tz"`,
			})}})
}

func TestPrebuiltEtcArchAndTargetVariant(t *testing.T) {
	runPrebuiltEtcTestCase(t, Bp2buildTestCase{
		Description: "prebuilt_etc - arch variant",
		Filesystem:  map[string]string{},
		Blueprint: `
prebuilt_etc {
    name: "apex_tz_version",
    src: "version/tz_version",
    filename: "tz_version",
    sub_dir: "tz",
    installable: false,
    arch: {
      arm: {
        src: "arm",
      },
      arm64: {
        src: "darwin_or_arm64",
      },
    },
    target: {
      darwin: {
        src: "darwin_or_arm64",
      }
    },
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("prebuilt_file", "apex_tz_version", AttrNameToString{
				"filename":    `"tz_version"`,
				"installable": `False`,
				"src": `select({
        "//build/bazel/platforms/os_arch:android_arm": "arm",
        "//build/bazel/platforms/os_arch:android_arm64": "darwin_or_arm64",
        "//build/bazel/platforms/os_arch:darwin_arm64": "darwin_or_arm64",
        "//build/bazel/platforms/os_arch:darwin_x86_64": "darwin_or_arm64",
        "//build/bazel/platforms/os_arch:linux_bionic_arm64": "darwin_or_arm64",
        "//conditions:default": "version/tz_version",
    })`,
				"dir": `"etc/tz"`,
			})}})
}

func runPrebuiltUsrShareTestCase(t *testing.T, tc Bp2buildTestCase) {
	t.Helper()
	(&tc).ModuleTypeUnderTest = "prebuilt_usr_share"
	(&tc).ModuleTypeUnderTestFactory = etc.PrebuiltUserShareFactory
	RunBp2BuildTestCase(t, registerPrebuiltEtcModuleTypes, tc)
}

func registerPrebuiltUsrShareModuleTypes(ctx android.RegistrationContext) {
}

func TestPrebuiltUsrShareSimple(t *testing.T) {
	runPrebuiltUsrShareTestCase(t, Bp2buildTestCase{
		Description: "prebuilt_usr_share - simple example",
		Filesystem:  map[string]string{},
		Blueprint: `
prebuilt_usr_share {
    name: "apex_tz_version",
    src: "version/tz_version",
    filename: "tz_version",
    sub_dir: "tz",
    installable: false,
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("prebuilt_file", "apex_tz_version", AttrNameToString{
				"filename":    `"tz_version"`,
				"installable": `False`,
				"src":         `"version/tz_version"`,
				"dir":         `"usr/share/tz"`,
			})}})
}

func TestPrebuiltEtcNoSubdir(t *testing.T) {
	runPrebuiltEtcTestCase(t, Bp2buildTestCase{
		Description: "prebuilt_etc - no subdir",
		Filesystem:  map[string]string{},
		Blueprint: `
prebuilt_etc {
    name: "apex_tz_version",
    src: "version/tz_version",
    filename: "tz_version",
    installable: false,
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("prebuilt_file", "apex_tz_version", AttrNameToString{
				"filename":    `"tz_version"`,
				"installable": `False`,
				"src":         `"version/tz_version"`,
				"dir":         `"etc"`,
			})}})
}

func TestFilenameAsProperty(t *testing.T) {
	runPrebuiltEtcTestCase(t, Bp2buildTestCase{
		Description: "prebuilt_etc - filename is specified as a property ",
		Filesystem:  map[string]string{},
		Blueprint: `
prebuilt_etc {
    name: "foo",
    src: "fooSrc",
    filename: "fooFileName",
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("prebuilt_file", "foo", AttrNameToString{
				"filename": `"fooFileName"`,
				"src":      `"fooSrc"`,
				"dir":      `"etc"`,
			})}})
}

func TestFileNameFromSrc(t *testing.T) {
	runPrebuiltEtcTestCase(t, Bp2buildTestCase{
		Description: "prebuilt_etc - filename_from_src is true  ",
		Filesystem:  map[string]string{},
		Blueprint: `
prebuilt_etc {
    name: "foo",
    filename_from_src: true,
    src: "fooSrc",
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("prebuilt_file", "foo", AttrNameToString{
				"filename": `"fooSrc"`,
				"src":      `"fooSrc"`,
				"dir":      `"etc"`,
			})}})
}

func TestFileNameFromSrcMultipleSrcs(t *testing.T) {
	runPrebuiltEtcTestCase(t, Bp2buildTestCase{
		Description: "prebuilt_etc - filename_from_src is true but there are multiple srcs",
		Filesystem:  map[string]string{},
		Blueprint: `
prebuilt_etc {
    name: "foo",
    filename_from_src: true,
		arch: {
        arm: {
            src: "barSrc",
        },
        arm64: {
            src: "bazSrc",
        },
	  }
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("prebuilt_file", "foo", AttrNameToString{
				"filename_from_src": `True`,
				"dir":               `"etc"`,
				"src": `select({
        "//build/bazel/platforms/arch:arm": "barSrc",
        "//build/bazel/platforms/arch:arm64": "bazSrc",
        "//conditions:default": None,
    })`,
			})}})
}

func TestFilenameFromModuleName(t *testing.T) {
	runPrebuiltEtcTestCase(t, Bp2buildTestCase{
		Description: "prebuilt_etc - neither filename nor filename_from_src are specified ",
		Filesystem:  map[string]string{},
		Blueprint: `
prebuilt_etc {
    name: "foo",
}
`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("prebuilt_file", "foo", AttrNameToString{
				"filename": `"foo"`,
				"dir":      `"etc"`,
			})}})
}
