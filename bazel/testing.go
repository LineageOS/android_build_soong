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

// TestModuleInfo implements blueprint.Module interface with sufficient information to mock a subset of
// a blueprint ModuleContext
type TestModuleInfo struct {
	ModuleName string
	Typ        string
	Dir        string
}

// Name returns name for testModuleInfo -- required to implement blueprint.Module
func (mi TestModuleInfo) Name() string {
	return mi.ModuleName
}

// GenerateBuildActions unused, but required to implmeent blueprint.Module
func (mi TestModuleInfo) GenerateBuildActions(blueprint.ModuleContext) {}

func (mi TestModuleInfo) equals(other TestModuleInfo) bool {
	return mi.ModuleName == other.ModuleName && mi.Typ == other.Typ && mi.Dir == other.Dir
}

// ensure testModuleInfo implements blueprint.Module
var _ blueprint.Module = TestModuleInfo{}

// OtherModuleTestContext is a mock context that implements OtherModuleContext
type OtherModuleTestContext struct {
	Modules []TestModuleInfo
	errors  []string
}

// ModuleFromName retrieves the testModuleInfo corresponding to name, if it exists
func (omc *OtherModuleTestContext) ModuleFromName(name string) (blueprint.Module, bool) {
	for _, m := range omc.Modules {
		if m.ModuleName == name {
			return m, true
		}
	}
	return TestModuleInfo{}, false
}

// testModuleInfo returns the testModuleInfo corresponding to a blueprint.Module if it exists in omc
func (omc *OtherModuleTestContext) testModuleInfo(m blueprint.Module) (TestModuleInfo, bool) {
	mi, ok := m.(TestModuleInfo)
	if !ok {
		return TestModuleInfo{}, false
	}
	for _, other := range omc.Modules {
		if other.equals(mi) {
			return mi, true
		}
	}
	return TestModuleInfo{}, false
}

// OtherModuleType returns type of m if it exists in omc
func (omc *OtherModuleTestContext) OtherModuleType(m blueprint.Module) string {
	if mi, ok := omc.testModuleInfo(m); ok {
		return mi.Typ
	}
	return ""
}

// OtherModuleName returns name of m if it exists in omc
func (omc *OtherModuleTestContext) OtherModuleName(m blueprint.Module) string {
	if mi, ok := omc.testModuleInfo(m); ok {
		return mi.ModuleName
	}
	return ""
}

// OtherModuleDir returns dir of m if it exists in omc
func (omc *OtherModuleTestContext) OtherModuleDir(m blueprint.Module) string {
	if mi, ok := omc.testModuleInfo(m); ok {
		return mi.Dir
	}
	return ""
}

func (omc *OtherModuleTestContext) ModuleErrorf(format string, args ...interface{}) {
	omc.errors = append(omc.errors, fmt.Sprintf(format, args...))
}

// Ensure otherModuleTestContext implements OtherModuleContext
var _ OtherModuleContext = &OtherModuleTestContext{}
