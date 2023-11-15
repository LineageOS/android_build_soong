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
	"encoding/json"
	"fmt"

	"android/soong/android"
)

// This singleton generates android java dependency into to a json file. It does so for each
// blueprint Android.bp resulting in a java.Module when either make, mm, mma, mmm or mmma is
// called. Dependency info file is generated in $OUT/module_bp_java_depend.json.

func init() {
	android.RegisterParallelSingletonType("jdeps_generator", jDepsGeneratorSingleton)
}

func jDepsGeneratorSingleton() android.Singleton {
	return &jdepsGeneratorSingleton{}
}

type jdepsGeneratorSingleton struct {
	outputPath android.Path
}

var _ android.SingletonMakeVarsProvider = (*jdepsGeneratorSingleton)(nil)

const (
	jdepsJsonFileName = "module_bp_java_deps.json"
)

func (j *jdepsGeneratorSingleton) GenerateBuildActions(ctx android.SingletonContext) {
	// (b/204397180) Generate module_bp_java_deps.json by default.
	moduleInfos := make(map[string]android.IdeInfo)

	ctx.VisitAllModules(func(module android.Module) {
		if !module.Enabled() {
			return
		}

		// Prevent including both prebuilts and matching source modules when one replaces the other.
		if !android.IsModulePreferred(module) {
			return
		}

		ideInfoProvider, ok := module.(android.IDEInfo)
		if !ok {
			return
		}
		name := ideInfoProvider.BaseModuleName()
		ideModuleNameProvider, ok := module.(android.IDECustomizedModuleName)
		if ok {
			name = ideModuleNameProvider.IDECustomizedModuleName()
		}

		dpInfo := moduleInfos[name]
		ideInfoProvider.IDEInfo(&dpInfo)
		dpInfo.Deps = android.FirstUniqueStrings(dpInfo.Deps)
		dpInfo.Srcs = android.FirstUniqueStrings(dpInfo.Srcs)
		dpInfo.Aidl_include_dirs = android.FirstUniqueStrings(dpInfo.Aidl_include_dirs)
		dpInfo.Jarjar_rules = android.FirstUniqueStrings(dpInfo.Jarjar_rules)
		dpInfo.Jars = android.FirstUniqueStrings(dpInfo.Jars)
		dpInfo.SrcJars = android.FirstUniqueStrings(dpInfo.SrcJars)
		dpInfo.Paths = []string{ctx.ModuleDir(module)}
		dpInfo.Static_libs = android.FirstUniqueStrings(dpInfo.Static_libs)
		dpInfo.Libs = android.FirstUniqueStrings(dpInfo.Libs)
		moduleInfos[name] = dpInfo

		mkProvider, ok := module.(android.AndroidMkDataProvider)
		if !ok {
			return
		}
		data := mkProvider.AndroidMk()
		if data.Class != "" {
			dpInfo.Classes = append(dpInfo.Classes, data.Class)
		}

		if ctx.ModuleHasProvider(module, JavaInfoProvider) {
			dep := ctx.ModuleProvider(module, JavaInfoProvider).(JavaInfo)
			dpInfo.Installed_paths = append(dpInfo.Installed_paths, dep.ImplementationJars.Strings()...)
		}
		dpInfo.Classes = android.FirstUniqueStrings(dpInfo.Classes)
		dpInfo.Installed_paths = android.FirstUniqueStrings(dpInfo.Installed_paths)
		moduleInfos[name] = dpInfo
	})

	jfpath := android.PathForOutput(ctx, jdepsJsonFileName)
	err := createJsonFile(moduleInfos, jfpath)
	if err != nil {
		ctx.Errorf(err.Error())
	}
	j.outputPath = jfpath

	// This is necessary to satisfy the dangling rules check as this file is written by Soong rather than a rule.
	ctx.Build(pctx, android.BuildParams{
		Rule:   android.Touch,
		Output: jfpath,
	})
}

func (j *jdepsGeneratorSingleton) MakeVars(ctx android.MakeVarsContext) {
	if j.outputPath == nil {
		return
	}

	ctx.DistForGoal("general-tests", j.outputPath)
}

func createJsonFile(moduleInfos map[string]android.IdeInfo, jfpath android.WritablePath) error {
	buf, err := json.MarshalIndent(moduleInfos, "", "\t")
	if err != nil {
		return fmt.Errorf("JSON marshal of java deps failed: %s", err)
	}
	err = android.WriteFileToOutputDir(jfpath, buf, 0666)
	if err != nil {
		return fmt.Errorf("Writing java deps to %s failed: %s", jfpath.String(), err)
	}
	return nil
}
