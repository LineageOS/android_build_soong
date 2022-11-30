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
	"path/filepath"
	"reflect"
	"testing"
)

func TestProductsMakefile(t *testing.T) {
	testDir := getTestDirectory()
	abspath := func(relPath string) string { return filepath.Join(testDir, relPath) }
	actualProducts := make(map[string]string)
	if err := UpdateProductConfigMap(actualProducts, abspath("android_products.mk.test")); err != nil {
		t.Fatal(err)
	}
	expectedProducts := map[string]string{
		"aosp_cf_x86_tv": abspath("vsoc_x86/tv/device.mk"),
		"aosp_tv_arm64":  abspath("aosp_tv_arm64.mk"),
	}
	if !reflect.DeepEqual(actualProducts, expectedProducts) {
		t.Errorf("\nExpected: %v\n  Actual: %v", expectedProducts, actualProducts)
	}
}
