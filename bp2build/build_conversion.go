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

package bp2build

/*
For shareable/common functionality for conversion from soong-module to build files
for queryview/bp2build
*/

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"android/soong/android"
	"android/soong/bazel"
	"android/soong/starlark_fmt"
	"android/soong/ui/metrics/bp2build_metrics_proto"

	"github.com/google/blueprint"
	"github.com/google/blueprint/bootstrap"
	"github.com/google/blueprint/proptools"
)

type BazelAttributes struct {
	Attrs map[string]string
}

type BazelLoadSymbol struct {
	// The name of the symbol in the file being loaded
	symbol string
	// The name the symbol wil have in this file. Can be left blank to use the same name as symbol.
	alias string
}

type BazelLoad struct {
	file    string
	symbols []BazelLoadSymbol
}

type BazelTarget struct {
	name        string
	packageName string
	content     string
	ruleClass   string
	loads       []BazelLoad
}

// Label is the fully qualified Bazel label constructed from the BazelTarget's
// package name and target name.
func (t BazelTarget) Label() string {
	if t.packageName == "." {
		return "//:" + t.name
	} else {
		return "//" + t.packageName + ":" + t.name
	}
}

// PackageName returns the package of the Bazel target.
// Defaults to root of tree.
func (t BazelTarget) PackageName() string {
	if t.packageName == "" {
		return "."
	}
	return t.packageName
}

// BazelTargets is a typedef for a slice of BazelTarget objects.
type BazelTargets []BazelTarget

func (targets BazelTargets) packageRule() *BazelTarget {
	for _, target := range targets {
		if target.ruleClass == "package" {
			return &target
		}
	}
	return nil
}

// sort a list of BazelTargets in-place, by name, and by generated/handcrafted types.
func (targets BazelTargets) sort() {
	sort.Slice(targets, func(i, j int) bool {
		return targets[i].name < targets[j].name
	})
}

// String returns the string representation of BazelTargets, without load
// statements (use LoadStatements for that), since the targets are usually not
// adjacent to the load statements at the top of the BUILD file.
func (targets BazelTargets) String() string {
	var res strings.Builder
	for i, target := range targets {
		if target.ruleClass != "package" {
			res.WriteString(target.content)
		}
		if i != len(targets)-1 {
			res.WriteString("\n\n")
		}
	}
	return res.String()
}

// LoadStatements return the string representation of the sorted and deduplicated
// Starlark rule load statements needed by a group of BazelTargets.
func (targets BazelTargets) LoadStatements() string {
	// First, merge all the load statements from all the targets onto one list
	bzlToLoadedSymbols := map[string][]BazelLoadSymbol{}
	for _, target := range targets {
		for _, load := range target.loads {
		outer:
			for _, symbol := range load.symbols {
				alias := symbol.alias
				if alias == "" {
					alias = symbol.symbol
				}
				for _, otherSymbol := range bzlToLoadedSymbols[load.file] {
					otherAlias := otherSymbol.alias
					if otherAlias == "" {
						otherAlias = otherSymbol.symbol
					}
					if symbol.symbol == otherSymbol.symbol && alias == otherAlias {
						continue outer
					} else if alias == otherAlias {
						panic(fmt.Sprintf("Conflicting destination (%s) for loads of %s and %s", alias, symbol.symbol, otherSymbol.symbol))
					}
				}
				bzlToLoadedSymbols[load.file] = append(bzlToLoadedSymbols[load.file], symbol)
			}
		}
	}

	var loadStatements strings.Builder
	for i, bzl := range android.SortedKeys(bzlToLoadedSymbols) {
		symbols := bzlToLoadedSymbols[bzl]
		loadStatements.WriteString("load(\"")
		loadStatements.WriteString(bzl)
		loadStatements.WriteString("\", ")
		sort.Slice(symbols, func(i, j int) bool {
			if symbols[i].symbol < symbols[j].symbol {
				return true
			}
			return symbols[i].alias < symbols[j].alias
		})
		for j, symbol := range symbols {
			if symbol.alias != "" && symbol.alias != symbol.symbol {
				loadStatements.WriteString(symbol.alias)
				loadStatements.WriteString(" = ")
			}
			loadStatements.WriteString("\"")
			loadStatements.WriteString(symbol.symbol)
			loadStatements.WriteString("\"")
			if j != len(symbols)-1 {
				loadStatements.WriteString(", ")
			}
		}
		loadStatements.WriteString(")")
		if i != len(bzlToLoadedSymbols)-1 {
			loadStatements.WriteString("\n")
		}
	}
	return loadStatements.String()
}

type bpToBuildContext interface {
	ModuleName(module blueprint.Module) string
	ModuleDir(module blueprint.Module) string
	ModuleSubDir(module blueprint.Module) string
	ModuleType(module blueprint.Module) string

	VisitAllModules(visit func(blueprint.Module))
	VisitDirectDeps(module blueprint.Module, visit func(blueprint.Module))
}

type CodegenContext struct {
	config             android.Config
	context            *android.Context
	mode               CodegenMode
	additionalDeps     []string
	unconvertedDepMode unconvertedDepsMode
	topDir             string
}

func (ctx *CodegenContext) Mode() CodegenMode {
	return ctx.mode
}

// CodegenMode is an enum to differentiate code-generation modes.
type CodegenMode int

const (
	// Bp2Build - generate BUILD files with targets buildable by Bazel directly.
	//
	// This mode is used for the Soong->Bazel build definition conversion.
	Bp2Build CodegenMode = iota

	// QueryView - generate BUILD files with targets representing fully mutated
	// Soong modules, representing the fully configured Soong module graph with
	// variants and dependency edges.
	//
	// This mode is used for discovering and introspecting the existing Soong
	// module graph.
	QueryView
)

