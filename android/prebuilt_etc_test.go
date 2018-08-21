// Copyright 2018 Google Inc. All rights reserved.
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

package android

import (
	"io/ioutil"
	"os"
	"testing"
)

func testPrebuiltEtc(t *testing.T, bp string) *TestContext {
	config, buildDir := setUp(t)
	defer tearDown(buildDir)
	ctx := NewTestArchContext()
	ctx.RegisterModuleType("prebuilt_etc", ModuleFactoryAdaptor(PrebuiltEtcFactory))
	ctx.PreDepsMutators(func(ctx RegisterMutatorsContext) {
		ctx.BottomUp("prebuilt_etc", prebuiltEtcMutator).Parallel()
	})
	ctx.Register()
	mockFiles := map[string][]byte{
		"Android.bp": []byte(bp),
		"foo.conf":   nil,
		"bar.conf":   nil,
		"baz.conf":   nil,
	}
	ctx.MockFileSystem(mockFiles)
	_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
	FailIfErrored(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	FailIfErrored(t, errs)

	return ctx
}

func setUp(t *testing.T) (config Config, buildDir string) {
	buildDir, err := ioutil.TempDir("", "soong_prebuilt_etc_test")
	if err != nil {
		t.Fatal(err)
	}

	config = TestArchConfig(buildDir, nil)
	return
}

func tearDown(buildDir string) {
	os.RemoveAll(buildDir)
}

func TestPrebuiltEtcVariants(t *testing.T) {
	ctx := testPrebuiltEtc(t, `
		prebuilt_etc {
			name: "foo.conf",
			src: "foo.conf",
		}
		prebuilt_etc {
			name: "bar.conf",
			src: "bar.conf",
			recovery_available: true,
		}
		prebuilt_etc {
			name: "baz.conf",
			src: "baz.conf",
			recovery: true,
		}
	`)

	foo_variants := ctx.ModuleVariantsForTests("foo.conf")
	if len(foo_variants) != 1 {
		t.Errorf("expected 1, got %#v", foo_variants)
	}

	bar_variants := ctx.ModuleVariantsForTests("bar.conf")
	if len(bar_variants) != 2 {
		t.Errorf("expected 2, got %#v", bar_variants)
	}

	baz_variants := ctx.ModuleVariantsForTests("baz.conf")
	if len(baz_variants) != 1 {
		t.Errorf("expected 1, got %#v", bar_variants)
	}
}
