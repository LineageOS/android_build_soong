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

	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/android/allowlists"
	"android/soong/bazel"
)

var (
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

func errored(t *testing.T, tc Bp2buildTestCase, errs []error) bool {
	t.Helper()
	if tc.ExpectedErr != nil {
		// Rely on checkErrors, as this test case is expected to have an error.
		return false
	}

	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("%s: %s", tc.Description, err)
		}
		return true
	}

	// All good, continue execution.
	return false
}

func RunBp2BuildTestCaseSimple(t *testing.T, tc Bp2buildTestCase) {
	t.Helper()
	RunBp2BuildTestCase(t, func(ctx android.RegistrationContext) {}, tc)
}

type Bp2buildTestCase struct {
	Description                string
	ModuleTypeUnderTest        string
	ModuleTypeUnderTestFactory android.ModuleFactory
	Blueprint                  string
	ExpectedBazelTargets       []string
	Filesystem                 map[string]string
	Dir                        string
	// An error with a string contained within the string of the expected error
	ExpectedErr         error
	UnconvertedDepsMode unconvertedDepsMode

	// For every directory listed here, the BUILD file for that directory will
	// be merged with the generated BUILD file. This allows custom BUILD targets
	// to be used in tests, or use BUILD files to draw package boundaries.
	KeepBuildFileForDirs []string
}

func RunBp2BuildTestCase(t *testing.T, registerModuleTypes func(ctx android.RegistrationContext), tc Bp2buildTestCase) {
	t.Helper()
	bp2buildSetup := android.GroupFixturePreparers(
		android.FixtureRegisterWithContext(registerModuleTypes),
		SetBp2BuildTestRunner,
	)
	runBp2BuildTestCaseWithSetup(t, bp2buildSetup, tc)
}

func RunApiBp2BuildTestCase(t *testing.T, registerModuleTypes func(ctx android.RegistrationContext), tc Bp2buildTestCase) {
	t.Helper()
	apiBp2BuildSetup := android.GroupFixturePreparers(
		android.FixtureRegisterWithContext(registerModuleTypes),
		SetApiBp2BuildTestRunner,
	)
	runBp2BuildTestCaseWithSetup(t, apiBp2BuildSetup, tc)
}

func runBp2BuildTestCaseWithSetup(t *testing.T, extraPreparer android.FixturePreparer, tc Bp2buildTestCase) {
	t.Helper()
	dir := "."
	filesystem := make(map[string][]byte)
	for f, content := range tc.Filesystem {
		filesystem[f] = []byte(content)
	}

	preparers := []android.FixturePreparer{
		extraPreparer,
		android.FixtureMergeMockFs(filesystem),
		android.FixtureWithRootAndroidBp(tc.Blueprint),
		android.FixtureRegisterWithContext(func(ctx android.RegistrationContext) {
			ctx.RegisterModuleType(tc.ModuleTypeUnderTest, tc.ModuleTypeUnderTestFactory)
		}),
		android.FixtureModifyContext(func(ctx *android.TestContext) {
			// A default configuration for tests to not have to specify bp2build_available on top level
			// targets.
			bp2buildConfig := android.NewBp2BuildAllowlist().SetDefaultConfig(
				allowlists.Bp2BuildConfig{
					android.Bp2BuildTopLevel: allowlists.Bp2BuildDefaultTrueRecursively,
				},
			)
			for _, f := range tc.KeepBuildFileForDirs {
				bp2buildConfig.SetKeepExistingBuildFile(map[string]bool{
					f: /*recursive=*/ false,
				})
			}
			ctx.RegisterBp2BuildConfig(bp2buildConfig)
		}),
		android.FixtureModifyEnv(func(env map[string]string) {
			if tc.UnconvertedDepsMode == errorModulesUnconvertedDeps {
				env["BP2BUILD_ERROR_UNCONVERTED"] = "true"
			}
		}),
	}

	preparer := android.GroupFixturePreparers(preparers...)
	if tc.ExpectedErr != nil {
		pattern := "\\Q" + tc.ExpectedErr.Error() + "\\E"
		preparer = preparer.ExtendWithErrorHandler(android.FixtureExpectsOneErrorPattern(pattern))
	}
	result := preparer.RunTestWithCustomResult(t).(*BazelTestResult)
	if len(result.Errs) > 0 {
		return
	}

	checkDir := dir
	if tc.Dir != "" {
		checkDir = tc.Dir
	}
	expectedTargets := map[string][]string{
		checkDir: tc.ExpectedBazelTargets,
	}

	result.CompareAllBazelTargets(t, tc.Description, expectedTargets, true)
}

