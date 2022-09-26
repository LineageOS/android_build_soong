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

package xml

import (
	"android/soong/android"
	"android/soong/bp2build"

	"testing"
)

func runXmlPrebuiltEtcTestCase(t *testing.T, tc bp2build.Bp2buildTestCase) {
	t.Helper()
	(&tc).ModuleTypeUnderTest = "prebuilt_etc_xml"
	(&tc).ModuleTypeUnderTestFactory = PrebuiltEtcXmlFactory
	bp2build.RunBp2BuildTestCase(t, registerXmlModuleTypes, tc)
}

func registerXmlModuleTypes(ctx android.RegistrationContext) {
}

func TestXmlPrebuiltEtcSimple(t *testing.T) {
	runXmlPrebuiltEtcTestCase(t, bp2build.Bp2buildTestCase{
		Description: "prebuilt_etc_xml - simple example",
		Filesystem:  map[string]string{},
		Blueprint: `
prebuilt_etc_xml {
    name: "foo",
    src: "fooSrc",
    filename: "fooFileName",
    sub_dir: "fooDir",
    schema: "foo.dtd",
}
`,
		ExpectedBazelTargets: []string{
			bp2build.MakeBazelTarget("prebuilt_xml", "foo", bp2build.AttrNameToString{
				"src":      `"fooSrc"`,
				"filename": `"fooFileName"`,
				"dir":      `"etc/fooDir"`,
				"schema":   `"foo.dtd"`,
			})}})
}

func TestXmlPrebuiltEtcFilenameFromSrc(t *testing.T) {
	runXmlPrebuiltEtcTestCase(t, bp2build.Bp2buildTestCase{
		Description: "prebuilt_etc_xml - filenameFromSrc True  ",
		Filesystem:  map[string]string{},
		Blueprint: `
prebuilt_etc_xml {
    name: "foo",
    src: "fooSrc",
    filename_from_src: true,
    sub_dir: "fooDir",
    schema: "foo.dtd",
}
`,
		ExpectedBazelTargets: []string{
			bp2build.MakeBazelTarget("prebuilt_xml", "foo", bp2build.AttrNameToString{
				"src":      `"fooSrc"`,
				"filename": `"fooSrc"`,
				"dir":      `"etc/fooDir"`,
				"schema":   `"foo.dtd"`,
			})}})
}

func TestXmlPrebuiltEtcFilenameAndFilenameFromSrc(t *testing.T) {
	runXmlPrebuiltEtcTestCase(t, bp2build.Bp2buildTestCase{
		Description: "prebuilt_etc_xml - filename provided and filenameFromSrc True  ",
		Filesystem:  map[string]string{},
		Blueprint: `
prebuilt_etc_xml {
    name: "foo",
    src: "fooSrc",
    filename: "fooFileName",
    filename_from_src: true,
    sub_dir: "fooDir",
    schema: "foo.dtd",
}
`,
		ExpectedBazelTargets: []string{
			bp2build.MakeBazelTarget("prebuilt_xml", "foo", bp2build.AttrNameToString{
				"src":      `"fooSrc"`,
				"filename": `"fooFileName"`,
				"dir":      `"etc/fooDir"`,
				"schema":   `"foo.dtd"`,
			})}})
}

func TestXmlPrebuiltEtcFileNameFromSrcMultipleSrcs(t *testing.T) {
	runXmlPrebuiltEtcTestCase(t, bp2build.Bp2buildTestCase{
		Description: "prebuilt_etc - filename_from_src is true but there are multiple srcs",
		Filesystem:  map[string]string{},
		Blueprint: `
prebuilt_etc_xml {
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
			bp2build.MakeBazelTarget("prebuilt_xml", "foo", bp2build.AttrNameToString{
				"filename_from_src": `True`,
				"dir":               `"etc"`,
				"src": `select({
        "//build/bazel/platforms/arch:arm": "barSrc",
        "//build/bazel/platforms/arch:arm64": "bazSrc",
        "//conditions:default": None,
    })`,
			})}})
}
