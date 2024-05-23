// Copyright 2024 Google Inc. All rights reserved.
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

package apex

import (
	"testing"

	"android/soong/aconfig/codegen"
	"android/soong/android"
	"android/soong/cc"
	"android/soong/genrule"
	"android/soong/java"
	"android/soong/rust"

	"github.com/google/blueprint/proptools"
)

var withAconfigValidationError = android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
	variables.AconfigContainerValidation = "error"
	variables.BuildId = proptools.StringPtr("TEST.BUILD_ID")
})

func TestValidationAcrossContainersExportedPass(t *testing.T) {
	testCases := []struct {
		name string
		bp   string
	}{
		{
			name: "Java lib passes for exported containers cross",
			bp: apex_default_bp + `
				apex {
					name: "myapex",
					manifest: ":myapex.manifest",
					androidManifest: ":myapex.androidmanifest",
					key: "myapex.key",
					java_libs: [
						"my_java_library_foo",
					],
					updatable: false,
				}
				java_library {
					name: "my_java_library_foo",
					srcs: ["foo/bar/MyClass.java"],
					sdk_version: "none",
					system_modules: "none",
					static_libs: ["my_java_aconfig_library_foo"],
					apex_available: [
						"myapex",
					],
				}
				aconfig_declarations {
					name: "my_aconfig_declarations_foo",
					package: "com.example.package",
					container: "otherapex",
					srcs: ["foo.aconfig"],
					exportable: true,
				}
				java_aconfig_library {
					name: "my_java_aconfig_library_foo",
					aconfig_declarations: "my_aconfig_declarations_foo",
					mode: "exported",
					apex_available: [
						"myapex",
					],
				}`,
		},
		{
			name: "Android app passes for exported containers cross",
			bp: apex_default_bp + `
				apex {
					name: "myapex",
					manifest: ":myapex.manifest",
					androidManifest: ":myapex.androidmanifest",
					key: "myapex.key",
					apps: [
						"my_android_app_foo",
					],
					updatable: false,
				}
				android_app {
					name: "my_android_app_foo",
					srcs: ["foo/MyClass.java"],
					sdk_version: "none",
					system_modules: "none",
					stl: "none",
					static_libs: ["my_java_library_bar"],
					apex_available: [ "myapex" ],
				}
				java_library {
					name: "my_java_library_bar",
					srcs: ["foo/bar/MyClass.java"],
					sdk_version: "none",
					system_modules: "none",
					static_libs: ["my_java_aconfig_library_bar"],
					apex_available: [
						"myapex",
					],
				}
				aconfig_declarations {
					name: "my_aconfig_declarations_bar",
					package: "com.example.package",
					container: "otherapex",
					srcs: ["bar.aconfig"],
					exportable: true,
				}
				java_aconfig_library {
					name: "my_java_aconfig_library_bar",
					aconfig_declarations: "my_aconfig_declarations_bar",
					mode: "exported",
					apex_available: [
						"myapex",
					],
				}`,
		},
		{
			name: "Cc lib passes for exported containers cross",
			bp: apex_default_bp + `
				apex {
					name: "myapex",
					manifest: ":myapex.manifest",
					androidManifest: ":myapex.androidmanifest",
					key: "myapex.key",
					native_shared_libs: [
						"my_cc_library_bar",
					],
					binaries: [
						"my_cc_binary_baz",
					],
					updatable: false,
				}
				cc_library {
					name: "my_cc_library_bar",
					srcs: ["foo/bar/MyClass.cc"],
					static_libs: [
						"my_cc_aconfig_library_bar",
						"my_cc_aconfig_library_baz",
					],
					apex_available: [
						"myapex",
					],
				}
				cc_binary {
					name: "my_cc_binary_baz",
					srcs: ["foo/bar/MyClass.cc"],
					static_libs: ["my_cc_aconfig_library_baz"],
					apex_available: [
						"myapex",
					],
				}
				cc_library {
					name: "server_configurable_flags",
					srcs: ["server_configurable_flags.cc"],
				}
				cc_library {
					name: "libbase",
					srcs: ["libbase.cc"],
			                apex_available: [
				            "myapex",
			                ],
				}
				cc_library {
					name: "libaconfig_storage_read_api_cc",
					srcs: ["libaconfig_storage_read_api_cc.cc"],
				}
				aconfig_declarations {
					name: "my_aconfig_declarations_bar",
					package: "com.example.package",
					container: "otherapex",
					srcs: ["bar.aconfig"],
					exportable: true,
				}
				cc_aconfig_library {
					name: "my_cc_aconfig_library_bar",
					aconfig_declarations: "my_aconfig_declarations_bar",
					apex_available: [
						"myapex",
					],
					mode: "exported",
				}
				aconfig_declarations {
					name: "my_aconfig_declarations_baz",
					package: "com.example.package",
					container: "otherapex",
					srcs: ["baz.aconfig"],
					exportable: true,
				}
				cc_aconfig_library {
					name: "my_cc_aconfig_library_baz",
					aconfig_declarations: "my_aconfig_declarations_baz",
					apex_available: [
						"myapex",
					],
					mode: "exported",
				}`,
		},
		{
			name: "Rust lib passes for exported containers cross",
			bp: apex_default_bp + `
			apex {
				name: "myapex",
				manifest: ":myapex.manifest",
				androidManifest: ":myapex.androidmanifest",
				key: "myapex.key",
				native_shared_libs: ["libmy_rust_library"],
				binaries: ["my_rust_binary"],
				updatable: false,
			}
			rust_library {
				name: "libflags_rust", // test mock
				crate_name: "flags_rust",
				srcs: ["lib.rs"],
				apex_available: ["myapex"],
			}
			rust_library {
				name: "liblazy_static", // test mock
				crate_name: "lazy_static",
				srcs: ["src/lib.rs"],
				apex_available: ["myapex"],
			}
			rust_library {
				name: "libaconfig_storage_read_api", // test mock
				crate_name: "aconfig_storage_read_api",
				srcs: ["src/lib.rs"],
				apex_available: ["myapex"],
			}
			rust_library {
				name: "liblogger", // test mock
				crate_name: "logger",
				srcs: ["src/lib.rs"],
				apex_available: ["myapex"],
			}
			rust_library {
				name: "liblog_rust", // test mock
				crate_name: "log_rust",
				srcs: ["src/lib.rs"],
				apex_available: ["myapex"],
                        }
			rust_ffi_shared {
				name: "libmy_rust_library",
				srcs: ["src/lib.rs"],
				rustlibs: ["libmy_rust_aconfig_library_foo"],
				crate_name: "my_rust_library",
				apex_available: ["myapex"],
			}
			rust_binary {
				name: "my_rust_binary",
				srcs: ["foo/bar/MyClass.rs"],
				rustlibs: ["libmy_rust_aconfig_library_bar"],
				apex_available: ["myapex"],
			}
			aconfig_declarations {
				name: "my_aconfig_declarations_foo",
				package: "com.example.package",
				container: "otherapex",
				srcs: ["foo.aconfig"],
			}
			aconfig_declarations {
				name: "my_aconfig_declarations_bar",
				package: "com.example.package",
				container: "otherapex",
				srcs: ["bar.aconfig"],
			}
			rust_aconfig_library {
				name: "libmy_rust_aconfig_library_foo",
				aconfig_declarations: "my_aconfig_declarations_foo",
				crate_name: "my_rust_aconfig_library_foo",
				apex_available: ["myapex"],
				mode: "exported",
			}
			rust_aconfig_library {
				name: "libmy_rust_aconfig_library_bar",
				aconfig_declarations: "my_aconfig_declarations_bar",
				crate_name: "my_rust_aconfig_library_bar",
				apex_available: ["myapex"],
				mode: "exported",
			}`,
		},
	}
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			android.GroupFixturePreparers(
				java.PrepareForTestWithJavaDefaultModules,
				cc.PrepareForTestWithCcBuildComponents,
				rust.PrepareForTestWithRustDefaultModules,
				codegen.PrepareForTestWithAconfigBuildComponents,
				PrepareForTestWithApexBuildComponents,
				prepareForTestWithMyapex,
				withAconfigValidationError,
			).
				RunTestWithBp(t, test.bp)
		})
	}
}

