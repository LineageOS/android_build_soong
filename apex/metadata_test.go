/*
 * Copyright (C) 2023 The Android Open Source Project
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package apex

import (
	"strings"
	"testing"

	"android/soong/android"
	"android/soong/java"
)

func TestModulesSingleton(t *testing.T) {
	result := android.GroupFixturePreparers(
		PrepareForTestWithApexMultitreeSingleton,
		java.PrepareForTestWithJavaDefaultModules,
		PrepareForTestWithApexBuildComponents,
		java.FixtureConfigureApexBootJars("myapex:foo"),
		java.PrepareForTestWithJavaSdkLibraryFiles,
	).RunTestWithBp(t, `
		prebuilt_apex {
			name: "myapex",
			src: "myapex.apex",
			exported_bootclasspath_fragments: ["mybootclasspath-fragment"],
		}

		// A prebuilt java_sdk_library_import that is not preferred by default but will be preferred
		// because AlwaysUsePrebuiltSdks() is true.
		java_sdk_library_import {
			name: "foo",
			prefer: false,
			shared_library: false,
			permitted_packages: ["foo"],
			public: {
				jars: ["sdk_library/public/foo-stubs.jar"],
				stub_srcs: ["sdk_library/public/foo_stub_sources"],
				current_api: "sdk_library/public/foo.txt",
				removed_api: "sdk_library/public/foo-removed.txt",
				sdk_version: "current",
			},
			apex_available: ["myapex"],
		}

		prebuilt_bootclasspath_fragment {
			name: "mybootclasspath-fragment",
			apex_available: [
				"myapex",
			],
			contents: [
				"foo",
			],
			hidden_api: {
				stub_flags: "prebuilt-stub-flags.csv",
				annotation_flags: "prebuilt-annotation-flags.csv",
				metadata: "prebuilt-metadata.csv",
				index: "prebuilt-index.csv",
				all_flags: "prebuilt-all-flags.csv",
			},
		}

		platform_bootclasspath {
			name: "myplatform-bootclasspath",
			fragments: [
				{
					apex: "myapex",
					module:"mybootclasspath-fragment",
				},
			],
		}
`,
	)

	outputs := result.SingletonForTests("apex_multitree_singleton").AllOutputs()
	for _, output := range outputs {
		testingBuildParam := result.SingletonForTests("apex_multitree_singleton").Output(output)
		switch {
		case strings.Contains(output, "soong/multitree_apex_metadata.json"):
			android.AssertStringEquals(t, "Invalid build rule", "android/soong/android.writeFile", testingBuildParam.Rule.String())
			android.AssertIntEquals(t, "Invalid input", len(testingBuildParam.Inputs), 0)
			android.AssertStringDoesContain(t, "Invalid output path", output, "soong/multitree_apex_metadata.json")

		case strings.HasSuffix(output, "multitree_apex_metadata"):
			android.AssertStringEquals(t, "Invalid build rule", "<builtin>:phony", testingBuildParam.Rule.String())
			android.AssertStringEquals(t, "Invalid input", testingBuildParam.Inputs[0].String(), "out/soong/multitree_apex_metadata.json")
			android.AssertStringEquals(t, "Invalid output path", output, "multitree_apex_metadata")
			android.AssertIntEquals(t, "Invalid args", len(testingBuildParam.Args), 0)
		}
	}
}
