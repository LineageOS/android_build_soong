// Copyright 2021 Google Inc. All rights reserved.
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

package bp2build

/*
For shareable/common bp2build testing functionality and dumping ground for
specific-but-shared functionality among tests in package
*/

import (
	"fmt"
	"strings"
	"testing"

	"android/soong/android"
	"android/soong/android/allowlists"
	"android/soong/bazel"
)

var (
	// A default configuration for tests to not have to specify bp2build_available on top level targets.
	bp2buildConfig = android.NewBp2BuildAllowlist().SetDefaultConfig(
		allowlists.Bp2BuildConfig{
			android.Bp2BuildTopLevel: allowlists.Bp2BuildDefaultTrueRecursively,
		},
	)

	buildDir string
)

func checkError(t *testing.T, errs []error, expectedErr error) bool {
	t.Helper()

	if len(errs) != 1 {
		return false
	}
	if strings.Contains(errs[0].Error(), expectedErr.Error()) {
		return true
	}

	return false
}

func errored(t *testing.T, tc bp2buildTestCase, errs []error) bool {
	t.Helper()
	if tc.expectedErr != nil {
		// Rely on checkErrors, as this test case is expected to have an error.
		return false
	}

	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("%s: %s", tc.description, err)
		}
		return true
	}

	// All good, continue execution.
	return false
}

func runBp2BuildTestCaseSimple(t *testing.T, tc bp2buildTestCase) {
	t.Helper()
	runBp2BuildTestCase(t, func(ctx android.RegistrationContext) {}, tc)
}

type bp2buildTestCase struct {
	description                string
	moduleTypeUnderTest        string
	moduleTypeUnderTestFactory android.ModuleFactory
	blueprint                  string
	expectedBazelTargets       []string
	filesystem                 map[string]string
	dir                        string
	expectedErr                error
	unconvertedDepsMode        unconvertedDepsMode
}

func runBp2BuildTestCase(t *testing.T, registerModuleTypes func(ctx android.RegistrationContext), tc bp2buildTestCase) {
	t.Helper()
	dir := "."
	filesystem := make(map[string][]byte)
	toParse := []string{
		"Android.bp",
	}
	for f, content := range tc.filesystem {
		if strings.HasSuffix(f, "Android.bp") {
			toParse = append(toParse, f)
		}
		filesystem[f] = []byte(content)
	}
	config := android.TestConfig(buildDir, nil, tc.blueprint, filesystem)
	ctx := android.NewTestContext(config)

	registerModuleTypes(ctx)
	ctx.RegisterModuleType(tc.moduleTypeUnderTest, tc.moduleTypeUnderTestFactory)
	ctx.RegisterBp2BuildConfig(bp2buildConfig)
	ctx.RegisterForBazelConversion()

	_, parseErrs := ctx.ParseFileList(dir, toParse)
	if errored(t, tc, parseErrs) {
		return
	}
	_, resolveDepsErrs := ctx.ResolveDependencies(config)
	if errored(t, tc, resolveDepsErrs) {
		return
	}

	errs := append(parseErrs, resolveDepsErrs...)
	if tc.expectedErr != nil && checkError(t, errs, tc.expectedErr) {
		return
	}

	checkDir := dir
	if tc.dir != "" {
		checkDir = tc.dir
	}
	codegenCtx := NewCodegenContext(config, *ctx.Context, Bp2Build)
	codegenCtx.unconvertedDepMode = tc.unconvertedDepsMode
	bazelTargets, errs := generateBazelTargetsForDir(codegenCtx, checkDir)
	if tc.expectedErr != nil {
		if checkError(t, errs, tc.expectedErr) {
			return
		} else {
			t.Errorf("Expected error: %q, got: %q", tc.expectedErr, errs)
		}
	} else {
		android.FailIfErrored(t, errs)
	}
	if actualCount, expectedCount := len(bazelTargets), len(tc.expectedBazelTargets); actualCount != expectedCount {
		t.Errorf("%s: Expected %d bazel target (%s), got `%d`` (%s)",
			tc.description, expectedCount, tc.expectedBazelTargets, actualCount, bazelTargets)
	} else {
		for i, target := range bazelTargets {
			if w, g := tc.expectedBazelTargets[i], target.content; w != g {
				t.Errorf(
					"%s: Expected generated Bazel target to be `%s`, got `%s`",
					tc.description, w, g)
			}
		}
	}
}

type nestedProps struct {
	Nested_prop *string
}

type EmbeddedProps struct {
	Embedded_prop *string
}

type OtherEmbeddedProps struct {
	Other_embedded_prop *string
}

type customProps struct {
	EmbeddedProps
	*OtherEmbeddedProps

	Bool_prop     bool
	Bool_ptr_prop *bool
	// Ensure that properties tagged `blueprint:mutated` are omitted
	Int_prop         int `blueprint:"mutated"`
	Int64_ptr_prop   *int64
	String_prop      string
	String_ptr_prop  *string
	String_list_prop []string

	Nested_props     nestedProps
	Nested_props_ptr *nestedProps

	Arch_paths         []string `android:"path,arch_variant"`
	Arch_paths_exclude []string `android:"path,arch_variant"`

	// Prop used to indicate this conversion should be 1 module -> multiple targets
	One_to_many_prop *bool
}

type customModule struct {
	android.ModuleBase
	android.BazelModuleBase

	props customProps
}

// OutputFiles is needed because some instances of this module use dist with a
// tag property which requires the module implements OutputFileProducer.
func (m *customModule) OutputFiles(tag string) (android.Paths, error) {
	return android.PathsForTesting("path" + tag), nil
}

func (m *customModule) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	// nothing for now.
}