func TestValidationAcrossContainersNotExportedFail(t *testing.T) {
	testCases := []struct {
		name          string
		expectedError string
		bp            string
	}{
		{
			name: "Java lib fails for non-exported containers cross",
			bp: apex_default_bp + `
				apex {
					name: "myapex",
					manifest: ":myapex.manifest",
					androidManifest: ":myapex.androidmanifest",
					key: "myapex.key",
					java_libs: [
						"my_java_library_foo",
					],
					updatable: false,
				}
				java_library {
					name: "my_java_library_foo",
					srcs: ["foo/bar/MyClass.java"],
					sdk_version: "none",
					system_modules: "none",
					static_libs: ["my_java_aconfig_library_foo"],
					apex_available: [
						"myapex",
					],
				}
				aconfig_declarations {
					name: "my_aconfig_declarations_foo",
					package: "com.example.package",
					container: "otherapex",
					srcs: ["foo.aconfig"],
				}
				java_aconfig_library {
					name: "my_java_aconfig_library_foo",
					aconfig_declarations: "my_aconfig_declarations_foo",
					apex_available: [
						"myapex",
					],
				}`,
			expectedError: `.*my_java_library_foo/myapex depends on my_java_aconfig_library_foo/otherapex/production across containers`,
		},
		{
			name: "Android app fails for non-exported containers cross",
			bp: apex_default_bp + `
				apex {
					name: "myapex",
					manifest: ":myapex.manifest",
					androidManifest: ":myapex.androidmanifest",
					key: "myapex.key",
					apps: [
						"my_android_app_foo",
					],
					updatable: false,
				}
				android_app {
					name: "my_android_app_foo",
					srcs: ["foo/MyClass.java"],
					sdk_version: "none",
					system_modules: "none",
					stl: "none",
					static_libs: ["my_java_library_foo"],
					apex_available: [ "myapex" ],
				}
				java_library {
					name: "my_java_library_foo",
					srcs: ["foo/bar/MyClass.java"],
					sdk_version: "none",
					system_modules: "none",
					static_libs: ["my_java_aconfig_library_foo"],
					apex_available: [
						"myapex",
					],
				}
				aconfig_declarations {
					name: "my_aconfig_declarations_foo",
					package: "com.example.package",
					container: "otherapex",
					srcs: ["bar.aconfig"],
				}
				java_aconfig_library {
					name: "my_java_aconfig_library_foo",
					aconfig_declarations: "my_aconfig_declarations_foo",
					apex_available: [
						"myapex",
					],
				}`,
			expectedError: `.*my_android_app_foo/myapex depends on my_java_aconfig_library_foo/otherapex/production across containers`,
		},
		{
			name: "Cc lib fails for non-exported containers cross",
			bp: apex_default_bp + `
				apex {
					name: "myapex",
					manifest: ":myapex.manifest",
					androidManifest: ":myapex.androidmanifest",
					key: "myapex.key",
					native_shared_libs: [
						"my_cc_library_foo",
					],
					updatable: false,
				}
				cc_library {
					name: "my_cc_library_foo",
					srcs: ["foo/bar/MyClass.cc"],
					shared_libs: [
						"my_cc_aconfig_library_foo",
					],
					apex_available: [
						"myapex",
					],
				}
				cc_library {
					name: "server_configurable_flags",
					srcs: ["server_configurable_flags.cc"],
				}
				cc_library {
					name: "libbase",
					srcs: ["libbase.cc"],
			                apex_available: [
				            "myapex",
			                ],
				}
				cc_library {
					name: "libaconfig_storage_read_api_cc",
					srcs: ["libaconfig_storage_read_api_cc.cc"],
				}
				aconfig_declarations {
					name: "my_aconfig_declarations_foo",
					package: "com.example.package",
					container: "otherapex",
					srcs: ["foo.aconfig"],
				}
				cc_aconfig_library {
					name: "my_cc_aconfig_library_foo",
					aconfig_declarations: "my_aconfig_declarations_foo",
					apex_available: [
						"myapex",
					],
				}`,
			expectedError: `.*my_cc_library_foo/myapex depends on my_cc_aconfig_library_foo/otherapex/production across containers`,
		},
		{
			name: "Cc binary fails for non-exported containers cross",
			bp: apex_default_bp + `
				apex {
					name: "myapex",
					manifest: ":myapex.manifest",
					androidManifest: ":myapex.androidmanifest",
					key: "myapex.key",
					binaries: [
						"my_cc_binary_foo",
					],
					updatable: false,
				}
				cc_library {
					name: "my_cc_library_foo",
					srcs: ["foo/bar/MyClass.cc"],
					static_libs: [
						"my_cc_aconfig_library_foo",
					],
					apex_available: [
						"myapex",
					],
				}
				cc_binary {
					name: "my_cc_binary_foo",
					srcs: ["foo/bar/MyClass.cc"],
					static_libs: ["my_cc_library_foo"],
					apex_available: [
						"myapex",
					],
				}
				cc_library {
					name: "server_configurable_flags",
					srcs: ["server_configurable_flags.cc"],
				}
				cc_library {
					name: "libbase",
					srcs: ["libbase.cc"],
			                apex_available: [
				            "myapex",
			                ],
				}
				cc_library {
					name: "libaconfig_storage_read_api_cc",
					srcs: ["libaconfig_storage_read_api_cc.cc"],
				}
				aconfig_declarations {
					name: "my_aconfig_declarations_foo",
					package: "com.example.package",
					container: "otherapex",
					srcs: ["foo.aconfig"],
				}
				cc_aconfig_library {
					name: "my_cc_aconfig_library_foo",
					aconfig_declarations: "my_aconfig_declarations_foo",
					apex_available: [
						"myapex",
					],
				}`,
			expectedError: `.*my_cc_binary_foo/myapex depends on my_cc_aconfig_library_foo/otherapex/production across containers`,
		},
		{
			name: "Rust lib fails for non-exported containers cross",
			bp: apex_default_bp + `
			apex {
				name: "myapex",
				manifest: ":myapex.manifest",
				androidManifest: ":myapex.androidmanifest",
				key: "myapex.key",
				native_shared_libs: ["libmy_rust_library"],
				updatable: false,
			}
			rust_library {
				name: "libflags_rust", // test mock
				crate_name: "flags_rust",
				srcs: ["lib.rs"],
				apex_available: ["myapex"],
			}
			rust_library {
				name: "liblazy_static", // test mock
				crate_name: "lazy_static",
				srcs: ["src/lib.rs"],
				apex_available: ["myapex"],
			}
			rust_library {
				name: "libaconfig_storage_read_api", // test mock
				crate_name: "aconfig_storage_read_api",
				srcs: ["src/lib.rs"],
				apex_available: ["myapex"],
			}
			rust_library {
				name: "liblogger", // test mock
				crate_name: "logger",
				srcs: ["src/lib.rs"],
				apex_available: ["myapex"],
			}
			rust_library {
				name: "liblog_rust", // test mock
				crate_name: "log_rust",
				srcs: ["src/lib.rs"],
				apex_available: ["myapex"],
			}
			rust_ffi_shared {
				name: "libmy_rust_library",
				srcs: ["src/lib.rs"],
				rustlibs: ["libmy_rust_aconfig_library_foo"],
				crate_name: "my_rust_library",
				apex_available: ["myapex"],
			}
			aconfig_declarations {
				name: "my_aconfig_declarations_foo",
				package: "com.example.package",
				container: "otherapex",
				srcs: ["foo.aconfig"],
			}
			rust_aconfig_library {
				name: "libmy_rust_aconfig_library_foo",
				aconfig_declarations: "my_aconfig_declarations_foo",
				crate_name: "my_rust_aconfig_library_foo",
				apex_available: ["myapex"],
			}`,
			expectedError: `.*libmy_rust_aconfig_library_foo/myapex depends on libmy_rust_aconfig_library_foo/otherapex/production across containers`,
		},
		{
			name: "Rust binary fails for non-exported containers cross",
			bp: apex_default_bp + `
			apex {
				name: "myapex",
				manifest: ":myapex.manifest",
				androidManifest: ":myapex.androidmanifest",
				key: "myapex.key",
				binaries: ["my_rust_binary"],
				updatable: false,
			}
			rust_library {
				name: "libflags_rust", // test mock
				crate_name: "flags_rust",
				srcs: ["lib.rs"],
				apex_available: ["myapex"],
			}
			rust_library {
				name: "liblazy_static", // test mock
				crate_name: "lazy_static",
				srcs: ["src/lib.rs"],
				apex_available: ["myapex"],
			}
			rust_library {
				name: "libaconfig_storage_read_api", // test mock
				crate_name: "aconfig_storage_read_api",
				srcs: ["src/lib.rs"],
				apex_available: ["myapex"],
			}
			rust_library {
				name: "liblogger", // test mock
				crate_name: "logger",
				srcs: ["src/lib.rs"],
				apex_available: ["myapex"],
			}
			rust_library {
				name: "liblog_rust", // test mock
				crate_name: "log_rust",
				srcs: ["src/lib.rs"],
				apex_available: ["myapex"],
			}
			rust_binary {
				name: "my_rust_binary",
				srcs: ["foo/bar/MyClass.rs"],
				rustlibs: ["libmy_rust_aconfig_library_bar"],
				apex_available: ["myapex"],
			}
			aconfig_declarations {
				name: "my_aconfig_declarations_bar",
				package: "com.example.package",
				container: "otherapex",
				srcs: ["bar.aconfig"],
			}
			rust_aconfig_library {
				name: "libmy_rust_aconfig_library_bar",
				aconfig_declarations: "my_aconfig_declarations_bar",
				crate_name: "my_rust_aconfig_library_bar",
				apex_available: ["myapex"],
			}`,
			expectedError: `.*libmy_rust_aconfig_library_bar/myapex depends on libmy_rust_aconfig_library_bar/otherapex/production across containers`,
		},
		{
			name: "Aconfig validation propagate along sourceOrOutputDependencyTag",
			bp: apex_default_bp + `
				apex {
					name: "myapex",
					manifest: ":myapex.manifest",
					androidManifest: ":myapex.androidmanifest",
					key: "myapex.key",
					apps: [
						"my_android_app_foo",
					],
					updatable: false,
				}
				android_app {
					name: "my_android_app_foo",
					srcs: ["foo/MyClass.java"],
					sdk_version: "none",
					system_modules: "none",
					stl: "none",
					static_libs: ["my_java_library_foo"],
					apex_available: [ "myapex" ],
				}
				java_library {
					name: "my_java_library_foo",
					srcs: [":my_genrule_foo"],
					sdk_version: "none",
					system_modules: "none",
					apex_available: [
						"myapex",
					],
				}
				aconfig_declarations_group {
						name: "my_aconfig_declarations_group_foo",
						java_aconfig_libraries: [
								"my_java_aconfig_library_foo",
						],
				}
				filegroup {
						name: "my_filegroup_foo_srcjars",
						srcs: [
								":my_aconfig_declarations_group_foo{.srcjars}",
						],
				}
				genrule {
						name: "my_genrule_foo",
						srcs: [":my_filegroup_foo_srcjars"],
						cmd: "cp $(in) $(out)",
						out: ["my_genrule_foo.srcjar"],
				}
				aconfig_declarations {
					name: "my_aconfig_declarations_foo",
					package: "com.example.package",
					container: "otherapex",
					srcs: ["bar.aconfig"],
				}
				java_aconfig_library {
					name: "my_java_aconfig_library_foo",
					aconfig_declarations: "my_aconfig_declarations_foo",
					apex_available: [
						"myapex",
					],
				}`,
			expectedError: `.*my_android_app_foo/myapex depends on my_java_aconfig_library_foo/otherapex/production across containers`,
		},
	}
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			errorHandler := android.FixtureExpectsNoErrors
			if test.expectedError != "" {
				errorHandler = android.FixtureExpectsAtLeastOneErrorMatchingPattern(test.expectedError)
			}
			android.GroupFixturePreparers(
				java.PrepareForTestWithJavaDefaultModules,
				cc.PrepareForTestWithCcBuildComponents,
				rust.PrepareForTestWithRustDefaultModules,
				codegen.PrepareForTestWithAconfigBuildComponents,
				genrule.PrepareForIntegrationTestWithGenrule,
				PrepareForTestWithApexBuildComponents,
				prepareForTestWithMyapex,
				withAconfigValidationError,
			).
				ExtendWithErrorHandler(errorHandler).
				RunTestWithBp(t, test.bp)
		})
	}
}

