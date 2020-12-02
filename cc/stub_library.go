// Copyright 2020 Google Inc. All rights reserved.
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

package cc

import (
	"strings"

	"android/soong/android"
)

func init() {
	// Use singleton type to gather all generated soong modules.
	android.RegisterSingletonType("stublibraries", stubLibrariesSingleton)
}

type stubLibraries struct {
	stubLibraryMap map[string]bool
}

// Check if the module defines stub, or itself is stub
func isStubTarget(m *Module) bool {
	if m.IsStubs() || m.HasStubsVariants() {
		return true
	}

	// Library which defines LLNDK Stub is also Stub target.
	// Pure LLNDK Stub target would not contain any packaging
	// with target file path.
	if library, ok := m.linker.(*libraryDecorator); ok {
		if library.Properties.Llndk_stubs != nil {
			return true
		}
	}

	return false
}

// Get target file name to be installed from this module
func getInstalledFileName(m *Module) string {
	for _, ps := range m.PackagingSpecs() {
		if name := ps.FileName(); name != "" {
			return name
		}
	}
	return ""
}

func (s *stubLibraries) GenerateBuildActions(ctx android.SingletonContext) {
	// Visit all generated soong modules and store stub library file names.
	ctx.VisitAllModules(func(module android.Module) {
		if m, ok := module.(*Module); ok {
			if isStubTarget(m) {
				if name := getInstalledFileName(m); name != "" {
					s.stubLibraryMap[name] = true
				}
			}
		}
	})
}

func stubLibrariesSingleton() android.Singleton {
	return &stubLibraries{
		stubLibraryMap: make(map[string]bool),
	}
}

func (s *stubLibraries) MakeVars(ctx android.MakeVarsContext) {
	// Convert stub library file names into Makefile variable.
	ctx.Strict("STUB_LIBRARIES", strings.Join(android.SortedStringKeys(s.stubLibraryMap), " "))
}
