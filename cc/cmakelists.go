// Copyright 2017 Google Inc. All rights reserved.
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
	"fmt"

	"android/soong/android"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// This singleton generates CMakeLists.txt files. It does so for each blueprint Android.bp resulting in a cc.Module
// when either make, mm, mma, mmm or mmma is called. CMakeLists.txt files are generated in a separate folder
// structure (see variable CLionOutputProjectsDirectory for root).

func init() {
	android.RegisterParallelSingletonType("cmakelists_generator", cMakeListsGeneratorSingleton)
}

func cMakeListsGeneratorSingleton() android.Singleton {
	return &cmakelistsGeneratorSingleton{}
}

type cmakelistsGeneratorSingleton struct{}

const (
	cMakeListsFilename              = "CMakeLists.txt"
	cLionAggregateProjectsDirectory = "development" + string(os.PathSeparator) + "ide" + string(os.PathSeparator) + "clion"
	cLionOutputProjectsDirectory    = "out" + string(os.PathSeparator) + cLionAggregateProjectsDirectory
	minimumCMakeVersionSupported    = "3.5"

	// Environment variables used to modify behavior of this singleton.
	envVariableGenerateCMakeLists = "SOONG_GEN_CMAKEFILES"
	envVariableGenerateDebugInfo  = "SOONG_GEN_CMAKEFILES_DEBUG"
	envVariableTrue               = "1"
)

// Instruct generator to trace how header include path and flags were generated.
// This is done to ease investigating bug reports.
var outputDebugInfo = false

func (c *cmakelistsGeneratorSingleton) GenerateBuildActions(ctx android.SingletonContext) {
	if getEnvVariable(envVariableGenerateCMakeLists, ctx) != envVariableTrue {
		return
	}

	outputDebugInfo = (getEnvVariable(envVariableGenerateDebugInfo, ctx) == envVariableTrue)

	// Track which projects have already had CMakeLists.txt generated to keep the first
	// variant for each project.
	seenProjects := map[string]bool{}

	ctx.VisitAllModules(func(module android.Module) {
		if ccModule, ok := module.(*Module); ok {
			if compiledModule, ok := ccModule.compiler.(CompiledInterface); ok {
				generateCLionProject(compiledModule, ctx, ccModule, seenProjects)
			}
		}
	})

	// Link all handmade CMakeLists.txt aggregate from
	//     BASE/development/ide/clion to
	// BASE/out/development/ide/clion.
	dir := filepath.Join(android.AbsSrcDirForExistingUseCases(), cLionAggregateProjectsDirectory)
	filepath.Walk(dir, linkAggregateCMakeListsFiles)

	return
}

func getEnvVariable(name string, ctx android.SingletonContext) string {
	// Using android.Config.Getenv instead of os.getEnv to guarantee soong will
	// re-run in case this environment variable changes.
	return ctx.Config().Getenv(name)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	return true
}

func linkAggregateCMakeListsFiles(path string, info os.FileInfo, err error) error {

	if info == nil {
		return nil
	}

	dst := strings.Replace(path, cLionAggregateProjectsDirectory, cLionOutputProjectsDirectory, 1)
	if info.IsDir() {
		// This is a directory to create
		os.MkdirAll(dst, os.ModePerm)
	} else {
		// This is a file to link
		os.Remove(dst)
		os.Symlink(path, dst)
	}
	return nil
}

