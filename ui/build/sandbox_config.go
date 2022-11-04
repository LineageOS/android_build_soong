// Copyright 2021 Google Inc. All rights reserved.
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

package build

type SandboxConfig struct {
	srcDirIsRO        bool
	srcDirRWAllowlist []string
}

func (sc *SandboxConfig) SetSrcDirIsRO(ro bool) {
	sc.srcDirIsRO = ro
}

func (sc *SandboxConfig) SrcDirIsRO() bool {
	return sc.srcDirIsRO
}

// Return the mount flag of the source directory in the nsjail command
func (sc *SandboxConfig) SrcDirMountFlag() string {
	ret := "-B" // Read-write
	if sc.SrcDirIsRO() {
		ret = "-R" // Read-only
	}
	return ret
}

func (sc *SandboxConfig) SetSrcDirRWAllowlist(allowlist []string) {
	sc.srcDirRWAllowlist = allowlist
}

func (sc *SandboxConfig) SrcDirRWAllowlist() []string {
	return sc.srcDirRWAllowlist
}
