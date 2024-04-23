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

package android

import (
	"path/filepath"
	"strings"
)

func init() {
	RegisterParallelSingletonType("testsuites", testSuiteFilesFactory)
}

func testSuiteFilesFactory() Singleton {
	return &testSuiteFiles{}
}

type testSuiteFiles struct {
	robolectric []Path
	ravenwood   []Path
}

type TestSuiteModule interface {
	Module
	TestSuites() []string
}

func (t *testSuiteFiles) GenerateBuildActions(ctx SingletonContext) {
	files := make(map[string]map[string]InstallPaths)

	ctx.VisitAllModules(func(m Module) {
		if tsm, ok := m.(TestSuiteModule); ok {
			for _, testSuite := range tsm.TestSuites() {
				if files[testSuite] == nil {
					files[testSuite] = make(map[string]InstallPaths)
				}
				name := ctx.ModuleName(m)
				files[testSuite][name] = append(files[testSuite][name], tsm.FilesToInstall()...)
			}
		}
	})

	t.robolectric = robolectricTestSuite(ctx, files["robolectric-tests"])
	ctx.Phony("robolectric-tests", t.robolectric...)

	t.ravenwood = ravenwoodTestSuite(ctx, files["ravenwood-tests"])
	ctx.Phony("ravenwood-tests", t.ravenwood...)
}

func (t *testSuiteFiles) MakeVars(ctx MakeVarsContext) {
	ctx.DistForGoal("robolectric-tests", t.robolectric...)
	ctx.DistForGoal("ravenwood-tests", t.ravenwood...)
}

func robolectricTestSuite(ctx SingletonContext, files map[string]InstallPaths) []Path {
	var installedPaths InstallPaths
	for _, module := range SortedKeys(files) {
		installedPaths = append(installedPaths, files[module]...)
	}

	outputFile := pathForPackaging(ctx, "robolectric-tests.zip")
	rule := NewRuleBuilder(pctx, ctx)
	rule.Command().BuiltTool("soong_zip").
		FlagWithOutput("-o ", outputFile).
		FlagWithArg("-P ", "host/testcases").
		FlagWithArg("-C ", pathForTestCases(ctx).String()).
		FlagWithRspFileInputList("-r ", outputFile.ReplaceExtension(ctx, "rsp"), installedPaths.Paths()).
		Flag("-sha256") // necessary to save cas_uploader's time

	testList := buildTestList(ctx, "robolectric-tests_list", installedPaths)
	testListZipOutputFile := pathForPackaging(ctx, "robolectric-tests_list.zip")

	rule.Command().BuiltTool("soong_zip").
		FlagWithOutput("-o ", testListZipOutputFile).
		FlagWithArg("-C ", pathForPackaging(ctx).String()).
		FlagWithInput("-f ", testList).
		Flag("-sha256")

	rule.Build("robolectric_tests_zip", "robolectric-tests.zip")

	return []Path{outputFile, testListZipOutputFile}
}

func ravenwoodTestSuite(ctx SingletonContext, files map[string]InstallPaths) []Path {
	var installedPaths InstallPaths
	for _, module := range SortedKeys(files) {
		installedPaths = append(installedPaths, files[module]...)
	}

	outputFile := pathForPackaging(ctx, "ravenwood-tests.zip")
	rule := NewRuleBuilder(pctx, ctx)
	rule.Command().BuiltTool("soong_zip").
		FlagWithOutput("-o ", outputFile).
		FlagWithArg("-P ", "host/testcases").
		FlagWithArg("-C ", pathForTestCases(ctx).String()).
		FlagWithRspFileInputList("-r ", outputFile.ReplaceExtension(ctx, "rsp"), installedPaths.Paths()).
		Flag("-sha256") // necessary to save cas_uploader's time

	testList := buildTestList(ctx, "ravenwood-tests_list", installedPaths)
	testListZipOutputFile := pathForPackaging(ctx, "ravenwood-tests_list.zip")

	rule.Command().BuiltTool("soong_zip").
		FlagWithOutput("-o ", testListZipOutputFile).
		FlagWithArg("-C ", pathForPackaging(ctx).String()).
		FlagWithInput("-f ", testList).
		Flag("-sha256")

	rule.Build("ravenwood_tests_zip", "ravenwood-tests.zip")

	return []Path{outputFile, testListZipOutputFile}
}

func buildTestList(ctx SingletonContext, listFile string, installedPaths InstallPaths) Path {
	buf := &strings.Builder{}
	for _, p := range installedPaths {
		if p.Ext() != ".config" {
			continue
		}
		pc, err := toTestListPath(p.String(), pathForTestCases(ctx).String(), "host/testcases")
		if err != nil {
			ctx.Errorf("Failed to convert path: %s, %v", p.String(), err)
			continue
		}
		buf.WriteString(pc)
		buf.WriteString("\n")
	}
	outputFile := pathForPackaging(ctx, listFile)
	WriteFileRuleVerbatim(ctx, outputFile, buf.String())
	return outputFile
}

func toTestListPath(path, relativeRoot, prefix string) (string, error) {
	dest, err := filepath.Rel(relativeRoot, path)
	if err != nil {
		return "", err
	}
	return filepath.Join(prefix, dest), nil
}

func pathForPackaging(ctx PathContext, pathComponents ...string) OutputPath {
	pathComponents = append([]string{"packaging"}, pathComponents...)
	return PathForOutput(ctx, pathComponents...)
}

func pathForTestCases(ctx PathContext) InstallPath {
	return pathForInstall(ctx, ctx.Config().BuildOS, X86, "testcases")
}
