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
	"io/ioutil"
	"os"
	"reflect"
	"testing"
)

type pathDepsMutatorTestModule struct {
	ModuleBase
	props struct {
		Foo string   `android:"path"`
		Bar []string `android:"path,arch_variant"`
		Baz *string  `android:"path"`
		Qux string
	}

	sourceDeps []string
}

func pathDepsMutatorTestModuleFactory() Module {
	module := &pathDepsMutatorTestModule{}
	module.AddProperties(&module.props)
	InitAndroidArchModule(module, DeviceSupported, MultilibBoth)
	return module
}

func (p *pathDepsMutatorTestModule) GenerateAndroidBuildActions(ctx ModuleContext) {
	ctx.VisitDirectDepsWithTag(SourceDepTag, func(dep Module) {
		p.sourceDeps = append(p.sourceDeps, ctx.OtherModuleName(dep))
	})
}

func TestPathDepsMutator(t *testing.T) {
	tests := []struct {
		name string
		bp   string
		deps []string
	}{
		{
			name: "all",
			bp: `
			test {
				name: "foo",
				foo: ":a",
				bar: [":b"],
				baz: ":c",
				qux: ":d",
			}`,
			deps: []string{"a", "b", "c"},
		},
		{
			name: "arch variant",
			bp: `
			test {
				name: "foo",
				arch: {
					arm64: {
						bar: [":a"],
					},
					arm: {
						bar: [":b"],
					},
				},
				bar: [":c"],
			}`,
			deps: []string{"c", "a"},
		},
	}

	buildDir, err := ioutil.TempDir("", "soong_path_properties_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(buildDir)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := TestArchConfig(buildDir, nil)
			ctx := NewTestArchContext()

			ctx.RegisterModuleType("test", ModuleFactoryAdaptor(pathDepsMutatorTestModuleFactory))
			ctx.RegisterModuleType("filegroup", ModuleFactoryAdaptor(FileGroupFactory))

			bp := test.bp + `
				filegroup {
					name: "a",
				}
				
				filegroup {
					name: "b",
				}
    	
				filegroup {
					name: "c",
				}
    	
				filegroup {
					name: "d",
				}
			`

			mockFS := map[string][]byte{
				"Android.bp": []byte(bp),
			}

			ctx.MockFileSystem(mockFS)

			ctx.Register()
			_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
			FailIfErrored(t, errs)
			_, errs = ctx.PrepareBuildActions(config)
			FailIfErrored(t, errs)

			m := ctx.ModuleForTests("foo", "android_arm64_armv8-a").Module().(*pathDepsMutatorTestModule)

			if g, w := m.sourceDeps, test.deps; !reflect.DeepEqual(g, w) {
				t.Errorf("want deps %q, got %q", w, g)
			}
		})
	}
}
