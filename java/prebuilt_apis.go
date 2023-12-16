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
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/genrule"
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

	// Directory containing finalized api txt files for extension versions.
	// Extension versions higher than the base sdk extension version will
	// be assumed to be finalized later than all Api_dirs.
	Extensions_dir *string

	// The next API directory can optionally point to a directory where
	// files incompatibility-tracking files are stored for the current
	// "in progress" API. Each module present in one of the api_dirs will have
	// a <module>-incompatibilities.api.<scope>.latest module created.
	Next_api_dir *string

	// The sdk_version of java_import modules generated based on jar files.
	// Defaults to "current"
	Imports_sdk_version *string

	// If set to true, compile dex for java_import modules. Defaults to false.
	Imports_compile_dex *bool

	// If set to true, allow incremental platform API of the form MM.m where MM is the major release
	// version corresponding to the API level/SDK_INT and m is an incremental release version
	// (e.g. API changes associated with QPR). Defaults to false.
	Allow_incremental_platform_api *bool
}

type prebuiltApis struct {
	android.ModuleBase
	properties prebuiltApisProperties
}

func (module *prebuiltApis) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	// no need to implement
}

// parsePrebuiltPath parses the relevant variables out of a variety of paths, e.g.
// <version>/<scope>/<module>.jar
// <version>/<scope>/api/<module>.txt
// *Note when using incremental platform API, <version> may be of the form MM.m where MM is the
// API level and m is an incremental release, otherwise <version> is a single integer corresponding to the API level only.
// extensions/<version>/<scope>/<module>.jar
// extensions/<version>/<scope>/api/<module>.txt
func parsePrebuiltPath(ctx android.LoadHookContext, p string) (module string, version string, scope string) {
	elements := strings.Split(p, "/")

	scopeIdx := len(elements) - 2
	if elements[scopeIdx] == "api" {
		scopeIdx--
	}
	scope = elements[scopeIdx]
	if scope != "core" && scope != "public" && scope != "system" && scope != "test" && scope != "module-lib" && scope != "system-server" {
		ctx.ModuleErrorf("invalid scope %q found in path: %q", scope, p)
		return
	}
	version = elements[scopeIdx-1]

	module = strings.TrimSuffix(path.Base(p), path.Ext(p))
	return
}

// parseFinalizedPrebuiltPath is like parsePrebuiltPath, but verifies the version is numeric (a finalized version).
func parseFinalizedPrebuiltPath(ctx android.LoadHookContext, p string, allowIncremental bool) (module string, version int, release int, scope string) {
	module, v, scope := parsePrebuiltPath(ctx, p)
	if allowIncremental {
		parts := strings.Split(v, ".")
		if len(parts) != 2 {
			ctx.ModuleErrorf("Found unexpected version '%v' for incremental prebuilts - expect MM.m format for incremental API with both major (MM) an minor (m) revision.", v)
			return
		}
		sdk, sdk_err := strconv.Atoi(parts[0])
		qpr, qpr_err := strconv.Atoi(parts[1])
		if sdk_err != nil || qpr_err != nil {
			ctx.ModuleErrorf("Unable to read version number for incremental prebuilt api '%v'", v)
			return
		}
		version = sdk
		release = qpr
		return
	}
	release = 0
	version, err := strconv.Atoi(v)
	if err != nil {
		ctx.ModuleErrorf("Found finalized API files in non-numeric dir '%v'", v)
		return
	}
	return
}

func prebuiltApiModuleName(mctx android.LoadHookContext, module, scope, version string) string {
	return fmt.Sprintf("%s_%s_%s_%s", mctx.ModuleName(), scope, version, module)
}
func createImport(mctx android.LoadHookContext, module, scope, version, path, sdkVersion string, compileDex bool) {
	props := struct {
		Name        *string
		Jars        []string
		Sdk_version *string
		Installable *bool
		Compile_dex *bool
	}{
		Name:        proptools.StringPtr(prebuiltApiModuleName(mctx, module, scope, version)),
		Jars:        []string{path},
		Sdk_version: proptools.StringPtr(sdkVersion),
		Installable: proptools.BoolPtr(false),
		Compile_dex: proptools.BoolPtr(compileDex),
	}
	mctx.CreateModule(ImportFactory, &props)
}

