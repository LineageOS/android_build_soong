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

package sdk

import (
	"testing"
)

// Needed in an _test.go file in this package to ensure tests run correctly, particularly in IDE.
func TestMain(m *testing.M) {
	runTestWithBuildDir(m)
}

func TestDepNotInRequiredSdks(t *testing.T) {
	testSdkError(t, `module "myjavalib".*depends on "otherlib".*that isn't part of the required SDKs:.*`, `
		sdk {
			name: "mysdk",
			java_header_libs: ["sdkmember"],
		}

		sdk_snapshot {
			name: "mysdk@1",
			java_header_libs: ["sdkmember_mysdk_1"],
		}

		java_import {
			name: "sdkmember",
			prefer: false,
			host_supported: true,
		}

		java_import {
			name: "sdkmember_mysdk_1",
			sdk_member_name: "sdkmember",
			host_supported: true,
		}

		java_library {
			name: "myjavalib",
			srcs: ["Test.java"],
			libs: [
				"sdkmember",
				"otherlib",
			],
			system_modules: "none",
			sdk_version: "none",
			compile_dex: true,
			host_supported: true,
		}

		// this lib is no in mysdk
		java_library {
			name: "otherlib",
			srcs: ["Test.java"],
			system_modules: "none",
			sdk_version: "none",
			compile_dex: true,
			host_supported: true,
		}

		apex {
			name: "myapex",
			java_libs: ["myjavalib"],
			uses_sdks: ["mysdk@1"],
			key: "myapex.key",
			certificate: ":myapex.cert",
		}
	`)
}