func generateCLionProject(compiledModule CompiledInterface, ctx android.SingletonContext, ccModule *Module,
	seenProjects map[string]bool) {
	srcs := compiledModule.Srcs()
	if len(srcs) == 0 {
		return
	}

	// Only write CMakeLists.txt for the first variant of each architecture of each module
	clionprojectLocation := getCMakeListsForModule(ccModule, ctx)
	if seenProjects[clionprojectLocation] {
		return
	}

	seenProjects[clionprojectLocation] = true

	// Ensure the directory hosting the cmakelists.txt exists
	projectDir := path.Dir(clionprojectLocation)
	os.MkdirAll(projectDir, os.ModePerm)

	// Create cmakelists.txt
	f, _ := os.Create(filepath.Join(projectDir, cMakeListsFilename))
	defer f.Close()

	// Header.
	f.WriteString("# THIS FILE WAS AUTOMATICALY GENERATED!\n")
	f.WriteString("# ANY MODIFICATION WILL BE OVERWRITTEN!\n\n")
	f.WriteString("# To improve project view in Clion    :\n")
	f.WriteString("# Tools > CMake > Change Project Root  \n\n")
	f.WriteString(fmt.Sprintf("cmake_minimum_required(VERSION %s)\n", minimumCMakeVersionSupported))
	f.WriteString(fmt.Sprintf("project(%s)\n", ccModule.ModuleBase.Name()))
	f.WriteString(fmt.Sprintf("set(ANDROID_ROOT %s)\n\n", android.AbsSrcDirForExistingUseCases()))

	pathToCC, _ := evalVariable(ctx, "${config.ClangBin}/")
	f.WriteString(fmt.Sprintf("set(CMAKE_C_COMPILER \"%s%s\")\n", buildCMakePath(pathToCC), "clang"))
	f.WriteString(fmt.Sprintf("set(CMAKE_CXX_COMPILER \"%s%s\")\n", buildCMakePath(pathToCC), "clang++"))

	// Add all sources to the project.
	f.WriteString("list(APPEND\n")
	f.WriteString("     SOURCE_FILES\n")
	for _, src := range srcs {
		f.WriteString(fmt.Sprintf("    ${ANDROID_ROOT}/%s\n", src.String()))
	}
	f.WriteString(")\n")

	// Add all header search path and compiler parameters (-D, -W, -f, -XXXX)
	f.WriteString("\n# GLOBAL ALL FLAGS:\n")
	globalAllParameters := parseCompilerParameters(ccModule.flags.Global.CommonFlags, ctx, f)
	translateToCMake(globalAllParameters, f, true, true)

	f.WriteString("\n# LOCAL ALL FLAGS:\n")
	localAllParameters := parseCompilerParameters(ccModule.flags.Local.CommonFlags, ctx, f)
	translateToCMake(localAllParameters, f, true, true)

	f.WriteString("\n# GLOBAL CFLAGS:\n")
	globalCParameters := parseCompilerParameters(ccModule.flags.Global.CFlags, ctx, f)
	translateToCMake(globalCParameters, f, true, true)

	f.WriteString("\n# LOCAL CFLAGS:\n")
	localCParameters := parseCompilerParameters(ccModule.flags.Local.CFlags, ctx, f)
	translateToCMake(localCParameters, f, true, true)

	f.WriteString("\n# GLOBAL C ONLY FLAGS:\n")
	globalConlyParameters := parseCompilerParameters(ccModule.flags.Global.ConlyFlags, ctx, f)
	translateToCMake(globalConlyParameters, f, true, false)

	f.WriteString("\n# LOCAL C ONLY FLAGS:\n")
	localConlyParameters := parseCompilerParameters(ccModule.flags.Local.ConlyFlags, ctx, f)
	translateToCMake(localConlyParameters, f, true, false)

	f.WriteString("\n# GLOBAL CPP FLAGS:\n")
	globalCppParameters := parseCompilerParameters(ccModule.flags.Global.CppFlags, ctx, f)
	translateToCMake(globalCppParameters, f, false, true)

	f.WriteString("\n# LOCAL CPP FLAGS:\n")
	localCppParameters := parseCompilerParameters(ccModule.flags.Local.CppFlags, ctx, f)
	translateToCMake(localCppParameters, f, false, true)

	f.WriteString("\n# GLOBAL SYSTEM INCLUDE FLAGS:\n")
	globalIncludeParameters := parseCompilerParameters(ccModule.flags.SystemIncludeFlags, ctx, f)
	translateToCMake(globalIncludeParameters, f, true, true)

	// Add project executable.
	f.WriteString(fmt.Sprintf("\nadd_executable(%s ${SOURCE_FILES})\n",
		cleanExecutableName(ccModule.ModuleBase.Name())))
}

func cleanExecutableName(s string) string {
	return strings.Replace(s, "@", "-", -1)
}

func translateToCMake(c compilerParameters, f *os.File, cflags bool, cppflags bool) {
	writeAllIncludeDirectories(c.systemHeaderSearchPath, f, true)
	writeAllIncludeDirectories(c.headerSearchPath, f, false)
	if cflags {
		writeAllRelativeFilePathFlags(c.relativeFilePathFlags, f, "CMAKE_C_FLAGS")
		writeAllFlags(c.flags, f, "CMAKE_C_FLAGS")
	}
	if cppflags {
		writeAllRelativeFilePathFlags(c.relativeFilePathFlags, f, "CMAKE_CXX_FLAGS")
		writeAllFlags(c.flags, f, "CMAKE_CXX_FLAGS")
	}
	if c.sysroot != "" {
		f.WriteString(fmt.Sprintf("include_directories(SYSTEM \"%s\")\n", buildCMakePath(path.Join(c.sysroot, "usr", "include"))))
	}

}