func createApiModule(mctx android.LoadHookContext, name string, path string) {
	genruleProps := struct {
		Name *string
		Srcs []string
		Out  []string
		Cmd  *string
	}{}
	genruleProps.Name = proptools.StringPtr(name)
	genruleProps.Srcs = []string{path}
	genruleProps.Out = []string{name}
	genruleProps.Cmd = proptools.StringPtr("cp $(in) $(out)")
	mctx.CreateModule(genrule.GenRuleFactory, &genruleProps)
}

func createLatestApiModuleExtensionVersionFile(mctx android.LoadHookContext, name string, version string) {
	genruleProps := struct {
		Name *string
		Srcs []string
		Out  []string
		Cmd  *string
	}{}
	genruleProps.Name = proptools.StringPtr(name)
	genruleProps.Out = []string{name}
	genruleProps.Cmd = proptools.StringPtr("echo " + version + " > $(out)")
	mctx.CreateModule(genrule.GenRuleFactory, &genruleProps)
}

func createEmptyFile(mctx android.LoadHookContext, name string) {
	props := struct {
		Name *string
		Cmd  *string
		Out  []string
	}{}
	props.Name = proptools.StringPtr(name)
	props.Out = []string{name}
	props.Cmd = proptools.StringPtr("touch $(genDir)/" + name)
	mctx.CreateModule(genrule.GenRuleFactory, &props)
}

// globApiDirs collects all the files in all api_dirs and all scopes that match the given glob, e.g. '*.jar' or 'api/*.txt'.
// <api-dir>/<scope>/<glob> for all api-dir and scope.
func globApiDirs(mctx android.LoadHookContext, p *prebuiltApis, api_dir_glob string) []string {
	var files []string
	for _, apiver := range p.properties.Api_dirs {
		files = append(files, globScopeDir(mctx, apiver, api_dir_glob)...)
	}
	return files
}

// globExtensionDirs collects all the files under the extension dir (for all versions and scopes) that match the given glob
// <extension-dir>/<version>/<scope>/<glob> for all version and scope.
func globExtensionDirs(mctx android.LoadHookContext, p *prebuiltApis, extension_dir_glob string) []string {
	// <extensions-dir>/<num>/<extension-dir-glob>
	return globScopeDir(mctx, *p.properties.Extensions_dir+"/*", extension_dir_glob)
}

// globScopeDir collects all the files in the given subdir across all scopes that match the given glob, e.g. '*.jar' or 'api/*.txt'.
// <subdir>/<scope>/<glob> for all scope.
func globScopeDir(mctx android.LoadHookContext, subdir string, subdir_glob string) []string {
	var files []string
	dir := mctx.ModuleDir() + "/" + subdir
	for _, scope := range []string{"public", "system", "test", "core", "module-lib", "system-server"} {
		glob := fmt.Sprintf("%s/%s/%s", dir, scope, subdir_glob)
		vfiles, err := mctx.GlobWithDeps(glob, nil)
		if err != nil {
			mctx.ModuleErrorf("failed to glob %s files under %q: %s", subdir_glob, dir+"/"+scope, err)
		}
		files = append(files, vfiles...)
	}
	for i, f := range files {
		files[i] = strings.TrimPrefix(f, mctx.ModuleDir()+"/")
	}
	return files
}

func prebuiltSdkStubs(mctx android.LoadHookContext, p *prebuiltApis) {
	// <apiver>/<scope>/<module>.jar
	files := globApiDirs(mctx, p, "*.jar")

	sdkVersion := proptools.StringDefault(p.properties.Imports_sdk_version, "current")
	compileDex := proptools.BoolDefault(p.properties.Imports_compile_dex, false)

	for _, f := range files {
		// create a Import module for each jar file
		module, version, scope := parsePrebuiltPath(mctx, f)
		createImport(mctx, module, scope, version, f, sdkVersion, compileDex)

		if module == "core-for-system-modules" {
			createSystemModules(mctx, version, scope)
		}
	}
}

func createSystemModules(mctx android.LoadHookContext, version, scope string) {
	props := struct {
		Name *string
		Libs []string
	}{}
	props.Name = proptools.StringPtr(prebuiltApiModuleName(mctx, "system_modules", scope, version))
	props.Libs = append(props.Libs, prebuiltApiModuleName(mctx, "core-for-system-modules", scope, version))

	mctx.CreateModule(systemModulesImportFactory, &props)
}

func PrebuiltApiModuleName(module, scope, version string) string {
	return module + ".api." + scope + "." + version
}

