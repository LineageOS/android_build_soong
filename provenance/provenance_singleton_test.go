/*
 * Copyright (C) 2022 The Android Open Source Project
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

package provenance

import (
	"strings"
	"testing"

	"android/soong/android"
)

func TestProvenanceSingleton(t *testing.T) {
	result := android.GroupFixturePreparers(
		PrepareForTestWithProvenanceSingleton,
		android.PrepareForTestWithAndroidMk).RunTestWithBp(t, "")

	outputs := result.SingletonForTests("provenance_metadata_singleton").AllOutputs()
	for _, output := range outputs {
		testingBuildParam := result.SingletonForTests("provenance_metadata_singleton").Output(output)
		switch {
		case strings.Contains(output, "soong/provenance_metadata.textproto"):
			android.AssertStringEquals(t, "Invalid build rule", "android/soong/provenance.mergeProvenanceMetaData", testingBuildParam.Rule.String())
			android.AssertIntEquals(t, "Invalid input", len(testingBuildParam.Inputs), 0)
			android.AssertStringDoesContain(t, "Invalid output path", output, "soong/provenance_metadata.textproto")
			android.AssertIntEquals(t, "Invalid args", len(testingBuildParam.Args), 0)

		case strings.HasSuffix(output, "provenance_metadata"):
			android.AssertStringEquals(t, "Invalid build rule", "<builtin>:phony", testingBuildParam.Rule.String())
			android.AssertStringEquals(t, "Invalid input", testingBuildParam.Inputs[0].String(), "out/soong/provenance_metadata.textproto")
			android.AssertStringEquals(t, "Invalid output path", output, "provenance_metadata")
			android.AssertIntEquals(t, "Invalid args", len(testingBuildParam.Args), 0)
		}
	}
}