type unconvertedDepsMode int

const (
	// Include a warning in conversion metrics about converted modules with unconverted direct deps
	warnUnconvertedDeps unconvertedDepsMode = iota
	// Error and fail conversion if encountering a module with unconverted direct deps
	// Enabled by setting environment variable `BP2BUILD_ERROR_UNCONVERTED`
	errorModulesUnconvertedDeps
)

func (mode CodegenMode) String() string {
	switch mode {
	case Bp2Build:
		return "Bp2Build"
	case QueryView:
		return "QueryView"
	default:
		return fmt.Sprintf("%d", mode)
	}
}

// AddNinjaFileDeps adds dependencies on the specified files to be added to the ninja manifest. The
// primary builder will be rerun whenever the specified files are modified. Allows us to fulfill the
// PathContext interface in order to add dependencies on hand-crafted BUILD files. Note: must also
// call AdditionalNinjaDeps and add them manually to the ninja file.
func (ctx *CodegenContext) AddNinjaFileDeps(deps ...string) {
	ctx.additionalDeps = append(ctx.additionalDeps, deps...)
}

// AdditionalNinjaDeps returns additional ninja deps added by CodegenContext
func (ctx *CodegenContext) AdditionalNinjaDeps() []string {
	return ctx.additionalDeps
}

func (ctx *CodegenContext) Config() android.Config    { return ctx.config }
func (ctx *CodegenContext) Context() *android.Context { return ctx.context }

// NewCodegenContext creates a wrapper context that conforms to PathContext for
// writing BUILD files in the output directory.
func NewCodegenContext(config android.Config, context *android.Context, mode CodegenMode, topDir string) *CodegenContext {
	var unconvertedDeps unconvertedDepsMode
	if config.IsEnvTrue("BP2BUILD_ERROR_UNCONVERTED") {
		unconvertedDeps = errorModulesUnconvertedDeps
	}
	return &CodegenContext{
		context:            context,
		config:             config,
		mode:               mode,
		unconvertedDepMode: unconvertedDeps,
		topDir:             topDir,
	}
}

// props is an unsorted map. This function ensures that
// the generated attributes are sorted to ensure determinism.
func propsToAttributes(props map[string]string) string {
	var attributes string
	for _, propName := range android.SortedKeys(props) {
		attributes += fmt.Sprintf("    %s = %s,\n", propName, props[propName])
	}
	return attributes
}

type conversionResults struct {
	buildFileToTargets map[string]BazelTargets
	metrics            CodegenMetrics
}

func (r conversionResults) BuildDirToTargets() map[string]BazelTargets {
	return r.buildFileToTargets
}

// struct to store state of b bazel targets (e.g. go targets which do not implement android.Module)
// this implements bp2buildModule interface and is passed to generateBazelTargets
type bTarget struct {
	targetName            string
	targetPackage         string
	bazelRuleClass        string
	bazelRuleLoadLocation string
	bazelAttributes       []interface{}
}

var _ bp2buildModule = (*bTarget)(nil)

func (b bTarget) TargetName() string {
	return b.targetName
}

func (b bTarget) TargetPackage() string {
	return b.targetPackage
}

func (b bTarget) BazelRuleClass() string {
	return b.bazelRuleClass
}

func (b bTarget) BazelRuleLoadLocation() string {
	return b.bazelRuleLoadLocation
}

func (b bTarget) BazelAttributes() []interface{} {
	return b.bazelAttributes
}

// Creates a target_compatible_with entry that is *not* compatible with android
func targetNotCompatibleWithAndroid() bazel.LabelListAttribute {
	ret := bazel.LabelListAttribute{}
	ret.SetSelectValue(bazel.OsConfigurationAxis, bazel.OsAndroid,
		bazel.MakeLabelList(
			[]bazel.Label{
				bazel.Label{
					Label: "@platforms//:incompatible",
				},
			},
		),
	)
	return ret
}

// helper function to return labels for srcs used in bootstrap_go_package and bootstrap_go_binary
// this function has the following limitations which make it unsuitable for widespread use
// - wildcard patterns in srcs
// This is ok for go since build/blueprint does not support it.
//
// Prefer to use `BazelLabelForModuleSrc` instead
func goSrcLabels(cfg android.Config, moduleDir string, srcs []string, linuxSrcs, darwinSrcs []string) bazel.LabelListAttribute {
	labels := func(srcs []string) bazel.LabelList {
		ret := []bazel.Label{}
		for _, src := range srcs {
			srcLabel := bazel.Label{
				Label: src,
			}
			ret = append(ret, srcLabel)
		}
		// Respect package boundaries
		return android.TransformSubpackagePaths(
			cfg,
			moduleDir,
			bazel.MakeLabelList(ret),
		)
	}

	ret := bazel.LabelListAttribute{}
	// common
	ret.SetSelectValue(bazel.NoConfigAxis, "", labels(srcs))
	// linux
	ret.SetSelectValue(bazel.OsConfigurationAxis, bazel.OsLinux, labels(linuxSrcs))
	// darwin
	ret.SetSelectValue(bazel.OsConfigurationAxis, bazel.OsDarwin, labels(darwinSrcs))
	return ret
}

func goDepLabels(deps []string, goModulesMap nameToGoLibraryModule) bazel.LabelListAttribute {
	labels := []bazel.Label{}
	for _, dep := range deps {
		moduleDir := goModulesMap[dep].Dir
		if moduleDir == "." {
			moduleDir = ""
		}
		label := bazel.Label{
			Label: fmt.Sprintf("//%s:%s", moduleDir, dep),
		}
		labels = append(labels, label)
	}
	return bazel.MakeLabelListAttribute(bazel.MakeLabelList(labels))
}