func prebuiltApiFiles(mctx android.LoadHookContext, p *prebuiltApis) {
	// <apiver>/<scope>/api/<module>.txt
	apiLevelFiles := globApiDirs(mctx, p, "api/*.txt")
	if len(apiLevelFiles) == 0 {
		mctx.ModuleErrorf("no api file found under %q", mctx.ModuleDir())
	}

	// Create modules for all (<module>, <scope, <version>) triplets,
	allowIncremental := proptools.BoolDefault(p.properties.Allow_incremental_platform_api, false)
	for _, f := range apiLevelFiles {
		module, version, release, scope := parseFinalizedPrebuiltPath(mctx, f, allowIncremental)
		if allowIncremental {
			incrementalVersion := strconv.Itoa(version) + "." + strconv.Itoa(release)
			createApiModule(mctx, PrebuiltApiModuleName(module, scope, incrementalVersion), f)
		} else {
			createApiModule(mctx, PrebuiltApiModuleName(module, scope, strconv.Itoa(version)), f)
		}
	}

	// Figure out the latest version of each module/scope
	type latestApiInfo struct {
		module, scope, path string
		version, release    int
		isExtensionApiFile  bool
	}

	getLatest := func(files []string, isExtensionApiFile bool) map[string]latestApiInfo {
		m := make(map[string]latestApiInfo)
		for _, f := range files {
			module, version, release, scope := parseFinalizedPrebuiltPath(mctx, f, allowIncremental)
			if strings.HasSuffix(module, "incompatibilities") {
				continue
			}
			key := module + "." + scope
			info, exists := m[key]
			if !exists || version > info.version || (version == info.version && release > info.release) {
				m[key] = latestApiInfo{module, scope, f, version, release, isExtensionApiFile}
			}
		}
		return m
	}

	latest := getLatest(apiLevelFiles, false)
	if p.properties.Extensions_dir != nil {
		extensionApiFiles := globExtensionDirs(mctx, p, "api/*.txt")
		for k, v := range getLatest(extensionApiFiles, true) {
			if _, exists := latest[k]; !exists {
				mctx.ModuleErrorf("Module %v finalized for extension %d but never during an API level; likely error", v.module, v.version)
			}
			// The extension version is always at least as new as the last sdk int version (potentially identical)
			latest[k] = v
		}
	}

	// Sort the keys in order to make build.ninja stable
	for _, k := range android.SortedKeys(latest) {
		info := latest[k]
		name := PrebuiltApiModuleName(info.module, info.scope, "latest")
		latestExtensionVersionModuleName := PrebuiltApiModuleName(info.module, info.scope, "latest.extension_version")
		if info.isExtensionApiFile {
			createLatestApiModuleExtensionVersionFile(mctx, latestExtensionVersionModuleName, strconv.Itoa(info.version))
		} else {
			createLatestApiModuleExtensionVersionFile(mctx, latestExtensionVersionModuleName, "-1")
		}
		createApiModule(mctx, name, info.path)
	}

	// Create incompatibilities tracking files for all modules, if we have a "next" api.
	incompatibilities := make(map[string]bool)
	if nextApiDir := String(p.properties.Next_api_dir); nextApiDir != "" {
		files := globScopeDir(mctx, nextApiDir, "api/*incompatibilities.txt")
		for _, f := range files {
			filename, _, scope := parsePrebuiltPath(mctx, f)
			referencedModule := strings.TrimSuffix(filename, "-incompatibilities")

			createApiModule(mctx, PrebuiltApiModuleName(referencedModule+"-incompatibilities", scope, "latest"), f)

			incompatibilities[referencedModule+"."+scope] = true
		}
	}
	// Create empty incompatibilities files for remaining modules
	for _, k := range android.SortedKeys(latest) {
		if _, ok := incompatibilities[k]; !ok {
			createEmptyFile(mctx, PrebuiltApiModuleName(latest[k].module+"-incompatibilities", latest[k].scope, "latest"))
		}
	}
}

func createPrebuiltApiModules(mctx android.LoadHookContext) {
	if p, ok := mctx.Module().(*prebuiltApis); ok {
		prebuiltApiFiles(mctx, p)
		prebuiltSdkStubs(mctx, p)
	}
}

// prebuilt_apis is a meta-module that generates modules for all API txt files
// found under the directory where the Android.bp is located.
// Specifically, an API file located at ./<ver>/<scope>/api/<module>.txt
// generates a module named <module>-api.<scope>.<ver>.
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
