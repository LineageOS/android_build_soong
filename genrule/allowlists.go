// Copyright 2023 Google Inc. All rights reserved.
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

package genrule

var (
	SandboxingDenyModuleList = []string{
		// go/keep-sorted start
		"aidl_camera_build_version",
		"com.google.pixel.camera.hal.manifest",
		// go/keep-sorted end
	}

	SandboxingDenyPathList = []string{
		// go/keep-sorted start
		// go/keep-sorted end
	}
)