// SetBp2BuildTestRunner customizes the test fixture mechanism to run tests in Bp2Build mode.
var SetBp2BuildTestRunner = android.FixtureSetTestRunner(&bazelTestRunner{Bp2Build})

// SetApiBp2BuildTestRunner customizes the test fixture mechanism to run tests in ApiBp2build mode.
var SetApiBp2BuildTestRunner = android.FixtureSetTestRunner(&bazelTestRunner{ApiBp2build})

// bazelTestRunner customizes the test fixture mechanism to run tests of the bp2build and
// apiBp2build build modes.
type bazelTestRunner struct {
	mode CodegenMode
}

func (b *bazelTestRunner) FinalPreparer(result *android.TestResult) android.CustomTestResult {
	ctx := result.TestContext
	switch b.mode {
	case Bp2Build:
		ctx.RegisterForBazelConversion()
	case ApiBp2build:
		ctx.RegisterForApiBazelConversion()
	default:
		panic(fmt.Errorf("unknown build mode: %d", b.mode))
	}

	return &BazelTestResult{TestResult: result}
}

func (b *bazelTestRunner) PostParseProcessor(result android.CustomTestResult) {
	bazelResult := result.(*BazelTestResult)
	ctx := bazelResult.TestContext
	config := bazelResult.Config
	_, errs := ctx.ResolveDependencies(config)
	if bazelResult.CollateErrs(errs) {
		return
	}

	codegenCtx := NewCodegenContext(config, ctx.Context, Bp2Build)
	res, errs := GenerateBazelTargets(codegenCtx, false)
	if bazelResult.CollateErrs(errs) {
		return
	}

	// Store additional data for access by tests.
	bazelResult.conversionResults = res
}

// BazelTestResult is a wrapper around android.TestResult to provide type safe access to the bazel
// specific data stored by the bazelTestRunner.
type BazelTestResult struct {
	*android.TestResult

	// The result returned by the GenerateBazelTargets function.
	conversionResults
}

// CompareAllBazelTargets compares the BazelTargets produced by the test for all the directories
// with the supplied set of expected targets.
//
// If ignoreUnexpected=false then this enforces an exact match where every BazelTarget produced must
// have a corresponding expected BazelTarget.
//
// If ignoreUnexpected=true then it will ignore directories for which there are no expected targets.
func (b BazelTestResult) CompareAllBazelTargets(t *testing.T, description string, expectedTargets map[string][]string, ignoreUnexpected bool) {
	actualTargets := b.buildFileToTargets

	// Generate the sorted set of directories to check.
	dirsToCheck := android.SortedStringKeys(expectedTargets)
	if !ignoreUnexpected {
		// This needs to perform an exact match so add the directories in which targets were
		// produced to the list of directories to check.
		dirsToCheck = append(dirsToCheck, android.SortedStringKeys(actualTargets)...)
		dirsToCheck = android.SortedUniqueStrings(dirsToCheck)
	}

	for _, dir := range dirsToCheck {
		expected := expectedTargets[dir]
		actual := actualTargets[dir]

		if expected == nil {
			if actual != nil {
				t.Errorf("did not expect any bazel modules in %q but found %d", dir, len(actual))
			}
		} else if actual == nil {
			expectedCount := len(expected)
			if expectedCount > 0 {
				t.Errorf("expected %d bazel modules in %q but did not find any", expectedCount, dir)
			}
		} else {
			b.CompareBazelTargets(t, description, expected, actual)
		}
	}
}

