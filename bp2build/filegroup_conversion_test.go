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
	"fmt"

	"testing"
)

func runFilegroupTestCase(t *testing.T, tc bp2buildTestCase) {
	t.Helper()
	(&tc).moduleTypeUnderTest = "filegroup"
	(&tc).moduleTypeUnderTestFactory = android.FileGroupFactory
	runBp2BuildTestCase(t, registerFilegroupModuleTypes, tc)
}

func registerFilegroupModuleTypes(ctx android.RegistrationContext) {}

func TestFilegroupSameNameAsFile_OneFile(t *testing.T) {
	runFilegroupTestCase(t, bp2buildTestCase{
		description: "filegroup - same name as file, with one file",
		filesystem:  map[string]string{},
		blueprint: `
filegroup {
    name: "foo",
    srcs: ["foo"],
}
`,
		expectedBazelTargets: []string{}})
}

func TestFilegroupSameNameAsFile_MultipleFiles(t *testing.T) {
	runFilegroupTestCase(t, bp2buildTestCase{
		description: "filegroup - same name as file, with multiple files",
		filesystem:  map[string]string{},
		blueprint: `
filegroup {
	name: "foo",
	srcs: ["foo", "bar"],
}
`,
		expectedErr: fmt.Errorf("filegroup 'foo' cannot contain a file with the same name"),
	})
}
