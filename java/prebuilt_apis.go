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
	"sort"
	"strings"

	"github.com/google/blueprint/proptools"

	"android/soong/android"
)

func init() {
	RegisterPrebuiltApisBuildComponents(android.InitRegistrationContext)
}

func RegisterPrebuiltApisBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("prebuilt_apis", PrebuiltApisFactory)
}

type prebuiltApisProperties struct {
	// list of api version directories
	Api_dirs []string
}

type prebuiltApis struct {
	android.ModuleBase
	properties prebuiltApisProperties
}

func (module *prebuiltApis) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	// no need to implement
}

func parseJarPath(path string) (module string, apiver string, scope string) {
	elements := strings.Split(path, "/")

	apiver = elements[0]
	scope = elements[1]

	module = strings.TrimSuffix(elements[2], ".jar")
	return
}

func parseApiFilePath(ctx android.LoadHookContext, path string) (module string, apiver string, scope string) {
	elements := strings.Split(path, "/")
	apiver = elements[0]

	scope = elements[1]
	if scope != "public" && scope != "system" && scope != "test" && scope != "module-lib" && scope != "system-server" {
		ctx.ModuleErrorf("invalid scope %q found in path: %q", scope, path)
		return
	}

	// elements[2] is string literal "api". skipping.
	module = strings.TrimSuffix(elements[3], ".txt")
	return
}

func prebuiltApiModuleName(mctx android.LoadHookContext, module string, scope string, apiver string) string {
	return mctx.ModuleName() + "_" + scope + "_" + apiver + "_" + module
}

func createImport(mctx android.LoadHookContext, module string, scope string, apiver string, path string) {
	props := struct {
		Name        *string
		Jars        []string
		Sdk_version *string
		Installable *bool
	}{}
	props.Name = proptools.StringPtr(prebuiltApiModuleName(mctx, module, scope, apiver))
	props.Jars = append(props.Jars, path)
	// TODO(hansson): change to scope after migration is done.
	props.Sdk_version = proptools.StringPtr("current")
	props.Installable = proptools.BoolPtr(false)

	mctx.CreateModule(ImportFactory, &props)
}

func createFilegroup(mctx android.LoadHookContext, module string, scope string, apiver string, path string) {
	fgName := module + ".api." + scope + "." + apiver
	filegroupProps := struct {
		Name *string
		Srcs []string
	}{}
	filegroupProps.Name = proptools.StringPtr(fgName)
	filegroupProps.Srcs = []string{path}
	mctx.CreateModule(android.FileGroupFactory, &filegroupProps)
}

func getPrebuiltFiles(mctx android.LoadHookContext, name string) []string {
	mydir := mctx.ModuleDir() + "/"
	var files []string
	for _, apiver := range mctx.Module().(*prebuiltApis).properties.Api_dirs {
		for _, scope := range []string{"public", "system", "test", "core", "module-lib", "system-server"} {
			vfiles, err := mctx.GlobWithDeps(mydir+apiver+"/"+scope+"/"+name, nil)
			if err != nil {
				mctx.ModuleErrorf("failed to glob %s files under %q: %s", name, mydir+apiver+"/"+scope, err)
			}
			files = append(files, vfiles...)
		}
	}
	return files
}

func prebuiltSdkStubs(mctx android.LoadHookContext) {
	mydir := mctx.ModuleDir() + "/"
	// <apiver>/<scope>/<module>.jar
	files := getPrebuiltFiles(mctx, "*.jar")

	for _, f := range files {
		// create a Import module for each jar file
		localPath := strings.TrimPrefix(f, mydir)
		module, apiver, scope := parseJarPath(localPath)
		createImport(mctx, module, scope, apiver, localPath)
	}
}

func createSystemModules(mctx android.LoadHookContext, apiver string) {
	props := struct {
		Name *string
		Libs []string
	}{}
	props.Name = proptools.StringPtr(prebuiltApiModuleName(mctx, "system_modules", "public", apiver))
	props.Libs = append(props.Libs, prebuiltApiModuleName(mctx, "core-for-system-modules", "public", apiver))

	mctx.CreateModule(SystemModulesFactory, &props)
}

func prebuiltSdkSystemModules(mctx android.LoadHookContext) {
	for _, apiver := range mctx.Module().(*prebuiltApis).properties.Api_dirs {
		jar := android.ExistentPathForSource(mctx,
			mctx.ModuleDir(), apiver, "public", "core-for-system-modules.jar")
		if jar.Valid() {
			createSystemModules(mctx, apiver)
		}
	}
}

func prebuiltApiFiles(mctx android.LoadHookContext) {
	mydir := mctx.ModuleDir() + "/"
	// <apiver>/<scope>/api/<module>.txt
	files := getPrebuiltFiles(mctx, "api/*.txt")

	if len(files) == 0 {
		mctx.ModuleErrorf("no api file found under %q", mydir)
	}

	// construct a map to find out the latest api file path
	// for each (<module>, <scope>) pair.
	type latestApiInfo struct {
		module string
		scope  string
		apiver string
		path   string
	}
	m := make(map[string]latestApiInfo)

	for _, f := range files {
		// create a filegroup for each api txt file
		localPath := strings.TrimPrefix(f, mydir)
		module, apiver, scope := parseApiFilePath(mctx, localPath)
		createFilegroup(mctx, module, scope, apiver, localPath)

		// find the latest apiver
		key := module + "." + scope
		info, ok := m[key]
		if !ok {
			m[key] = latestApiInfo{module, scope, apiver, localPath}
		} else if len(apiver) > len(info.apiver) || (len(apiver) == len(info.apiver) &&
			strings.Compare(apiver, info.apiver) > 0) {
			info.apiver = apiver
			info.path = localPath
			m[key] = info
		}
	}
	// create filegroups for the latest version of (<module>, <scope>) pairs
	// sort the keys in order to make build.ninja stable
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		info := m[k]
		createFilegroup(mctx, info.module, info.scope, "latest", info.path)
	}
}

func createPrebuiltApiModules(mctx android.LoadHookContext) {
	if _, ok := mctx.Module().(*prebuiltApis); ok {
		prebuiltApiFiles(mctx)
		prebuiltSdkStubs(mctx)
		prebuiltSdkSystemModules(mctx)
	}
}

// prebuilt_apis is a meta-module that generates filegroup modules for all
// API txt files found under the directory where the Android.bp is located.
// Specifically, an API file located at ./<ver>/<scope>/api/<module>.txt
// generates a filegroup module named <module>-api.<scope>.<ver>.
//
// It also creates <module>-api.<scope>.latest for the latest <ver>.
//
// Similarly, it generates a java_import for all API .jar files found under the
// directory where the Android.bp is located. Specifically, an API file located
// at ./<ver>/<scope>/api/<module>.jar generates a java_import module named
// <prebuilt-api-module>_<scope>_<ver>_<module>, and for SDK versions >= 30
// a java_system_modules module named
// <prebuilt-api-module>_public_<ver>_system_modules
func PrebuiltApisFactory() android.Module {
	module := &prebuiltApis{}
	module.AddProperties(&module.properties)
	android.InitAndroidModule(module)
	android.AddLoadHook(module, createPrebuiltApiModules)
	return module
}