func customModuleFactoryBase() android.Module {
	module := &customModule{}
	module.AddProperties(&module.props)
	android.InitBazelModule(module)
	return module
}

func customModuleFactory() android.Module {
	m := customModuleFactoryBase()
	android.InitAndroidArchModule(m, android.HostAndDeviceSupported, android.MultilibBoth)
	return m
}

type testProps struct {
	Test_prop struct {
		Test_string_prop string
	}
}

type customTestModule struct {
	android.ModuleBase

	props      customProps
	test_props testProps
}

func (m *customTestModule) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	// nothing for now.
}

func customTestModuleFactoryBase() android.Module {
	m := &customTestModule{}
	m.AddProperties(&m.props)
	m.AddProperties(&m.test_props)
	return m
}

func customTestModuleFactory() android.Module {
	m := customTestModuleFactoryBase()
	android.InitAndroidModule(m)
	return m
}

type customDefaultsModule struct {
	android.ModuleBase
	android.DefaultsModuleBase
}

func customDefaultsModuleFactoryBase() android.DefaultsModule {
	module := &customDefaultsModule{}
	module.AddProperties(&customProps{})
	return module
}

func customDefaultsModuleFactoryBasic() android.Module {
	return customDefaultsModuleFactoryBase()
}

func customDefaultsModuleFactory() android.Module {
	m := customDefaultsModuleFactoryBase()
	android.InitDefaultsModule(m)
	return m
}

type EmbeddedAttr struct {
	Embedded_attr *string
}

type OtherEmbeddedAttr struct {
	Other_embedded_attr *string
}

type customBazelModuleAttributes struct {
	EmbeddedAttr
	*OtherEmbeddedAttr
	String_ptr_prop  *string
	String_list_prop []string
	Arch_paths       bazel.LabelListAttribute
}

func (m *customModule) ConvertWithBp2build(ctx android.TopDownMutatorContext) {
	paths := bazel.LabelListAttribute{}

	if p := m.props.One_to_many_prop; p != nil && *p {
		customBp2buildOneToMany(ctx, m)
		return
	}

	for axis, configToProps := range m.GetArchVariantProperties(ctx, &customProps{}) {
		for config, props := range configToProps {
			if archProps, ok := props.(*customProps); ok && archProps.Arch_paths != nil {
				paths.SetSelectValue(axis, config, android.BazelLabelForModuleSrcExcludes(ctx, archProps.Arch_paths, archProps.Arch_paths_exclude))
			}
		}
	}

	paths.ResolveExcludes()

	attrs := &customBazelModuleAttributes{
		String_ptr_prop:  m.props.String_ptr_prop,
		String_list_prop: m.props.String_list_prop,
		Arch_paths:       paths,
	}
	attrs.Embedded_attr = m.props.Embedded_prop
	if m.props.OtherEmbeddedProps != nil {
		attrs.OtherEmbeddedAttr = &OtherEmbeddedAttr{Other_embedded_attr: m.props.OtherEmbeddedProps.Other_embedded_prop}
	}

	props := bazel.BazelTargetModuleProperties{
		Rule_class: "custom",
	}

	ctx.CreateBazelTargetModule(props, android.CommonAttributes{Name: m.Name()}, attrs)
}

// A bp2build mutator that uses load statements and creates a 1:M mapping from
// module to target.
func customBp2buildOneToMany(ctx android.TopDownMutatorContext, m *customModule) {

	baseName := m.Name()
	attrs := &customBazelModuleAttributes{}

	myLibraryProps := bazel.BazelTargetModuleProperties{
		Rule_class:        "my_library",
		Bzl_load_location: "//build/bazel/rules:rules.bzl",
	}
	ctx.CreateBazelTargetModule(myLibraryProps, android.CommonAttributes{Name: baseName}, attrs)

	protoLibraryProps := bazel.BazelTargetModuleProperties{
		Rule_class:        "proto_library",
		Bzl_load_location: "//build/bazel/rules:proto.bzl",
	}
	ctx.CreateBazelTargetModule(protoLibraryProps, android.CommonAttributes{Name: baseName + "_proto_library_deps"}, attrs)

	myProtoLibraryProps := bazel.BazelTargetModuleProperties{
		Rule_class:        "my_proto_library",
		Bzl_load_location: "//build/bazel/rules:proto.bzl",
	}
	ctx.CreateBazelTargetModule(myProtoLibraryProps, android.CommonAttributes{Name: baseName + "_my_proto_library_deps"}, attrs)
}

// Helper method for tests to easily access the targets in a dir.
func generateBazelTargetsForDir(codegenCtx *CodegenContext, dir string) (BazelTargets, []error) {
	// TODO: Set generateFilegroups to true and/or remove the generateFilegroups argument completely
	res, err := GenerateBazelTargets(codegenCtx, false)
	return res.buildFileToTargets[dir], err
}

func registerCustomModuleForBp2buildConversion(ctx *android.TestContext) {
	ctx.RegisterModuleType("custom", customModuleFactory)
	ctx.RegisterForBazelConversion()
}

func simpleModuleDoNotConvertBp2build(typ, name string) string {
	return fmt.Sprintf(`
%s {
		name: "%s",
		bazel_module: { bp2build_available: false },
}`, typ, name)
}

type attrNameToString map[string]string

func makeBazelTarget(typ, name string, attrs attrNameToString) string {
	attrStrings := make([]string, 0, len(attrs)+1)
	attrStrings = append(attrStrings, fmt.Sprintf(`    name = "%s",`, name))
	for _, k := range android.SortedStringKeys(attrs) {
		attrStrings = append(attrStrings, fmt.Sprintf("    %s = %s,", k, attrs[k]))
	}
	return fmt.Sprintf(`%s(
%s
)`, typ, strings.Join(attrStrings, "\n"))
}
