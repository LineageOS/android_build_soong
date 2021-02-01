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

package java

import (
	"testing"
)

func TestDexpreoptEnabled(t *testing.T) {
	tests := []struct {
		name    string
		bp      string
		enabled bool
	}{
		{
			name: "app",
			bp: `
				android_app {
					name: "foo",
					srcs: ["a.java"],
					sdk_version: "current",
				}`,
			enabled: true,
		},
		{
			name: "installable java library",
			bp: `
				java_library {
					name: "foo",
					installable: true,
					srcs: ["a.java"],
				}`,
			enabled: true,
		},
		{
			name: "java binary",
			bp: `
				java_binary {
					name: "foo",
					srcs: ["a.java"],
				}`,
			enabled: true,
		},
		{
			name: "app without sources",
			bp: `
				android_app {
					name: "foo",
					sdk_version: "current",
				}`,
			enabled: false,
		},
		{
			name: "app with libraries",
			bp: `
				android_app {
					name: "foo",
					static_libs: ["lib"],
					sdk_version: "current",
				}

				java_library {
					name: "lib",
					srcs: ["a.java"],
					sdk_version: "current",
				}`,
			enabled: true,
		},
		{
			name: "installable java library without sources",
			bp: `
				java_library {
					name: "foo",
					installable: true,
				}`,
			enabled: false,
		},
		{
			name: "static java library",
			bp: `
				java_library {
					name: "foo",
					srcs: ["a.java"],
				}`,
			enabled: false,
		},
		{
			name: "java test",
			bp: `
				java_test {
					name: "foo",
					srcs: ["a.java"],
				}`,
			enabled: false,
		},
		{
			name: "android test",
			bp: `
				android_test {
					name: "foo",
					srcs: ["a.java"],
				}`,
			enabled: false,
		},
		{
			name: "android test helper app",
			bp: `
				android_test_helper_app {
					name: "foo",
					srcs: ["a.java"],
				}`,
			enabled: false,
		},
		{
			name: "compile_dex",
			bp: `
				java_library {
					name: "foo",
					srcs: ["a.java"],
					compile_dex: true,
				}`,
			enabled: false,
		},
		{
			name: "dex_import",
			bp: `
				dex_import {
					name: "foo",
					jars: ["a.jar"],
				}`,
			enabled: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, _ := testJava(t, test.bp)

			dexpreopt := ctx.ModuleForTests("foo", "android_common").MaybeRule("dexpreopt")
			enabled := dexpreopt.Rule != nil

			if enabled != test.enabled {
				t.Fatalf("want dexpreopt %s, got %s", enabledString(test.enabled), enabledString(enabled))
			}
		})

	}
}

func enabledString(enabled bool) string {
	if enabled {
		return "enabled"
	} else {
		return "disabled"
	}
}
