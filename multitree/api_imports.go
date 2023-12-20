// Copyright 2022 Google Inc. All rights reserved.
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

package multitree

import (
	"android/soong/android"
	"strings"

	"github.com/google/blueprint"
)

var (
	apiImportNameSuffix = ".apiimport"
)

func init() {
	RegisterApiImportsModule(android.InitRegistrationContext)
	android.RegisterMakeVarsProvider(pctx, makeVarsProvider)
}

func RegisterApiImportsModule(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("api_imports", apiImportsFactory)
}

type ApiImports struct {
	android.ModuleBase
	properties apiImportsProperties
}

type apiImportsProperties struct {
	Shared_libs      []string // List of C shared libraries from API surfaces
	Header_libs      []string // List of C header libraries from API surfaces
	Apex_shared_libs []string // List of C shared libraries with APEX stubs
}

// 'api_imports' is a module which describes modules available from API surfaces.
// This module is required to get the list of all imported API modules, because
// it is discouraged to loop and fetch all modules from its type information. The
// only module with name 'api_imports' will be used from the build.
func apiImportsFactory() android.Module {
	module := &ApiImports{}
	module.AddProperties(&module.properties)
	android.InitAndroidModule(module)
	return module
}

func (imports *ApiImports) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	// ApiImport module does not generate any build actions
}

type ApiImportInfo struct {
	SharedLibs, HeaderLibs, ApexSharedLibs map[string]string
}

var ApiImportsProvider = blueprint.NewMutatorProvider[ApiImportInfo]("deps")

// Store module lists into ApiImportInfo and share it over mutator provider.
func (imports *ApiImports) DepsMutator(ctx android.BottomUpMutatorContext) {
	generateNameMapWithSuffix := func(names []string) map[string]string {
		moduleNameMap := make(map[string]string)
		for _, name := range names {
			moduleNameMap[name] = name + apiImportNameSuffix
		}

		return moduleNameMap
	}

	sharedLibs := generateNameMapWithSuffix(imports.properties.Shared_libs)
	headerLibs := generateNameMapWithSuffix(imports.properties.Header_libs)
	apexSharedLibs := generateNameMapWithSuffix(imports.properties.Apex_shared_libs)

	android.SetProvider(ctx, ApiImportsProvider, ApiImportInfo{
		SharedLibs:     sharedLibs,
		HeaderLibs:     headerLibs,
		ApexSharedLibs: apexSharedLibs,
	})
}

func GetApiImportSuffix() string {
	return apiImportNameSuffix
}

func makeVarsProvider(ctx android.MakeVarsContext) {
	ctx.VisitAllModules(func(m android.Module) {
		if i, ok := m.(*ApiImports); ok {
			ctx.Strict("API_IMPORTED_SHARED_LIBRARIES", strings.Join(i.properties.Shared_libs, " "))
			ctx.Strict("API_IMPORTED_HEADER_LIBRARIES", strings.Join(i.properties.Header_libs, " "))
		}
	})
}
