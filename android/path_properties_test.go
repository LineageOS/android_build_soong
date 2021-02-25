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

	// A second property struct with a duplicate property name
	props2 struct {
		Foo string `android:"path"`
	}

	// nested slices of struct
	props3 struct {
		X []struct {
			Y []struct {
				Z []string `android:"path"`
			}
		}
	}

	sourceDeps []string
}

func pathDepsMutatorTestModuleFactory() Module {
	module := &pathDepsMutatorTestModule{}
	module.AddProperties(&module.props, &module.props2, &module.props3)
	InitAndroidArchModule(module, DeviceSupported, MultilibBoth)
	return module
}

func (p *pathDepsMutatorTestModule) GenerateAndroidBuildActions(ctx ModuleContext) {
	ctx.VisitDirectDeps(func(dep Module) {
		if _, ok := ctx.OtherModuleDependencyTag(dep).(sourceOrOutputDependencyTag); ok {
			p.sourceDeps = append(p.sourceDeps, ctx.OtherModuleName(dep))
		}
	})

	if p.props.Foo != "" {
		// Make sure there is only one dependency on a module listed in a property present in multiple property structs
		if ctx.GetDirectDepWithTag(SrcIsModule(p.props.Foo), sourceOrOutputDepTag("")) == nil {
			ctx.ModuleErrorf("GetDirectDepWithTag failed")
		}
	}
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
				baz: ":c{.bar}",
				qux: ":d",
				x: [
					{
						y: [
							{
								z: [":x", ":y"],
							},
							{
								z: [":z"],
							},
						],
					},
				],
			}`,
			deps: []string{"a", "b", "c", "x", "y", "z"},
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

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
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

				filegroup {
					name: "x",
				}

				filegroup {
					name: "y",
				}

				filegroup {
					name: "z",
				}
			`

			config := TestArchConfig(buildDir, nil, bp, nil)
			ctx := NewTestArchContext(config)

			ctx.RegisterModuleType("test", pathDepsMutatorTestModuleFactory)
			ctx.RegisterModuleType("filegroup", FileGroupFactory)

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
