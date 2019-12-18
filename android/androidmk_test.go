// Copyright 2019 Google Inc. All rights reserved.
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
	"io"
	"reflect"
	"testing"
)

type customModule struct {
	ModuleBase
	data AndroidMkData
}

func (m *customModule) GenerateAndroidBuildActions(ctx ModuleContext) {
}

func (m *customModule) AndroidMk() AndroidMkData {
	return AndroidMkData{
		Custom: func(w io.Writer, name, prefix, moduleDir string, data AndroidMkData) {
			m.data = data
		},
	}
}

func customModuleFactory() Module {
	module := &customModule{}
	InitAndroidModule(module)
	return module
}

func TestAndroidMkSingleton_PassesUpdatedAndroidMkDataToCustomCallback(t *testing.T) {
	bp := `
	custom {
		name: "foo",
		required: ["bar"],
		host_required: ["baz"],
		target_required: ["qux"],
	}
	`

	config := TestConfig(buildDir, nil, bp, nil)
	config.inMake = true // Enable androidmk Singleton

	ctx := NewTestContext()
	ctx.RegisterSingletonType("androidmk", AndroidMkSingleton)
	ctx.RegisterModuleType("custom", customModuleFactory)
	ctx.Register(config)

	_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
	FailIfErrored(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	FailIfErrored(t, errs)

	m := ctx.ModuleForTests("foo", "").Module().(*customModule)

	assertEqual := func(expected interface{}, actual interface{}) {
		if !reflect.DeepEqual(expected, actual) {
			t.Errorf("%q expected, but got %q", expected, actual)
		}
	}
	assertEqual([]string{"bar"}, m.data.Required)
	assertEqual([]string{"baz"}, m.data.Host_required)
	assertEqual([]string{"qux"}, m.data.Target_required)
}
