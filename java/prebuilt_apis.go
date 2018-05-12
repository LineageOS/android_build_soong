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
	"android/soong/android"
	"sort"
	"strconv"
	"strings"

	"github.com/google/blueprint/proptools"
)

// prebuilt_apis is a meta-module that generates filegroup modules for all
// API txt files found under the directory where the Android.bp is located.
// Specificaly, an API file located at ./<ver>/<scope>/api/<module>.txt
// generates a filegroup module named <module>-api.<scope>.<ver>.
//
// It also creates <module>-api.<scope>.latest for the lastest <ver>.
//
func init() {
	android.RegisterModuleType("prebuilt_apis", prebuiltApisFactory)

	android.PreArchMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.TopDown("prebuilt_apis", prebuiltApisMutator).Parallel()
	})
}

type prebuiltApis struct {
	android.ModuleBase
}

func (module *prebuiltApis) DepsMutator(ctx android.BottomUpMutatorContext) {
	// no need to implement
}

func (module *prebuiltApis) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	// no need to implement
}

func parseApiFilePath(ctx android.BaseModuleContext, path string) (module string, apiver int, scope string) {
	elements := strings.Split(path, "/")
	ver, err := strconv.Atoi(elements[0])
	if err != nil {
		ctx.ModuleErrorf("invalid version %q found in path: %q", elements[0], path)
		return
	}
	apiver = ver

	scope = elements[1]
	if scope != "public" && scope != "system" && scope != "test" {
		ctx.ModuleErrorf("invalid scope %q found in path: %q", scope, path)
		return
	}

	// elements[2] is string literal "api". skipping.
	module = strings.TrimSuffix(elements[3], ".txt")
	return
}

func createFilegroup(mctx android.TopDownMutatorContext, module string, scope string, apiver string, path string) {
	fgName := module + ".api." + scope + "." + apiver
	filegroupProps := struct {
		Name *string
		Srcs []string
	}{}
	filegroupProps.Name = proptools.StringPtr(fgName)
	filegroupProps.Srcs = []string{path}
	mctx.CreateModule(android.ModuleFactoryAdaptor(android.FileGroupFactory), &filegroupProps)
}

func prebuiltApisMutator(mctx android.TopDownMutatorContext) {
	if _, ok := mctx.Module().(*prebuiltApis); ok {
		mydir := mctx.ModuleDir() + "/"
		// <apiver>/<scope>/api/<module>.txt
		files, err := mctx.GlobWithDeps(mydir+"*/*/api/*.txt", nil)
		if err != nil {
			mctx.ModuleErrorf("failed to glob api txt files under %q: %s", mydir, err)
		}
		if len(files) == 0 {
			mctx.ModuleErrorf("no api file found under %q", mydir)
		}

		// construct a map to find out the latest api file path
		// for each (<module>, <scope>) pair.
		type latestApiInfo struct {
			module string
			scope  string
			apiver int
			path   string
		}
		m := make(map[string]latestApiInfo)

		for _, f := range files {
			// create a filegroup for each api txt file
			localPath := strings.TrimPrefix(f, mydir)
			module, apiver, scope := parseApiFilePath(mctx, localPath)
			createFilegroup(mctx, module, scope, strconv.Itoa(apiver), localPath)

			// find the latest apiver
			key := module + "." + scope
			info, ok := m[key]
			if !ok {
				m[key] = latestApiInfo{module, scope, apiver, localPath}
			} else if apiver > info.apiver {
				info.apiver = apiver
				info.path = localPath
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
}

func prebuiltApisFactory() android.Module {
	module := &prebuiltApis{}
	android.InitAndroidModule(module)
	return module
}
