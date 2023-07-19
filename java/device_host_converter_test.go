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

package java

import (
	"android/soong/android"
	"reflect"
	"strings"
	"testing"
)

func TestDeviceForHost(t *testing.T) {
	bp := `
		java_library {
			name: "device_module",
			srcs: ["a.java"],
			java_resources: ["java-res/a/a"],
		}

		java_import {
			name: "device_import_module",
			jars: ["a.jar"],
		}

		java_device_for_host {
			name: "device_for_host_module",
			libs: [
				"device_module",
				"device_import_module",
			],
		}

		java_library_host {
			name: "host_module",
			srcs: ["b.java"],
			java_resources: ["java-res/b/b"],
			static_libs: ["device_for_host_module"],
		}
	`

	ctx, config := testJava(t, bp)

	deviceModule := ctx.ModuleForTests("device_module", "android_common")
	deviceTurbineCombined := deviceModule.Output("turbine-combined/device_module.jar")
	deviceJavac := deviceModule.Output("javac/device_module.jar")
	deviceRes := deviceModule.Output("res/device_module.jar")

	deviceImportModule := ctx.ModuleForTests("device_import_module", "android_common")
	deviceImportCombined := deviceImportModule.Output("combined/device_import_module.jar")

	hostModule := ctx.ModuleForTests("host_module", config.BuildOSCommonTarget.String())
	hostJavac := hostModule.Output("javac/host_module.jar")
	hostRes := hostModule.Output("res/host_module.jar")
	combined := hostModule.Output("combined/host_module.jar")
	resCombined := hostModule.Output("res-combined/host_module.jar")

	// check classpath of host module with dependency on device_for_host_module
	expectedClasspath := "-classpath " + strings.Join(android.Paths{
		deviceTurbineCombined.Output,
		deviceImportCombined.Output,
	}.Strings(), ":")

	if hostJavac.Args["classpath"] != expectedClasspath {
		t.Errorf("expected host_module javac classpath:\n%s\ngot:\n%s",
			expectedClasspath, hostJavac.Args["classpath"])
	}

	// check host module merged with static dependency implementation jars from device_for_host module
	expectedInputs := android.Paths{
		hostJavac.Output,
		deviceJavac.Output,
		deviceImportCombined.Output,
	}

	if !reflect.DeepEqual(combined.Inputs, expectedInputs) {
		t.Errorf("expected host_module combined inputs:\n%q\ngot:\n%q",
			expectedInputs, combined.Inputs)
	}

	// check host module merged with static dependency resource jars from device_for_host module
	expectedInputs = android.Paths{
		hostRes.Output,
		deviceRes.Output,
	}

	if !reflect.DeepEqual(resCombined.Inputs, expectedInputs) {
		t.Errorf("expected host_module res combined inputs:\n%q\ngot:\n%q",
			expectedInputs, resCombined.Inputs)
	}
}

func TestHostForDevice(t *testing.T) {
	bp := `
		java_library_host {
			name: "host_module",
			srcs: ["a.java"],
			java_resources: ["java-res/a/a"],
		}

		java_import_host {
			name: "host_import_module",
			jars: ["a.jar"],
		}

		java_host_for_device {
			name: "host_for_device_module",
			libs: [
				"host_module",
				"host_import_module",
			],
		}

		java_library {
			name: "device_module",
			sdk_version: "core_platform",
			srcs: ["b.java"],
			java_resources: ["java-res/b/b"],
			static_libs: ["host_for_device_module"],
		}
	`

	ctx, config := testJava(t, bp)

	hostModule := ctx.ModuleForTests("host_module", config.BuildOSCommonTarget.String())
	hostJavac := hostModule.Output("javac/host_module.jar")
	hostJavacHeader := hostModule.Output("javac-header/host_module.jar")
	hostRes := hostModule.Output("res/host_module.jar")

	hostImportModule := ctx.ModuleForTests("host_import_module", config.BuildOSCommonTarget.String())
	hostImportCombined := hostImportModule.Output("combined/host_import_module.jar")

	deviceModule := ctx.ModuleForTests("device_module", "android_common")
	deviceJavac := deviceModule.Output("javac/device_module.jar")
	deviceRes := deviceModule.Output("res/device_module.jar")
	combined := deviceModule.Output("combined/device_module.jar")
	resCombined := deviceModule.Output("res-combined/device_module.jar")

	// check classpath of device module with dependency on host_for_device_module
	expectedClasspath := "-classpath " + strings.Join(android.Paths{
		hostJavacHeader.Output,
		hostImportCombined.Output,
	}.Strings(), ":")

	if deviceJavac.Args["classpath"] != expectedClasspath {
		t.Errorf("expected device_module javac classpath:\n%s\ngot:\n%s",
			expectedClasspath, deviceJavac.Args["classpath"])
	}

	// check device module merged with static dependency implementation jars from host_for_device module
	expectedInputs := android.Paths{
		deviceJavac.Output,
		hostJavac.Output,
		hostImportCombined.Output,
	}

	if !reflect.DeepEqual(combined.Inputs, expectedInputs) {
		t.Errorf("expected device_module combined inputs:\n%q\ngot:\n%q",
			expectedInputs, combined.Inputs)
	}

	// check device module merged with static dependency resource jars from host_for_device module
	expectedInputs = android.Paths{
		deviceRes.Output,
		hostRes.Output,
	}

	if !reflect.DeepEqual(resCombined.Inputs, expectedInputs) {
		t.Errorf("expected device_module res combined inputs:\n%q\ngot:\n%q",
			expectedInputs, resCombined.Inputs)
	}
}