func (b BazelTestResult) CompareBazelTargets(t *testing.T, description string, expectedContents []string, actualTargets BazelTargets) {
	if actualCount, expectedCount := len(actualTargets), len(expectedContents); actualCount != expectedCount {
		t.Errorf("%s: Expected %d bazel target (%s), got %d (%s)",
			description, expectedCount, expectedContents, actualCount, actualTargets)
	} else {
		for i, actualTarget := range actualTargets {
			if w, g := expectedContents[i], actualTarget.content; w != g {
				t.Errorf(
					"%s[%d]: Expected generated Bazel target to be `%s`, got `%s`",
					description, i, w, g)
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
	Int_prop            int `blueprint:"mutated"`
	Int64_ptr_prop      *int64
	String_prop         string
	String_literal_prop *string `android:"arch_variant"`
	String_ptr_prop     *string
	String_list_prop    []string

	Nested_props     nestedProps
	Nested_props_ptr *nestedProps

	Arch_paths         []string `android:"path,arch_variant"`
	Arch_paths_exclude []string `android:"path,arch_variant"`

	// Prop used to indicate this conversion should be 1 module -> multiple targets
	One_to_many_prop *bool

	Api *string // File describing the APIs of this module
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

func customModuleFactoryHostAndDevice() android.Module {
	m := customModuleFactoryBase()
	android.InitAndroidArchModule(m, android.HostAndDeviceSupported, android.MultilibBoth)
	return m
}

func customModuleFactoryDeviceSupported() android.Module {
	m := customModuleFactoryBase()
	android.InitAndroidArchModule(m, android.DeviceSupported, android.MultilibBoth)
	return m
}

func customModuleFactoryHostSupported() android.Module {
	m := customModuleFactoryBase()
	android.InitAndroidArchModule(m, android.HostSupported, android.MultilibBoth)
	return m
}

func customModuleFactoryHostAndDeviceDefault() android.Module {
	m := customModuleFactoryBase()
	android.InitAndroidArchModule(m, android.HostAndDeviceDefault, android.MultilibBoth)
	return m
}

func customModuleFactoryNeitherHostNorDeviceSupported() android.Module {
	m := customModuleFactoryBase()
	android.InitAndroidArchModule(m, android.NeitherHostNorDeviceSupported, android.MultilibBoth)
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
	String_literal_prop bazel.StringAttribute
	String_ptr_prop     *string
	String_list_prop    []string
	Arch_paths          bazel.LabelListAttribute
	Api                 bazel.LabelAttribute
}

func (m *customModule) ConvertWithBp2build(ctx android.TopDownMutatorContext) {
	if p := m.props.One_to_many_prop; p != nil && *p {
		customBp2buildOneToMany(ctx, m)
		return
	}

	paths := bazel.LabelListAttribute{}
	strAttr := bazel.StringAttribute{}
	for axis, configToProps := range m.GetArchVariantProperties(ctx, &customProps{}) {
		for config, props := range configToProps {
			if custProps, ok := props.(*customProps); ok {
				if custProps.Arch_paths != nil {
					paths.SetSelectValue(axis, config, android.BazelLabelForModuleSrcExcludes(ctx, custProps.Arch_paths, custProps.Arch_paths_exclude))
				}
				if custProps.String_literal_prop != nil {
					strAttr.SetSelectValue(axis, config, custProps.String_literal_prop)
				}
			}
		}
	}

	paths.ResolveExcludes()

	attrs := &customBazelModuleAttributes{
		String_literal_prop: strAttr,
		String_ptr_prop:     m.props.String_ptr_prop,
		String_list_prop:    m.props.String_list_prop,
		Arch_paths:          paths,
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

var _ android.ApiProvider = (*customModule)(nil)

func (c *customModule) ConvertWithApiBp2build(ctx android.TopDownMutatorContext) {
	props := bazel.BazelTargetModuleProperties{
		Rule_class: "custom_api_contribution",
	}
	apiAttribute := bazel.MakeLabelAttribute(
		android.BazelLabelForModuleSrcSingle(ctx, proptools.String(c.props.Api)).Label,
	)
	attrs := &customBazelModuleAttributes{
		Api: *apiAttribute,
	}
	ctx.CreateBazelTargetModule(props,
		android.CommonAttributes{Name: c.Name()},
		attrs)
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
	if err != nil {
		return BazelTargets{}, err
	}
	return res.buildFileToTargets[dir], err
}

func registerCustomModuleForBp2buildConversion(ctx *android.TestContext) {
	ctx.RegisterModuleType("custom", customModuleFactoryHostAndDevice)
	ctx.RegisterForBazelConversion()
}

func simpleModuleDoNotConvertBp2build(typ, name string) string {
	return fmt.Sprintf(`
%s {
		name: "%s",
		bazel_module: { bp2build_available: false },
}`, typ, name)
}

type AttrNameToString map[string]string

func (a AttrNameToString) clone() AttrNameToString {
	newAttrs := make(AttrNameToString, len(a))
	for k, v := range a {
		newAttrs[k] = v
	}
	return newAttrs
}

// makeBazelTargetNoRestrictions returns bazel target build file definition that can be host or
// device specific, or independent of host/device.
func makeBazelTargetHostOrDevice(typ, name string, attrs AttrNameToString, hod android.HostOrDeviceSupported) string {
	if _, ok := attrs["target_compatible_with"]; !ok {
		switch hod {
		case android.HostSupported:
			attrs["target_compatible_with"] = `select({
        "//build/bazel/platforms/os:android": ["@platforms//:incompatible"],
        "//conditions:default": [],
    })`
		case android.DeviceSupported:
			attrs["target_compatible_with"] = `["//build/bazel/platforms/os:android"]`
		}
	}

	attrStrings := make([]string, 0, len(attrs)+1)
	if name != "" {
		attrStrings = append(attrStrings, fmt.Sprintf(`    name = "%s",`, name))
	}
	for _, k := range android.SortedStringKeys(attrs) {
		attrStrings = append(attrStrings, fmt.Sprintf("    %s = %s,", k, attrs[k]))
	}
	return fmt.Sprintf(`%s(
%s
)`, typ, strings.Join(attrStrings, "\n"))
}

// MakeBazelTargetNoRestrictions returns bazel target build file definition that does not add a
// target_compatible_with.  This is useful for module types like filegroup and genrule that arch not
// arch variant
func MakeBazelTargetNoRestrictions(typ, name string, attrs AttrNameToString) string {
	return makeBazelTargetHostOrDevice(typ, name, attrs, android.HostAndDeviceDefault)
}

// makeBazelTargetNoRestrictions returns bazel target build file definition that is device specific
// as this is the most common default in Soong.
func MakeBazelTarget(typ, name string, attrs AttrNameToString) string {
	return makeBazelTargetHostOrDevice(typ, name, attrs, android.DeviceSupported)
}

type ExpectedRuleTarget struct {
	Rule  string
	Name  string
	Attrs AttrNameToString
	Hod   android.HostOrDeviceSupported
}

func (ebr ExpectedRuleTarget) String() string {
	return makeBazelTargetHostOrDevice(ebr.Rule, ebr.Name, ebr.Attrs, ebr.Hod)
}

func makeCcStubSuiteTargets(name string, attrs AttrNameToString) string {
	if _, hasStubs := attrs["stubs_symbol_file"]; !hasStubs {
		return ""
	}
	STUB_SUITE_ATTRS := map[string]string{
		"stubs_symbol_file": "symbol_file",
		"stubs_versions":    "versions",
		"soname":            "soname",
		"source_library":    "source_library",
	}

	stubSuiteAttrs := AttrNameToString{}
	for key, _ := range attrs {
		if _, stubSuiteAttr := STUB_SUITE_ATTRS[key]; stubSuiteAttr {
			stubSuiteAttrs[STUB_SUITE_ATTRS[key]] = attrs[key]
		}
	}
	return MakeBazelTarget("cc_stub_suite", name+"_stub_libs", stubSuiteAttrs)
}

func MakeNeverlinkDuplicateTarget(moduleType string, name string) string {
	return MakeBazelTarget(moduleType, name+"-neverlink", AttrNameToString{
		"neverlink": `True`,
		"exports":   `[":` + name + `"]`,
	})
}
