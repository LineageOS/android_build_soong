// Copyright 2019 Google Inc. All rights reserved.
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
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"

	"android/soong/android"
)

// This singleton collects cc modules' source and flags into to a json file.
// It does so for generating CMakeLists.txt project files needed data when
// either make, mm, mma, mmm or mmma is called.
// The info file is generated in $OUT/module_bp_cc_depend.json.

func init() {
	android.RegisterParallelSingletonType("ccdeps_generator", ccDepsGeneratorSingleton)
}

func ccDepsGeneratorSingleton() android.Singleton {
	return &ccdepsGeneratorSingleton{}
}

type ccdepsGeneratorSingleton struct {
	outputPath android.Path
}

var _ android.SingletonMakeVarsProvider = (*ccdepsGeneratorSingleton)(nil)

const (
	ccdepsJsonFileName = "module_bp_cc_deps.json"
	cClang             = "clang"
	cppClang           = "clang++"
)

type ccIdeInfo struct {
	Path                 []string     `json:"path,omitempty"`
	Srcs                 []string     `json:"srcs,omitempty"`
	Global_Common_Flags  ccParameters `json:"global_common_flags,omitempty"`
	Local_Common_Flags   ccParameters `json:"local_common_flags,omitempty"`
	Global_C_flags       ccParameters `json:"global_c_flags,omitempty"`
	Local_C_flags        ccParameters `json:"local_c_flags,omitempty"`
	Global_C_only_flags  ccParameters `json:"global_c_only_flags,omitempty"`
	Local_C_only_flags   ccParameters `json:"local_c_only_flags,omitempty"`
	Global_Cpp_flags     ccParameters `json:"global_cpp_flags,omitempty"`
	Local_Cpp_flags      ccParameters `json:"local_cpp_flags,omitempty"`
	System_include_flags ccParameters `json:"system_include_flags,omitempty"`
	Module_name          string       `json:"module_name,omitempty"`
}

type ccParameters struct {
	HeaderSearchPath       []string          `json:"header_search_path,omitempty"`
	SystemHeaderSearchPath []string          `json:"system_search_path,omitempty"`
	FlagParameters         []string          `json:"flag,omitempty"`
	SysRoot                string            `json:"system_root,omitempty"`
	RelativeFilePathFlags  map[string]string `json:"relative_file_path,omitempty"`
}

type ccMapIdeInfos map[string]ccIdeInfo

type ccDeps struct {
	C_clang   string        `json:"clang,omitempty"`
	Cpp_clang string        `json:"clang++,omitempty"`
	Modules   ccMapIdeInfos `json:"modules,omitempty"`
}

func (c *ccdepsGeneratorSingleton) GenerateBuildActions(ctx android.SingletonContext) {
	// (b/204397180) Generate module_bp_cc_deps.json by default.
	moduleDeps := ccDeps{}
	moduleInfos := map[string]ccIdeInfo{}

	// Track which projects have already had CMakeLists.txt generated to keep the first
	// variant for each project.
	seenProjects := map[string]bool{}

	pathToCC, _ := evalVariable(ctx, "${config.ClangBin}/")
	moduleDeps.C_clang = fmt.Sprintf("%s%s", buildCMakePath(pathToCC), cClang)
	moduleDeps.Cpp_clang = fmt.Sprintf("%s%s", buildCMakePath(pathToCC), cppClang)

	ctx.VisitAllModules(func(module android.Module) {
		if ccModule, ok := module.(*Module); ok {
			if compiledModule, ok := ccModule.compiler.(CompiledInterface); ok {
				generateCLionProjectData(ctx, compiledModule, ccModule, seenProjects, moduleInfos)
			}
		}
	})

	moduleDeps.Modules = moduleInfos

	ccfpath := android.PathForOutput(ctx, ccdepsJsonFileName)
	err := createJsonFile(moduleDeps, ccfpath)
	if err != nil {
		ctx.Errorf(err.Error())
	}
	c.outputPath = ccfpath

	// This is necessary to satisfy the dangling rules check as this file is written by Soong rather than a rule.
	ctx.Build(pctx, android.BuildParams{
		Rule:   android.Touch,
		Output: ccfpath,
	})
}

func (c *ccdepsGeneratorSingleton) MakeVars(ctx android.MakeVarsContext) {
	if c.outputPath == nil {
		return
	}

	ctx.DistForGoal("general-tests", c.outputPath)
}