// attributes common to blueprint_go_binary and bootstap_go_package
type goAttributes struct {
	Importpath             bazel.StringAttribute
	Srcs                   bazel.LabelListAttribute
	Deps                   bazel.LabelListAttribute
	Data                   bazel.LabelListAttribute
	Target_compatible_with bazel.LabelListAttribute

	// attributes for the dynamically generated go_test target
	Embed bazel.LabelListAttribute
}

type goTestProperties struct {
	name           string
	dir            string
	testSrcs       []string
	linuxTestSrcs  []string
	darwinTestSrcs []string
	testData       []string
	// Name of the target that should be compiled together with the test
	embedName string
}

// Creates a go_test target for bootstrap_go_package / blueprint_go_binary
func generateBazelTargetsGoTest(ctx *android.Context, goModulesMap nameToGoLibraryModule, gp goTestProperties) (BazelTarget, error) {
	ca := android.CommonAttributes{
		Name: gp.name,
	}
	ga := goAttributes{
		Srcs: goSrcLabels(ctx.Config(), gp.dir, gp.testSrcs, gp.linuxTestSrcs, gp.darwinTestSrcs),
		Data: goSrcLabels(ctx.Config(), gp.dir, gp.testData, []string{}, []string{}),
		Embed: bazel.MakeLabelListAttribute(
			bazel.MakeLabelList(
				[]bazel.Label{bazel.Label{Label: ":" + gp.embedName}},
			),
		),
		Target_compatible_with: targetNotCompatibleWithAndroid(),
	}

	libTest := bTarget{
		targetName:            gp.name,
		targetPackage:         gp.dir,
		bazelRuleClass:        "go_test",
		bazelRuleLoadLocation: "@io_bazel_rules_go//go:def.bzl",
		bazelAttributes:       []interface{}{&ca, &ga},
	}
	return generateBazelTarget(ctx, libTest)
}

// TODO - b/288491147: testSrcs of certain bootstrap_go_package/blueprint_go_binary are not hermetic and depend on
// testdata checked into the filesystem.
// Denylist the generation of go_test targets for these Soong modules.
// The go_library/go_binary will still be generated, since those are hermitic.
var (
	goTestsDenylist = []string{
		"android-archive-zip",
		"bazel_notice_gen",
		"blueprint-bootstrap-bpdoc",
		"blueprint-microfactory",
		"blueprint-pathtools",
		"bssl_ar",
		"compliance_checkmetadata",
		"compliance_checkshare",
		"compliance_dumpgraph",
		"compliance_dumpresolutions",
		"compliance_listshare",
		"compliance-module",
		"compliancenotice_bom",
		"compliancenotice_shippedlibs",
		"compliance_rtrace",
		"compliance_sbom",
		"golang-protobuf-internal-fuzz-jsonfuzz",
		"golang-protobuf-internal-fuzz-textfuzz",
		"golang-protobuf-internal-fuzz-wirefuzz",
		"htmlnotice",
		"protoc-gen-go",
		"rbcrun-module",
		"spdx-tools-builder",
		"spdx-tools-builder2v1",
		"spdx-tools-builder2v2",
		"spdx-tools-builder2v3",
		"spdx-tools-idsearcher",
		"spdx-tools-spdx-json",
		"spdx-tools-utils",
		"soong-ui-build",
		"textnotice",
		"xmlnotice",
	}
)

func testOfGoPackageIsIncompatible(g *bootstrap.GoPackage) bool {
	return android.InList(g.Name(), goTestsDenylist) ||
		// Denylist tests of soong_build
		// Theses tests have a guard that prevent usage outside a test environment
		// The guard (`ensureTestOnly`) looks for a `-test` in os.Args, which is present in soong's gotestrunner, but missing in `b test`
		g.IsPluginFor("soong_build") ||
		// soong-android is a dep of soong_build
		// This dependency is created by soong_build by listing it in its deps explicitly in Android.bp, and not via `plugin_for` in `soong-android`
		g.Name() == "soong-android"
}

func testOfGoBinaryIsIncompatible(g *bootstrap.GoBinary) bool {
	return android.InList(g.Name(), goTestsDenylist)
}

