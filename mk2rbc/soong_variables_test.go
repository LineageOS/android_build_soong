// Copyright 2021 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mk2rbc

import (
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
)

type dirResolverForTest struct {
	ScopeBase
}

func (t dirResolverForTest) Get(name string) string {
	if name != "BUILD_SYSTEM" {
		return fmt.Sprintf("$(%s)", name)
	}
	return getTestDirectory()
}

func TestSoongVariables(t *testing.T) {
	testFile := filepath.Join(getTestDirectory(), "soong_variables.mk.test")
	var actual testVariables
	if err := FindSoongVariables(testFile, dirResolverForTest{}, &actual); err != nil {
		t.Fatal(err)
	}
	expected := testVariables{[]testVar{
		{"BUILD_ID", VarClassSoong, starlarkTypeString},
		{"PLATFORM_SDK_VERSION", VarClassSoong, starlarkTypeInt},
		{"DEVICE_PACKAGE_OVERLAYS", VarClassSoong, starlarkTypeList},
		{"ENABLE_CFI", VarClassSoong, starlarkTypeString},
		{"ENABLE_PREOPT", VarClassSoong, starlarkTypeString},
	}}
	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("\nExpected: %v\n  Actual: %v", expected, actual)
	}
}
