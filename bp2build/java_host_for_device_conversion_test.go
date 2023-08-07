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
	"android/soong/java"
)

func runJavaHostForDeviceTestCaseWithRegistrationCtxFunc(t *testing.T, tc Bp2buildTestCase, registrationCtxFunc func(ctx android.RegistrationContext)) {
	t.Helper()
	(&tc).ModuleTypeUnderTest = "java_host_for_device"
	(&tc).ModuleTypeUnderTestFactory = java.HostForDeviceFactory
	RunBp2BuildTestCase(t, registrationCtxFunc, tc)
}

func runJavaHostForDeviceTestCase(t *testing.T, tc Bp2buildTestCase) {
	t.Helper()
	runJavaHostForDeviceTestCaseWithRegistrationCtxFunc(t, tc, func(ctx android.RegistrationContext) {
		ctx.RegisterModuleType("java_library", java.LibraryFactory)
	})
}

func TestJavaHostForDevice(t *testing.T) {
	runJavaHostForDeviceTestCase(t, Bp2buildTestCase{
		Description: "java_host_for_device test",
		Blueprint: `java_host_for_device {
    name: "java-lib-1",
    libs: ["java-lib-2"],
    bazel_module: { bp2build_available: true },
}

java_library {
    name: "java-lib-2",
    srcs: ["b.java"],
    bazel_module: { bp2build_available: true },
    sdk_version: "current",
}`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("java_host_for_device", "java-lib-1", AttrNameToString{
				"exports": `[":java-lib-2"]`,
			}),
			MakeNeverlinkDuplicateTargetWithAttrs("java_library", "java-lib-1", AttrNameToString{
				"sdk_version": `"none"`,
			}),
			MakeBazelTarget("java_library", "java-lib-2", AttrNameToString{
				"srcs":        `["b.java"]`,
				"sdk_version": `"current"`,
			}),
			MakeNeverlinkDuplicateTarget("java_library", "java-lib-2"),
		},
	})
}