func parseCompilerCCParameters(ctx android.SingletonContext, params []string) ccParameters {
	compilerParams := ccParameters{}

	cparams := []string{}
	for _, param := range params {
		param, _ = evalVariable(ctx, param)
		cparams = append(cparams, param)
	}

	// Soong does not guarantee that each flag will be in an individual string. e.g: The
	// input received could be:
	// params = {"-isystem", "path/to/system"}
	// or it could be
	// params = {"-isystem path/to/system"}
	// To normalize the input, we split all strings with the "space" character and consolidate
	// all tokens into a flattened parameters list
	cparams = normalizeParameters(cparams)

	for i := 0; i < len(cparams); i++ {
		param := cparams[i]
		if param == "" {
			continue
		}

		switch categorizeParameter(param) {
		case headerSearchPath:
			compilerParams.HeaderSearchPath =
				append(compilerParams.HeaderSearchPath, strings.TrimPrefix(param, "-I"))
		case systemHeaderSearchPath:
			if i < len(cparams)-1 {
				compilerParams.SystemHeaderSearchPath = append(compilerParams.SystemHeaderSearchPath, cparams[i+1])
			}
			i = i + 1
		case flag:
			c := cleanupParameter(param)
			compilerParams.FlagParameters = append(compilerParams.FlagParameters, c)
		case systemRoot:
			if i < len(cparams)-1 {
				compilerParams.SysRoot = cparams[i+1]
			}
			i = i + 1
		case relativeFilePathFlag:
			flagComponents := strings.Split(param, "=")
			if len(flagComponents) == 2 {
				if compilerParams.RelativeFilePathFlags == nil {
					compilerParams.RelativeFilePathFlags = map[string]string{}
				}
				compilerParams.RelativeFilePathFlags[flagComponents[0]] = flagComponents[1]
			}
		}
	}
	return compilerParams
}

func generateCLionProjectData(ctx android.SingletonContext, compiledModule CompiledInterface,
	ccModule *Module, seenProjects map[string]bool, moduleInfos map[string]ccIdeInfo) {
	srcs := compiledModule.Srcs()
	if len(srcs) == 0 {
		return
	}

	// Only keep the DeviceArch variant module.
	if ctx.DeviceConfig().DeviceArch() != ccModule.ModuleBase.Arch().ArchType.Name {
		return
	}

	clionProjectLocation := getCMakeListsForModule(ccModule, ctx)
	if seenProjects[clionProjectLocation] {
		return
	}

	seenProjects[clionProjectLocation] = true

	name := ccModule.ModuleBase.Name()
	dpInfo := moduleInfos[name]

	dpInfo.Path = append(dpInfo.Path, path.Dir(ctx.BlueprintFile(ccModule)))
	dpInfo.Srcs = append(dpInfo.Srcs, srcs.Strings()...)
	dpInfo.Path = android.FirstUniqueStrings(dpInfo.Path)
	dpInfo.Srcs = android.FirstUniqueStrings(dpInfo.Srcs)

	dpInfo.Global_Common_Flags = parseCompilerCCParameters(ctx, ccModule.flags.Global.CommonFlags)
	dpInfo.Local_Common_Flags = parseCompilerCCParameters(ctx, ccModule.flags.Local.CommonFlags)
	dpInfo.Global_C_flags = parseCompilerCCParameters(ctx, ccModule.flags.Global.CFlags)
	dpInfo.Local_C_flags = parseCompilerCCParameters(ctx, ccModule.flags.Local.CFlags)
	dpInfo.Global_C_only_flags = parseCompilerCCParameters(ctx, ccModule.flags.Global.ConlyFlags)
	dpInfo.Local_C_only_flags = parseCompilerCCParameters(ctx, ccModule.flags.Local.ConlyFlags)
	dpInfo.Global_Cpp_flags = parseCompilerCCParameters(ctx, ccModule.flags.Global.CppFlags)
	dpInfo.Local_Cpp_flags = parseCompilerCCParameters(ctx, ccModule.flags.Local.CppFlags)
	dpInfo.System_include_flags = parseCompilerCCParameters(ctx, ccModule.flags.SystemIncludeFlags)

	dpInfo.Module_name = name

	moduleInfos[name] = dpInfo
}

type Deal struct {
	Name    string
	ideInfo ccIdeInfo
}

type Deals []Deal

// Ensure it satisfies sort.Interface
func (d Deals) Len() int           { return len(d) }
func (d Deals) Less(i, j int) bool { return d[i].Name < d[j].Name }
func (d Deals) Swap(i, j int)      { d[i], d[j] = d[j], d[i] }

func sortMap(moduleInfos map[string]ccIdeInfo) map[string]ccIdeInfo {
	var deals Deals
	for k, v := range moduleInfos {
		deals = append(deals, Deal{k, v})
	}

	sort.Sort(deals)

	m := map[string]ccIdeInfo{}
	for _, d := range deals {
		m[d.Name] = d.ideInfo
	}
	return m
}

func createJsonFile(moduleDeps ccDeps, ccfpath android.WritablePath) error {
	buf, err := json.MarshalIndent(moduleDeps, "", "\t")
	if err != nil {
		return fmt.Errorf("JSON marshal of cc deps failed: %s", err)
	}
	err = android.WriteFileToOutputDir(ccfpath, buf, 0666)
	if err != nil {
		return fmt.Errorf("Writing cc deps to %s failed: %s", ccfpath.String(), err)
	}
	return nil
}