func TestValidationNotPropagateAcrossShared(t *testing.T) {
	testCases := []struct {
		name string
		bp   string
	}{
		{
			name: "Java shared lib not propagate aconfig validation",
			bp: apex_default_bp + `
				apex {
					name: "myapex",
					manifest: ":myapex.manifest",
					androidManifest: ":myapex.androidmanifest",
					key: "myapex.key",
					java_libs: [
						"my_java_library_bar",
					],
					updatable: false,
				}
				java_library {
					name: "my_java_library_bar",
					srcs: ["foo/bar/MyClass.java"],
					sdk_version: "none",
					system_modules: "none",
					libs: ["my_java_library_foo"],
					apex_available: [
						"myapex",
					],
				}
				java_library {
					name: "my_java_library_foo",
					srcs: ["foo/bar/MyClass.java"],
					sdk_version: "none",
					system_modules: "none",
					static_libs: ["my_java_aconfig_library_foo"],
					apex_available: [
						"myapex",
					],
				}
				aconfig_declarations {
					name: "my_aconfig_declarations_foo",
					package: "com.example.package",
					container: "otherapex",
					srcs: ["foo.aconfig"],
				}
				java_aconfig_library {
					name: "my_java_aconfig_library_foo",
					aconfig_declarations: "my_aconfig_declarations_foo",
					apex_available: [
						"myapex",
					],
				}`,
		},
	}
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			android.GroupFixturePreparers(
				java.PrepareForTestWithJavaDefaultModules,
				cc.PrepareForTestWithCcBuildComponents,
				rust.PrepareForTestWithRustDefaultModules,
				codegen.PrepareForTestWithAconfigBuildComponents,
				PrepareForTestWithApexBuildComponents,
				prepareForTestWithMyapex,
				withAconfigValidationError,
			).
				RunTestWithBp(t, test.bp)
		})
	}
}
