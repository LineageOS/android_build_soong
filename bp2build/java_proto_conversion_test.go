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
	"testing"

	"android/soong/android"
	"android/soong/java"
)

func runJavaProtoTestCase(t *testing.T, tc Bp2buildTestCase) {
	t.Helper()
	(&tc).ModuleTypeUnderTest = "java_library_static"
	(&tc).ModuleTypeUnderTestFactory = java.LibraryFactory
	RunBp2BuildTestCase(t, func(ctx android.RegistrationContext) {}, tc)
}

func TestJavaProto(t *testing.T) {
	testCases := []struct {
		protoType                string
		javaLibraryType          string
		javaLibraryNameExtension string
	}{
		{
			protoType:                "nano",
			javaLibraryType:          "java_nano_proto_library",
			javaLibraryNameExtension: "java_proto_nano",
		},
		{
			protoType:                "micro",
			javaLibraryType:          "java_micro_proto_library",
			javaLibraryNameExtension: "java_proto_micro",
		},
		{
			protoType:                "lite",
			javaLibraryType:          "java_lite_proto_library",
			javaLibraryNameExtension: "java_proto_lite",
		},
		{
			protoType:                "stream",
			javaLibraryType:          "java_stream_proto_library",
			javaLibraryNameExtension: "java_proto_stream",
		},
		{
			protoType:                "full",
			javaLibraryType:          "java_proto_library",
			javaLibraryNameExtension: "java_proto",
		},
	}

	bp := `java_library_static {
    name: "java-protos",
    proto: {
        type: "%s",
    },
    srcs: ["a.proto"],
}`

	protoLibrary := makeBazelTarget("proto_library", "java-protos_proto", AttrNameToString{
		"srcs": `["a.proto"]`,
	})

	for _, tc := range testCases {
		javaLibraryName := fmt.Sprintf("java-protos_%s", tc.javaLibraryNameExtension)

		runJavaProtoTestCase(t, Bp2buildTestCase{
			Description: fmt.Sprintf("java_proto %s", tc.protoType),
			Blueprint:   fmt.Sprintf(bp, tc.protoType),
			ExpectedBazelTargets: []string{
				protoLibrary,
				makeBazelTarget(
					tc.javaLibraryType,
					javaLibraryName,
					AttrNameToString{
						"deps": `[":java-protos_proto"]`,
					}),
				makeBazelTarget("java_library", "java-protos", AttrNameToString{
					"exports": fmt.Sprintf(`[":%s"]`, javaLibraryName),
				}),
			},
		})
	}
}

func TestJavaProtoDefault(t *testing.T) {
	runJavaProtoTestCase(t, Bp2buildTestCase{
		Description: "java_library proto default",
		Blueprint: `java_library_static {
    name: "java-protos",
    srcs: ["a.proto"],
    java_version: "7",
}
`,
		ExpectedBazelTargets: []string{
			makeBazelTarget("proto_library", "java-protos_proto", AttrNameToString{
				"srcs": `["a.proto"]`,
			}),
			makeBazelTarget(
				"java_lite_proto_library",
				"java-protos_java_proto_lite",
				AttrNameToString{
					"deps": `[":java-protos_proto"]`,
				}),
			makeBazelTarget("java_library", "java-protos", AttrNameToString{
				"exports":   `[":java-protos_java_proto_lite"]`,
				"javacopts": `["-source 1.7 -target 1.7"]`,
			}),
		},
	})
}