func buildCMakePath(p string) string {
	if path.IsAbs(p) {
		return p
	}
	return fmt.Sprintf("${ANDROID_ROOT}/%s", p)
}

func writeAllIncludeDirectories(includes []string, f *os.File, isSystem bool) {
	if len(includes) == 0 {
		return
	}

	system := ""
	if isSystem {
		system = "SYSTEM"
	}

	f.WriteString(fmt.Sprintf("include_directories(%s \n", system))

	for _, include := range includes {
		f.WriteString(fmt.Sprintf("    \"%s\"\n", buildCMakePath(include)))
	}
	f.WriteString(")\n\n")

	// Also add all headers to source files.
	f.WriteString("file (GLOB_RECURSE TMP_HEADERS\n")
	for _, include := range includes {
		f.WriteString(fmt.Sprintf("    \"%s/**/*.h\"\n", buildCMakePath(include)))
	}
	f.WriteString(")\n")
	f.WriteString("list (APPEND SOURCE_FILES ${TMP_HEADERS})\n\n")
}

type relativeFilePathFlagType struct {
	flag             string
	relativeFilePath string
}

func writeAllRelativeFilePathFlags(relativeFilePathFlags []relativeFilePathFlagType, f *os.File, tag string) {
	for _, flag := range relativeFilePathFlags {
		f.WriteString(fmt.Sprintf("set(%s \"${%s} %s=%s\")\n", tag, tag, flag.flag, buildCMakePath(flag.relativeFilePath)))
	}
}

func writeAllFlags(flags []string, f *os.File, tag string) {
	for _, flag := range flags {
		f.WriteString(fmt.Sprintf("set(%s \"${%s} %s\")\n", tag, tag, flag))
	}
}

type parameterType int

const (
	headerSearchPath parameterType = iota
	variable
	systemHeaderSearchPath
	flag
	systemRoot
	relativeFilePathFlag
)

type compilerParameters struct {
	headerSearchPath       []string
	systemHeaderSearchPath []string
	flags                  []string
	sysroot                string
	// Must be in a=b/c/d format and can be split into "a" and "b/c/d"
	relativeFilePathFlags []relativeFilePathFlagType
}

func makeCompilerParameters() compilerParameters {
	return compilerParameters{
		sysroot: "",
	}
}

func categorizeParameter(parameter string) parameterType {
	if strings.HasPrefix(parameter, "-I") {
		return headerSearchPath
	}
	if strings.HasPrefix(parameter, "$") {
		return variable
	}
	if strings.HasPrefix(parameter, "-isystem") {
		return systemHeaderSearchPath
	}
	if strings.HasPrefix(parameter, "-isysroot") {
		return systemRoot
	}
	if strings.HasPrefix(parameter, "--sysroot") {
		return systemRoot
	}
	if strings.HasPrefix(parameter, "-fsanitize-ignorelist") {
		return relativeFilePathFlag
	}
	if strings.HasPrefix(parameter, "-fprofile-sample-use") {
		return relativeFilePathFlag
	}
	return flag
}

// Flattens a list of strings potentially containing space characters into a list of string containing no
// spaces.
func normalizeParameters(params []string) []string {
	var flatParams []string
	for _, s := range params {
		s = strings.Trim(s, " ")
		if len(s) == 0 {
			continue
		}
		flatParams = append(flatParams, strings.Split(s, " ")...)
	}
	return flatParams
}