func generateBazelTargetsGoPackage(ctx *android.Context, g *bootstrap.GoPackage, goModulesMap nameToGoLibraryModule) ([]BazelTarget, []error) {
	ca := android.CommonAttributes{
		Name: g.Name(),
	}

	// For this bootstrap_go_package dep chain,
	// A --> B --> C ( ---> depends on)
	// Soong provides the convenience of only listing B as deps of A even if a src file of A imports C
	// Bazel OTOH
	// 1. requires C to be listed in `deps` expllicity.
	// 2. does not require C to be listed if src of A does not import C
	//
	// bp2build does not have sufficient info on whether C is a direct dep of A or not, so for now collect all transitive deps and add them to deps
	transitiveDeps := transitiveGoDeps(g.Deps(), goModulesMap)

	ga := goAttributes{
		Importpath: bazel.StringAttribute{
			Value: proptools.StringPtr(g.GoPkgPath()),
		},
		Srcs: goSrcLabels(ctx.Config(), ctx.ModuleDir(g), g.Srcs(), g.LinuxSrcs(), g.DarwinSrcs()),
		Deps: goDepLabels(
			android.FirstUniqueStrings(transitiveDeps),
			goModulesMap,
		),
		Target_compatible_with: targetNotCompatibleWithAndroid(),
	}

	lib := bTarget{
		targetName:            g.Name(),
		targetPackage:         ctx.ModuleDir(g),
		bazelRuleClass:        "go_library",
		bazelRuleLoadLocation: "@io_bazel_rules_go//go:def.bzl",
		bazelAttributes:       []interface{}{&ca, &ga},
	}
	retTargets := []BazelTarget{}
	var retErrs []error
	if libTarget, err := generateBazelTarget(ctx, lib); err == nil {
		retTargets = append(retTargets, libTarget)
	} else {
		retErrs = []error{err}
	}

	// If the library contains test srcs, create an additional go_test target
	if !testOfGoPackageIsIncompatible(g) && (len(g.TestSrcs()) > 0 || len(g.LinuxTestSrcs()) > 0 || len(g.DarwinTestSrcs()) > 0) {
		gp := goTestProperties{
			name:           g.Name() + "-test",
			dir:            ctx.ModuleDir(g),
			testSrcs:       g.TestSrcs(),
			linuxTestSrcs:  g.LinuxTestSrcs(),
			darwinTestSrcs: g.DarwinTestSrcs(),
			testData:       g.TestData(),
			embedName:      g.Name(), // embed the source go_library in the test so that its .go files are included in the compilation unit
		}
		if libTestTarget, err := generateBazelTargetsGoTest(ctx, goModulesMap, gp); err == nil {
			retTargets = append(retTargets, libTestTarget)
		} else {
			retErrs = append(retErrs, err)
		}
	}

	return retTargets, retErrs
}

type goLibraryModule struct {
	Dir  string
	Deps []string
}

type buildConversionMetadata struct {
	nameToGoLibraryModule nameToGoLibraryModule
	ndkHeaders            []blueprint.Module
}

type nameToGoLibraryModule map[string]goLibraryModule

// Visit each module in the graph, and collect metadata about the build graph
// If a module is of type `bootstrap_go_package`, return a map containing metadata like its dir and deps
// If a module is of type `ndk_headers`, add it to a list and return the list
func createBuildConversionMetadata(ctx *android.Context) buildConversionMetadata {
	goMap := nameToGoLibraryModule{}
	ndkHeaders := []blueprint.Module{}
	ctx.VisitAllModules(func(m blueprint.Module) {
		moduleType := ctx.ModuleType(m)
		// We do not need to store information about blueprint_go_binary since it does not have any rdeps
		if moduleType == "bootstrap_go_package" {
			goMap[m.Name()] = goLibraryModule{
				Dir:  ctx.ModuleDir(m),
				Deps: m.(*bootstrap.GoPackage).Deps(),
			}
		} else if moduleType == "ndk_headers" || moduleType == "versioned_ndk_headers" {
			ndkHeaders = append(ndkHeaders, m)
		}
	})
	return buildConversionMetadata{
		nameToGoLibraryModule: goMap,
		ndkHeaders:            ndkHeaders,
	}
}

// Returns the deps in the transitive closure of a go target
func transitiveGoDeps(directDeps []string, goModulesMap nameToGoLibraryModule) []string {
	allDeps := directDeps
	i := 0
	for i < len(allDeps) {
		curr := allDeps[i]
		allDeps = append(allDeps, goModulesMap[curr].Deps...)
		i += 1
	}
	allDeps = android.SortedUniqueStrings(allDeps)
	return allDeps
}

func generateBazelTargetsGoBinary(ctx *android.Context, g *bootstrap.GoBinary, goModulesMap nameToGoLibraryModule) ([]BazelTarget, []error) {
	ca := android.CommonAttributes{
		Name: g.Name(),
	}

	retTargets := []BazelTarget{}
	var retErrs []error

	// For this bootstrap_go_package dep chain,
	// A --> B --> C ( ---> depends on)
	// Soong provides the convenience of only listing B as deps of A even if a src file of A imports C
	// Bazel OTOH
	// 1. requires C to be listed in `deps` expllicity.
	// 2. does not require C to be listed if src of A does not import C
	//
	// bp2build does not have sufficient info on whether C is a direct dep of A or not, so for now collect all transitive deps and add them to deps
	transitiveDeps := transitiveGoDeps(g.Deps(), goModulesMap)

	goSource := ""
	// If the library contains test srcs, create an additional go_test target
	// The go_test target will embed a go_source containining the source .go files it tests
	if !testOfGoBinaryIsIncompatible(g) && (len(g.TestSrcs()) > 0 || len(g.LinuxTestSrcs()) > 0 || len(g.DarwinTestSrcs()) > 0) {
		// Create a go_source containing the source .go files of go_library
		// This target will be an `embed` of the go_binary and go_test
		goSource = g.Name() + "-source"
		ca := android.CommonAttributes{
			Name: goSource,
		}
		ga := goAttributes{
			Srcs:                   goSrcLabels(ctx.Config(), ctx.ModuleDir(g), g.Srcs(), g.LinuxSrcs(), g.DarwinSrcs()),
			Deps:                   goDepLabels(transitiveDeps, goModulesMap),
			Target_compatible_with: targetNotCompatibleWithAndroid(),
		}
		libTestSource := bTarget{
			targetName:            goSource,
			targetPackage:         ctx.ModuleDir(g),
			bazelRuleClass:        "go_source",
			bazelRuleLoadLocation: "@io_bazel_rules_go//go:def.bzl",
			bazelAttributes:       []interface{}{&ca, &ga},
		}
		if libSourceTarget, err := generateBazelTarget(ctx, libTestSource); err == nil {
			retTargets = append(retTargets, libSourceTarget)
		} else {
			retErrs = append(retErrs, err)
		}

		// Create a go_test target
		gp := goTestProperties{
			name:           g.Name() + "-test",
			dir:            ctx.ModuleDir(g),
			testSrcs:       g.TestSrcs(),
			linuxTestSrcs:  g.LinuxTestSrcs(),
			darwinTestSrcs: g.DarwinTestSrcs(),
			testData:       g.TestData(),
			// embed the go_source in the test
			embedName: g.Name() + "-source",
		}
		if libTestTarget, err := generateBazelTargetsGoTest(ctx, goModulesMap, gp); err == nil {
			retTargets = append(retTargets, libTestTarget)
		} else {
			retErrs = append(retErrs, err)
		}

	}

	// Create a go_binary target
	ga := goAttributes{
		Deps:                   goDepLabels(transitiveDeps, goModulesMap),
		Target_compatible_with: targetNotCompatibleWithAndroid(),
	}

	// If the binary has testSrcs, embed the common `go_source`
	if goSource != "" {
		ga.Embed = bazel.MakeLabelListAttribute(
			bazel.MakeLabelList(
				[]bazel.Label{bazel.Label{Label: ":" + goSource}},
			),
		)
	} else {
		ga.Srcs = goSrcLabels(ctx.Config(), ctx.ModuleDir(g), g.Srcs(), g.LinuxSrcs(), g.DarwinSrcs())
	}

	bin := bTarget{
		targetName:            g.Name(),
		targetPackage:         ctx.ModuleDir(g),
		bazelRuleClass:        "go_binary",
		bazelRuleLoadLocation: "@io_bazel_rules_go//go:def.bzl",
		bazelAttributes:       []interface{}{&ca, &ga},
	}

	if binTarget, err := generateBazelTarget(ctx, bin); err == nil {
		retTargets = append(retTargets, binTarget)
	} else {
		retErrs = []error{err}
	}

	return retTargets, retErrs
}

