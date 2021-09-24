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

package bazel

import (
	"fmt"

	"github.com/google/blueprint"
)

// testModuleInfo implements blueprint.Module interface with sufficient information to mock a subset of
// a blueprint ModuleContext
type testModuleInfo struct {
	name string
	typ  string
	dir  string
}

// Name returns name for testModuleInfo -- required to implement blueprint.Module
func (mi testModuleInfo) Name() string {
	return mi.name
}

// GenerateBuildActions unused, but required to implmeent blueprint.Module
func (mi testModuleInfo) GenerateBuildActions(blueprint.ModuleContext) {}

func (mi testModuleInfo) equals(other testModuleInfo) bool {
	return mi.name == other.name && mi.typ == other.typ && mi.dir == other.dir
}

// ensure testModuleInfo implements blueprint.Module
var _ blueprint.Module = testModuleInfo{}

// otherModuleTestContext is a mock context that implements OtherModuleContext
type otherModuleTestContext struct {
	modules []testModuleInfo
	errors  []string
}

// ModuleFromName retrieves the testModuleInfo corresponding to name, if it exists
func (omc *otherModuleTestContext) ModuleFromName(name string) (blueprint.Module, bool) {
	for _, m := range omc.modules {
		if m.name == name {
			return m, true
		}
	}
	return testModuleInfo{}, false
}

// testModuleInfo returns the testModuleInfo corresponding to a blueprint.Module if it exists in omc
func (omc *otherModuleTestContext) testModuleInfo(m blueprint.Module) (testModuleInfo, bool) {
	mi, ok := m.(testModuleInfo)
	if !ok {
		return testModuleInfo{}, false
	}
	for _, other := range omc.modules {
		if other.equals(mi) {
			return mi, true
		}
	}
	return testModuleInfo{}, false
}

// OtherModuleType returns type of m if it exists in omc
func (omc *otherModuleTestContext) OtherModuleType(m blueprint.Module) string {
	if mi, ok := omc.testModuleInfo(m); ok {
		return mi.typ
	}
	return ""
}

// OtherModuleName returns name of m if it exists in omc
func (omc *otherModuleTestContext) OtherModuleName(m blueprint.Module) string {
	if mi, ok := omc.testModuleInfo(m); ok {
		return mi.name
	}
	return ""
}

// OtherModuleDir returns dir of m if it exists in omc
func (omc *otherModuleTestContext) OtherModuleDir(m blueprint.Module) string {
	if mi, ok := omc.testModuleInfo(m); ok {
		return mi.dir
	}
	return ""
}

func (omc *otherModuleTestContext) ModuleErrorf(format string, args ...interface{}) {
	omc.errors = append(omc.errors, fmt.Sprintf(format, args...))
}

// Ensure otherModuleTestContext implements OtherModuleContext
var _ OtherModuleContext = &otherModuleTestContext{}
