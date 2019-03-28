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
	"os"

	"android/soong/android"
)

// This singleton generates android java dependency into to a json file. It does so for each
// blueprint Android.bp resulting in a java.Module when either make, mm, mma, mmm or mmma is
// called. Dependency info file is generated in $OUT/module_bp_java_depend.json.

func init() {
	android.RegisterSingletonType("jdeps_generator", jDepsGeneratorSingleton)
}

func jDepsGeneratorSingleton() android.Singleton {
	return &jdepsGeneratorSingleton{}
}

type jdepsGeneratorSingleton struct {
}

const (
	// Environment variables used to modify behavior of this singleton.
	envVariableCollectJavaDeps = "SOONG_COLLECT_JAVA_DEPS"
	jdepsJsonFileName          = "module_bp_java_deps.json"
)

func (j *jdepsGeneratorSingleton) GenerateBuildActions(ctx android.SingletonContext) {
	if !ctx.Config().IsEnvTrue(envVariableCollectJavaDeps) {
		return
	}

	moduleInfos := make(map[string]android.IdeInfo)

	ctx.VisitAllModules(func(module android.Module) {
		if !module.Enabled() {
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
		moduleInfos[name] = dpInfo

		mkProvider, ok := module.(android.AndroidMkDataProvider)
		if !ok {
			return
		}
		data := mkProvider.AndroidMk()
		if data.Class != "" {
			dpInfo.Classes = append(dpInfo.Classes, data.Class)
		}

		if dep, ok := module.(Dependency); ok {
			dpInfo.Installed_paths = append(dpInfo.Installed_paths, dep.ImplementationJars().Strings()...)
		}
		dpInfo.Classes = android.FirstUniqueStrings(dpInfo.Classes)
		dpInfo.Installed_paths = android.FirstUniqueStrings(dpInfo.Installed_paths)
		moduleInfos[name] = dpInfo
	})

	jfpath := android.PathForOutput(ctx, jdepsJsonFileName).String()
	err := createJsonFile(moduleInfos, jfpath)
	if err != nil {
		ctx.Errorf(err.Error())
	}
}

func createJsonFile(moduleInfos map[string]android.IdeInfo, jfpath string) error {
	file, err := os.Create(jfpath)
	if err != nil {
		return fmt.Errorf("Failed to create file: %s, relative: %v", jdepsJsonFileName, err)
	}
	defer file.Close()
	buf, err := json.MarshalIndent(moduleInfos, "", "\t")
	if err != nil {
		return fmt.Errorf("Write file failed: %s, relative: %v", jdepsJsonFileName, err)
	}
	fmt.Fprintf(file, string(buf))
	return nil
}