func GenerateBazelTargets(ctx *CodegenContext, generateFilegroups bool) (conversionResults, []error) {
	ctx.Context().BeginEvent("GenerateBazelTargets")
	defer ctx.Context().EndEvent("GenerateBazelTargets")
	buildFileToTargets := make(map[string]BazelTargets)

	// Simple metrics tracking for bp2build
	metrics := CreateCodegenMetrics()

	dirs := make(map[string]bool)

	var errs []error

	// Visit go libraries in a pre-run and store its state in a map
	// The time complexity remains O(N), and this does not add significant wall time.
	meta := createBuildConversionMetadata(ctx.Context())
	nameToGoLibMap := meta.nameToGoLibraryModule
	ndkHeaders := meta.ndkHeaders

	bpCtx := ctx.Context()
	bpCtx.VisitAllModules(func(m blueprint.Module) {
		dir := bpCtx.ModuleDir(m)
		moduleType := bpCtx.ModuleType(m)
		dirs[dir] = true

		var targets []BazelTarget
		var targetErrs []error

		switch ctx.Mode() {
		case Bp2Build:
			if aModule, ok := m.(android.Module); ok {
				reason := aModule.GetUnconvertedReason()
				if reason != nil {
					// If this module was force-enabled, cause an error.
					if _, ok := ctx.Config().BazelModulesForceEnabledByFlag()[m.Name()]; ok && m.Name() != "" {
						err := fmt.Errorf("Force Enabled Module %s not converted", m.Name())
						errs = append(errs, err)
					}

					// Log the module isn't to be converted by bp2build.
					// TODO: b/291598248 - Log handcrafted modules differently than other unconverted modules.
					metrics.AddUnconvertedModule(m, moduleType, dir, *reason)
					return
				}
				if len(aModule.Bp2buildTargets()) == 0 {
					panic(fmt.Errorf("illegal bp2build invariant: module '%s' was neither converted nor marked unconvertible", aModule.Name()))
				}

				// Handle modules converted to generated targets.
				targets, targetErrs = generateBazelTargets(bpCtx, aModule)
				errs = append(errs, targetErrs...)
				for _, t := range targets {
					// A module can potentially generate more than 1 Bazel
					// target, each of a different rule class.
					metrics.IncrementRuleClassCount(t.ruleClass)
				}

				// Log the module.
				metrics.AddConvertedModule(aModule, moduleType, dir)

				// Handle modules with unconverted deps. By default, emit a warning.
				if unconvertedDeps := aModule.GetUnconvertedBp2buildDeps(); len(unconvertedDeps) > 0 {
					msg := fmt.Sprintf("%s %s:%s depends on unconverted modules: %s",
						moduleType, bpCtx.ModuleDir(m), m.Name(), strings.Join(unconvertedDeps, ", "))
					switch ctx.unconvertedDepMode {
					case warnUnconvertedDeps:
						metrics.moduleWithUnconvertedDepsMsgs = append(metrics.moduleWithUnconvertedDepsMsgs, msg)
					case errorModulesUnconvertedDeps:
						errs = append(errs, fmt.Errorf(msg))
						return
					}
				}
				if unconvertedDeps := aModule.GetMissingBp2buildDeps(); len(unconvertedDeps) > 0 {
					msg := fmt.Sprintf("%s %s:%s depends on missing modules: %s",
						moduleType, bpCtx.ModuleDir(m), m.Name(), strings.Join(unconvertedDeps, ", "))
					switch ctx.unconvertedDepMode {
					case warnUnconvertedDeps:
						metrics.moduleWithMissingDepsMsgs = append(metrics.moduleWithMissingDepsMsgs, msg)
					case errorModulesUnconvertedDeps:
						errs = append(errs, fmt.Errorf(msg))
						return
					}
				}
			} else if glib, ok := m.(*bootstrap.GoPackage); ok {
				targets, targetErrs = generateBazelTargetsGoPackage(bpCtx, glib, nameToGoLibMap)
				errs = append(errs, targetErrs...)
				metrics.IncrementRuleClassCount("bootstrap_go_package")
				metrics.AddConvertedModule(glib, "bootstrap_go_package", dir)
			} else if gbin, ok := m.(*bootstrap.GoBinary); ok {
				targets, targetErrs = generateBazelTargetsGoBinary(bpCtx, gbin, nameToGoLibMap)
				errs = append(errs, targetErrs...)
				metrics.IncrementRuleClassCount("blueprint_go_binary")
				metrics.AddConvertedModule(gbin, "blueprint_go_binary", dir)
			} else {
				metrics.AddUnconvertedModule(m, moduleType, dir, android.UnconvertedReason{
					ReasonType: int(bp2build_metrics_proto.UnconvertedReasonType_TYPE_UNSUPPORTED),
				})
				return
			}
		case QueryView:
			// Blocklist certain module types from being generated.
			if canonicalizeModuleType(bpCtx.ModuleType(m)) == "package" {
				// package module name contain slashes, and thus cannot
				// be mapped cleanly to a bazel label.
				return
			}
			t, err := generateSoongModuleTarget(bpCtx, m)
			if err != nil {
				errs = append(errs, err)
			}
			targets = append(targets, t)
		default:
			errs = append(errs, fmt.Errorf("Unknown code-generation mode: %s", ctx.Mode()))
			return
		}

		for _, target := range targets {
			targetDir := target.PackageName()
			buildFileToTargets[targetDir] = append(buildFileToTargets[targetDir], target)
		}
	})

	// Create an ndk_sysroot target that has a dependency edge on every target corresponding to Soong's ndk_headers
	// This root target will provide headers to sdk variants of jni libraries
	if ctx.Mode() == Bp2Build {
		var depLabels bazel.LabelList
		for _, ndkHeader := range ndkHeaders {
			depLabel := bazel.Label{
				Label: "//" + bpCtx.ModuleDir(ndkHeader) + ":" + ndkHeader.Name(),
			}
			depLabels.Add(&depLabel)
		}
		a := struct {
			Deps                bazel.LabelListAttribute
			System_dynamic_deps bazel.LabelListAttribute
		}{
			Deps:                bazel.MakeLabelListAttribute(bazel.UniqueSortedBazelLabelList(depLabels)),
			System_dynamic_deps: bazel.MakeLabelListAttribute(bazel.MakeLabelList([]bazel.Label{})),
		}
		ndkSysroot := bTarget{
			targetName:            "ndk_sysroot",
			targetPackage:         "build/bazel/rules/cc", // The location is subject to change, use build/bazel for now
			bazelRuleClass:        "cc_library_headers",
			bazelRuleLoadLocation: "//build/bazel/rules/cc:cc_library_headers.bzl",
			bazelAttributes:       []interface{}{&a},
		}

		if t, err := generateBazelTarget(bpCtx, ndkSysroot); err == nil {
			dir := ndkSysroot.targetPackage
			buildFileToTargets[dir] = append(buildFileToTargets[dir], t)
		} else {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return conversionResults{}, errs
	}

	if generateFilegroups {
		// Add a filegroup target that exposes all sources in the subtree of this package
		// NOTE: This also means we generate a BUILD file for every Android.bp file (as long as it has at least one module)
		//
		// This works because: https://bazel.build/reference/be/functions#exports_files
		// "As a legacy behaviour, also files mentioned as input to a rule are exported with the
		// default visibility until the flag --incompatible_no_implicit_file_export is flipped. However, this behavior
		// should not be relied upon and actively migrated away from."
		//
		// TODO(b/198619163): We should change this to export_files(glob(["**/*"])) instead, but doing that causes these errors:
		// "Error in exports_files: generated label '//external/avb:avbtool' conflicts with existing py_binary rule"
		// So we need to solve all the "target ... is both a rule and a file" warnings first.
		for dir := range dirs {
			buildFileToTargets[dir] = append(buildFileToTargets[dir], BazelTarget{
				name:      "bp2build_all_srcs",
				content:   `filegroup(name = "bp2build_all_srcs", srcs = glob(["**/*"]), tags = ["manual"])`,
				ruleClass: "filegroup",
			})
		}
	}

	return conversionResults{
		buildFileToTargets: buildFileToTargets,
		metrics:            metrics,
	}, errs
}

func generateBazelTargets(ctx bpToBuildContext, m android.Module) ([]BazelTarget, []error) {
	var targets []BazelTarget
	var errs []error
	for _, m := range m.Bp2buildTargets() {
		target, err := generateBazelTarget(ctx, m)
		if err != nil {
			errs = append(errs, err)
			return targets, errs
		}
		targets = append(targets, target)
	}
	return targets, errs
}

type bp2buildModule interface {
	TargetName() string
	TargetPackage() string
	BazelRuleClass() string
	BazelRuleLoadLocation() string
	BazelAttributes() []interface{}
}

func generateBazelTarget(ctx bpToBuildContext, m bp2buildModule) (BazelTarget, error) {
	ruleClass := m.BazelRuleClass()
	bzlLoadLocation := m.BazelRuleLoadLocation()

	// extract the bazel attributes from the module.
	attrs := m.BazelAttributes()
	props, err := extractModuleProperties(attrs, true)
	if err != nil {
		return BazelTarget{}, err
	}

	// name is handled in a special manner
	delete(props.Attrs, "name")

	// Return the Bazel target with rule class and attributes, ready to be
	// code-generated.
	attributes := propsToAttributes(props.Attrs)
	var content string
	targetName := m.TargetName()
	if targetName != "" {
		content = fmt.Sprintf(ruleTargetTemplate, ruleClass, targetName, attributes)
	} else {
		content = fmt.Sprintf(unnamedRuleTargetTemplate, ruleClass, attributes)
	}
	var loads []BazelLoad
	if bzlLoadLocation != "" {
		loads = append(loads, BazelLoad{
			file:    bzlLoadLocation,
			symbols: []BazelLoadSymbol{{symbol: ruleClass}},
		})
	}
	return BazelTarget{
		name:        targetName,
		packageName: m.TargetPackage(),
		ruleClass:   ruleClass,
		loads:       loads,
		content:     content,
	}, nil
}

// Convert a module and its deps and props into a Bazel macro/rule
// representation in the BUILD file.
func generateSoongModuleTarget(ctx bpToBuildContext, m blueprint.Module) (BazelTarget, error) {
	props, err := getBuildProperties(ctx, m)

	// TODO(b/163018919): DirectDeps can have duplicate (module, variant)
	// items, if the modules are added using different DependencyTag. Figure
	// out the implications of that.
	depLabels := map[string]bool{}
	if aModule, ok := m.(android.Module); ok {
		ctx.VisitDirectDeps(aModule, func(depModule blueprint.Module) {
			depLabels[qualifiedTargetLabel(ctx, depModule)] = true
		})
	}

	for p := range ignoredPropNames {
		delete(props.Attrs, p)
	}
	attributes := propsToAttributes(props.Attrs)

	depLabelList := "[\n"
	for depLabel := range depLabels {
		depLabelList += fmt.Sprintf("        %q,\n", depLabel)
	}
	depLabelList += "    ]"

	targetName := targetNameWithVariant(ctx, m)
	return BazelTarget{
		name:        targetName,
		packageName: ctx.ModuleDir(m),
		content: fmt.Sprintf(
			soongModuleTargetTemplate,
			targetName,
			ctx.ModuleName(m),
			canonicalizeModuleType(ctx.ModuleType(m)),
			ctx.ModuleSubDir(m),
			depLabelList,
			attributes),
	}, err
}

func getBuildProperties(ctx bpToBuildContext, m blueprint.Module) (BazelAttributes, error) {
	// TODO: this omits properties for blueprint modules (blueprint_go_binary,
	// bootstrap_go_binary, bootstrap_go_package), which will have to be handled separately.
	if aModule, ok := m.(android.Module); ok {
		return extractModuleProperties(aModule.GetProperties(), false)
	}

	return BazelAttributes{}, nil
}

// Generically extract module properties and types into a map, keyed by the module property name.
func extractModuleProperties(props []interface{}, checkForDuplicateProperties bool) (BazelAttributes, error) {
	ret := map[string]string{}

	// Iterate over this android.Module's property structs.
	for _, properties := range props {
		propertiesValue := reflect.ValueOf(properties)
		// Check that propertiesValue is a pointer to the Properties struct, like
		// *cc.BaseLinkerProperties or *java.CompilerProperties.
		//
		// propertiesValue can also be type-asserted to the structs to
		// manipulate internal props, if needed.
		if isStructPtr(propertiesValue.Type()) {
			structValue := propertiesValue.Elem()
			ok, err := extractStructProperties(structValue, 0)
			if err != nil {
				return BazelAttributes{}, err
			}
			for k, v := range ok {
				if existing, exists := ret[k]; checkForDuplicateProperties && exists {
					return BazelAttributes{}, fmt.Errorf(
						"%s (%v) is present in properties whereas it should be consolidated into a commonAttributes",
						k, existing)
				}
				ret[k] = v
			}
		} else {
			return BazelAttributes{},
				fmt.Errorf(
					"properties must be a pointer to a struct, got %T",
					propertiesValue.Interface())
		}
	}

	return BazelAttributes{
		Attrs: ret,
	}, nil
}

func isStructPtr(t reflect.Type) bool {
	return t.Kind() == reflect.Ptr && t.Elem().Kind() == reflect.Struct
}

// prettyPrint a property value into the equivalent Starlark representation
// recursively.
func prettyPrint(propertyValue reflect.Value, indent int, emitZeroValues bool) (string, error) {
	if !emitZeroValues && isZero(propertyValue) {
		// A property value being set or unset actually matters -- Soong does set default
		// values for unset properties, like system_shared_libs = ["libc", "libm", "libdl"] at
		// https://cs.android.com/android/platform/superproject/+/master:build/soong/cc/linker.go;l=281-287;drc=f70926eef0b9b57faf04c17a1062ce50d209e480
		//
		// In Bazel-parlance, we would use "attr.<type>(default = <default
		// value>)" to set the default value of unset attributes. In the cases
		// where the bp2build converter didn't set the default value within the
		// mutator when creating the BazelTargetModule, this would be a zero
		// value. For those cases, we return an empty string so we don't
		// unnecessarily generate empty values.
		return "", nil
	}

	switch propertyValue.Kind() {
	case reflect.String:
		return fmt.Sprintf("\"%v\"", escapeString(propertyValue.String())), nil
	case reflect.Bool:
		return starlark_fmt.PrintBool(propertyValue.Bool()), nil
	case reflect.Int, reflect.Uint, reflect.Int64:
		return fmt.Sprintf("%v", propertyValue.Interface()), nil
	case reflect.Ptr:
		return prettyPrint(propertyValue.Elem(), indent, emitZeroValues)
	case reflect.Slice:
		elements := make([]string, 0, propertyValue.Len())
		for i := 0; i < propertyValue.Len(); i++ {
			val, err := prettyPrint(propertyValue.Index(i), indent, emitZeroValues)
			if err != nil {
				return "", err
			}
			if val != "" {
				elements = append(elements, val)
			}
		}
		return starlark_fmt.PrintList(elements, indent, func(s string) string {
			return "%s"
		}), nil

	case reflect.Struct:
		// Special cases where the bp2build sends additional information to the codegenerator
		// by wrapping the attributes in a custom struct type.
		if attr, ok := propertyValue.Interface().(bazel.Attribute); ok {
			return prettyPrintAttribute(attr, indent)
		} else if label, ok := propertyValue.Interface().(bazel.Label); ok {
			return fmt.Sprintf("%q", label.Label), nil
		}

		// Sort and print the struct props by the key.
		structProps, err := extractStructProperties(propertyValue, indent)

		if err != nil {
			return "", err
		}

		if len(structProps) == 0 {
			return "", nil
		}
		return starlark_fmt.PrintDict(structProps, indent), nil
	case reflect.Interface:
		// TODO(b/164227191): implement pretty print for interfaces.
		// Interfaces are used for for arch, multilib and target properties.
		return "", nil
	case reflect.Map:
		if v, ok := propertyValue.Interface().(bazel.StringMapAttribute); ok {
			return starlark_fmt.PrintStringStringDict(v, indent), nil
		}
		return "", fmt.Errorf("bp2build expects map of type map[string]string for field: %s", propertyValue)
	default:
		return "", fmt.Errorf(
			"unexpected kind for property struct field: %s", propertyValue.Kind())
	}
}

// Converts a reflected property struct value into a map of property names and property values,
// which each property value correctly pretty-printed and indented at the right nest level,
// since property structs can be nested. In Starlark, nested structs are represented as nested
// dicts: https://docs.bazel.build/skylark/lib/dict.html
func extractStructProperties(structValue reflect.Value, indent int) (map[string]string, error) {
	if structValue.Kind() != reflect.Struct {
		return map[string]string{}, fmt.Errorf("Expected a reflect.Struct type, but got %s", structValue.Kind())
	}

	var err error

	ret := map[string]string{}
	structType := structValue.Type()
	for i := 0; i < structValue.NumField(); i++ {
		field := structType.Field(i)
		if shouldSkipStructField(field) {
			continue
		}

		fieldValue := structValue.Field(i)
		if isZero(fieldValue) {
			// Ignore zero-valued fields
			continue
		}

		// if the struct is embedded (anonymous), flatten the properties into the containing struct
		if field.Anonymous {
			if field.Type.Kind() == reflect.Ptr {
				fieldValue = fieldValue.Elem()
			}
			if fieldValue.Type().Kind() == reflect.Struct {
				propsToMerge, err := extractStructProperties(fieldValue, indent)
				if err != nil {
					return map[string]string{}, err
				}
				for prop, value := range propsToMerge {
					ret[prop] = value
				}
				continue
			}
		}

		propertyName := proptools.PropertyNameForField(field.Name)
		var prettyPrintedValue string
		prettyPrintedValue, err = prettyPrint(fieldValue, indent+1, false)
		if err != nil {
			return map[string]string{}, fmt.Errorf(
				"Error while parsing property: %q. %s",
				propertyName,
				err)
		}
		if prettyPrintedValue != "" {
			ret[propertyName] = prettyPrintedValue
		}
	}

	return ret, nil
}

func isZero(value reflect.Value) bool {
	switch value.Kind() {
	case reflect.Func, reflect.Map, reflect.Slice:
		return value.IsNil()
	case reflect.Array:
		valueIsZero := true
		for i := 0; i < value.Len(); i++ {
			valueIsZero = valueIsZero && isZero(value.Index(i))
		}
		return valueIsZero
	case reflect.Struct:
		valueIsZero := true
		for i := 0; i < value.NumField(); i++ {
			valueIsZero = valueIsZero && isZero(value.Field(i))
		}
		return valueIsZero
	case reflect.Ptr:
		if !value.IsNil() {
			return isZero(reflect.Indirect(value))
		} else {
			return true
		}
	// Always print bool/strings, if you want a bool/string attribute to be able to take the default value, use a
	// pointer instead
	case reflect.Bool, reflect.String:
		return false
	default:
		if !value.IsValid() {
			return true
		}
		zeroValue := reflect.Zero(value.Type())
		result := value.Interface() == zeroValue.Interface()
		return result
	}
}

func escapeString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")

	// b/184026959: Reverse the application of some common control sequences.
	// These must be generated literally in the BUILD file.
	s = strings.ReplaceAll(s, "\t", "\\t")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")

	return strings.ReplaceAll(s, "\"", "\\\"")
}

func targetNameWithVariant(c bpToBuildContext, logicModule blueprint.Module) string {
	name := ""
	if c.ModuleSubDir(logicModule) != "" {
		// TODO(b/162720883): Figure out a way to drop the "--" variant suffixes.
		name = c.ModuleName(logicModule) + "--" + c.ModuleSubDir(logicModule)
	} else {
		name = c.ModuleName(logicModule)
	}

	return strings.Replace(name, "//", "", 1)
}

func qualifiedTargetLabel(c bpToBuildContext, logicModule blueprint.Module) string {
	return fmt.Sprintf("//%s:%s", c.ModuleDir(logicModule), targetNameWithVariant(c, logicModule))
}