func parseCompilerParameters(params []string, ctx android.SingletonContext, f *os.File) compilerParameters {
	var compilerParameters = makeCompilerParameters()

	for i, str := range params {
		f.WriteString(fmt.Sprintf("# Raw param [%d] = '%s'\n", i, str))
	}

	// Soong does not guarantee that each flag will be in an individual string. e.g: The
	// input received could be:
	// params = {"-isystem", "path/to/system"}
	// or it could be
	// params = {"-isystem path/to/system"}
	// To normalize the input, we split all strings with the "space" character and consolidate
	// all tokens into a flattened parameters list
	params = normalizeParameters(params)

	for i := 0; i < len(params); i++ {
		param := params[i]
		if param == "" {
			continue
		}

		switch categorizeParameter(param) {
		case headerSearchPath:
			compilerParameters.headerSearchPath =
				append(compilerParameters.headerSearchPath, strings.TrimPrefix(param, "-I"))
		case variable:
			if evaluated, error := evalVariable(ctx, param); error == nil {
				if outputDebugInfo {
					f.WriteString(fmt.Sprintf("# variable %s = '%s'\n", param, evaluated))
				}

				paramsFromVar := parseCompilerParameters(strings.Split(evaluated, " "), ctx, f)
				concatenateParams(&compilerParameters, paramsFromVar)

			} else {
				if outputDebugInfo {
					f.WriteString(fmt.Sprintf("# variable %s could NOT BE RESOLVED\n", param))
				}
			}
		case systemHeaderSearchPath:
			if i < len(params)-1 {
				compilerParameters.systemHeaderSearchPath =
					append(compilerParameters.systemHeaderSearchPath, params[i+1])
			} else if outputDebugInfo {
				f.WriteString("# Found a header search path marker with no path")
			}
			i = i + 1
		case flag:
			c := cleanupParameter(param)
			f.WriteString(fmt.Sprintf("# FLAG '%s' became %s\n", param, c))
			compilerParameters.flags = append(compilerParameters.flags, c)
		case systemRoot:
			if i < len(params)-1 {
				compilerParameters.sysroot = params[i+1]
			} else if outputDebugInfo {
				f.WriteString("# Found a system root path marker with no path")
			}
			i = i + 1
		case relativeFilePathFlag:
			flagComponents := strings.Split(param, "=")
			if len(flagComponents) == 2 {
				flagStruct := relativeFilePathFlagType{flag: flagComponents[0], relativeFilePath: flagComponents[1]}
				compilerParameters.relativeFilePathFlags = append(compilerParameters.relativeFilePathFlags, flagStruct)
			} else {
				if outputDebugInfo {
					f.WriteString(fmt.Sprintf("# Relative File Path Flag [%s] is not formatted as a=b/c/d \n", param))
				}
			}
		}
	}
	return compilerParameters
}

func cleanupParameter(p string) string {
	// In the blueprint, c flags can be passed as:
	//  cflags: [ "-DLOG_TAG=\"libEGL\"", ]
	// which becomes:
	// '-DLOG_TAG="libEGL"' in soong.
	// In order to be injected in CMakelists.txt we need to:
	// - Remove the wrapping ' character
	// - Double escape all special \ and " characters.
	// For a end result like:
	// -DLOG_TAG=\\\"libEGL\\\"
	if !strings.HasPrefix(p, "'") || !strings.HasSuffix(p, "'") || len(p) < 3 {
		return p
	}

	// Reverse wrapper quotes and escaping that may have happened in NinjaAndShellEscape
	// TODO:  It is ok to reverse here for now but if NinjaAndShellEscape becomes more complex,
	// we should create a method NinjaAndShellUnescape in escape.go and use that instead.
	p = p[1 : len(p)-1]
	p = strings.Replace(p, `'\''`, `'`, -1)
	p = strings.Replace(p, `$$`, `$`, -1)

	p = doubleEscape(p)
	return p
}

func escape(s string) string {
	s = strings.Replace(s, `\`, `\\`, -1)
	s = strings.Replace(s, `"`, `\"`, -1)
	return s
}

func doubleEscape(s string) string {
	s = escape(s)
	s = escape(s)
	return s
}

func concatenateParams(c1 *compilerParameters, c2 compilerParameters) {
	c1.headerSearchPath = append(c1.headerSearchPath, c2.headerSearchPath...)
	c1.systemHeaderSearchPath = append(c1.systemHeaderSearchPath, c2.systemHeaderSearchPath...)
	if c2.sysroot != "" {
		c1.sysroot = c2.sysroot
	}
	c1.flags = append(c1.flags, c2.flags...)
}

func evalVariable(ctx android.SingletonContext, str string) (string, error) {
	evaluated, err := ctx.Eval(pctx, str)
	if err == nil {
		return evaluated, nil
	}
	return "", err
}

func getCMakeListsForModule(module *Module, ctx android.SingletonContext) string {
	return filepath.Join(android.AbsSrcDirForExistingUseCases(),
		cLionOutputProjectsDirectory,
		path.Dir(ctx.BlueprintFile(module)),
		module.ModuleBase.Name()+"-"+
			module.ModuleBase.Arch().ArchType.Name+"-"+
			module.ModuleBase.Os().Name,
		cMakeListsFilename)
}
