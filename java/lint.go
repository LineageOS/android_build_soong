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

package java

import (
	"fmt"
	"sort"

	"android/soong/android"
)

type LintProperties struct {
	// Controls for running Android Lint on the module.
	Lint struct {

		// If true, run Android Lint on the module.  Defaults to true.
		Enabled *bool

		// Flags to pass to the Android Lint tool.
		Flags []string

		// Checks that should be treated as fatal.
		Fatal_checks []string

		// Checks that should be treated as errors.
		Error_checks []string

		// Checks that should be treated as warnings.
		Warning_checks []string

		// Checks that should be skipped.
		Disabled_checks []string

		// Modules that provide extra lint checks
		Extra_check_modules []string
	}
}

type linter struct {
	name                string
	manifest            android.Path
	mergedManifest      android.Path
	srcs                android.Paths
	srcJars             android.Paths
	resources           android.Paths
	classpath           android.Paths
	classes             android.Path
	extraLintCheckJars  android.Paths
	test                bool
	library             bool
	minSdkVersion       string
	targetSdkVersion    string
	compileSdkVersion   string
	javaLanguageLevel   string
	kotlinLanguageLevel string
	outputs             lintOutputs
	properties          LintProperties
}

type lintOutputs struct {
	html android.ModuleOutPath
	text android.ModuleOutPath
	xml  android.ModuleOutPath
}

func (l *linter) enabled() bool {
	return BoolDefault(l.properties.Lint.Enabled, true)
}

func (l *linter) deps(ctx android.BottomUpMutatorContext) {
	if !l.enabled() {
		return
	}

	ctx.AddFarVariationDependencies(ctx.Config().BuildOSCommonTarget.Variations(), extraLintCheckTag, l.properties.Lint.Extra_check_modules...)
}

func (l *linter) writeLintProjectXML(ctx android.ModuleContext,
	rule *android.RuleBuilder) (projectXMLPath, configXMLPath, cacheDir, homeDir android.WritablePath, deps android.Paths) {

	var resourcesList android.WritablePath
	if len(l.resources) > 0 {
		// The list of resources may be too long to put on the command line, but
		// we can't use the rsp file because it is already being used for srcs.
		// Insert a second rule to write out the list of resources to a file.
		resourcesList = android.PathForModuleOut(ctx, "lint", "resources.list")
		resListRule := android.NewRuleBuilder()
		resListRule.Command().Text("cp").FlagWithRspFileInputList("", l.resources).Output(resourcesList)
		resListRule.Build(pctx, ctx, "lint_resources_list", "lint resources list")
		deps = append(deps, l.resources...)
	}

	projectXMLPath = android.PathForModuleOut(ctx, "lint", "project.xml")
	// Lint looks for a lint.xml file next to the project.xml file, give it one.
	configXMLPath = android.PathForModuleOut(ctx, "lint", "lint.xml")
	cacheDir = android.PathForModuleOut(ctx, "lint", "cache")
	homeDir = android.PathForModuleOut(ctx, "lint", "home")

	srcJarDir := android.PathForModuleOut(ctx, "lint-srcjars")
	srcJarList := zipSyncCmd(ctx, rule, srcJarDir, l.srcJars)

	cmd := rule.Command().
		BuiltTool(ctx, "lint-project-xml").
		FlagWithOutput("--project_out ", projectXMLPath).
		FlagWithOutput("--config_out ", configXMLPath).
		FlagWithArg("--name ", ctx.ModuleName())

	if l.library {
		cmd.Flag("--library")
	}
	if l.test {
		cmd.Flag("--test")
	}
	if l.manifest != nil {
		deps = append(deps, l.manifest)
		cmd.FlagWithArg("--manifest ", l.manifest.String())
	}
	if l.mergedManifest != nil {
		deps = append(deps, l.mergedManifest)
		cmd.FlagWithArg("--merged_manifest ", l.mergedManifest.String())
	}

	// TODO(ccross): some of the files in l.srcs are generated sources and should be passed to
	// lint separately.
	cmd.FlagWithRspFileInputList("--srcs ", l.srcs)
	deps = append(deps, l.srcs...)

	cmd.FlagWithInput("--generated_srcs ", srcJarList)
	deps = append(deps, l.srcJars...)

	if resourcesList != nil {
		cmd.FlagWithInput("--resources ", resourcesList)
	}

	if l.classes != nil {
		deps = append(deps, l.classes)
		cmd.FlagWithArg("--classes ", l.classes.String())
	}

	cmd.FlagForEachArg("--classpath ", l.classpath.Strings())
	deps = append(deps, l.classpath...)

	cmd.FlagForEachArg("--extra_checks_jar ", l.extraLintCheckJars.Strings())
	deps = append(deps, l.extraLintCheckJars...)

	cmd.FlagWithArg("--root_dir ", "$PWD")

	// The cache tag in project.xml is relative to the root dir, or the project.xml file if
	// the root dir is not set.
	cmd.FlagWithArg("--cache_dir ", cacheDir.String())

	cmd.FlagWithInput("@",
		android.PathForSource(ctx, "build/soong/java/lint_defaults.txt"))

	cmd.FlagForEachArg("--disable_check ", l.properties.Lint.Disabled_checks)
	cmd.FlagForEachArg("--warning_check ", l.properties.Lint.Warning_checks)
	cmd.FlagForEachArg("--error_check ", l.properties.Lint.Error_checks)
	cmd.FlagForEachArg("--fatal_check ", l.properties.Lint.Fatal_checks)

	return projectXMLPath, configXMLPath, cacheDir, homeDir, deps
}

// generateManifest adds a command to the rule to write a dummy manifest cat contains the
// minSdkVersion and targetSdkVersion for modules (like java_library) that don't have a manifest.
func (l *linter) generateManifest(ctx android.ModuleContext, rule *android.RuleBuilder) android.Path {
	manifestPath := android.PathForModuleOut(ctx, "lint", "AndroidManifest.xml")

	rule.Command().Text("(").
		Text(`echo "<?xml version='1.0' encoding='utf-8'?>" &&`).
		Text(`echo "<manifest xmlns:android='http://schemas.android.com/apk/res/android'" &&`).
		Text(`echo "    android:versionCode='1' android:versionName='1' >" &&`).
		Textf(`echo "  <uses-sdk android:minSdkVersion='%s' android:targetSdkVersion='%s'/>" &&`,
			l.minSdkVersion, l.targetSdkVersion).
		Text(`echo "</manifest>"`).
		Text(") >").Output(manifestPath)

	return manifestPath
}

func (l *linter) lint(ctx android.ModuleContext) {
	if !l.enabled() {
		return
	}

	extraLintCheckModules := ctx.GetDirectDepsWithTag(extraLintCheckTag)
	for _, extraLintCheckModule := range extraLintCheckModules {
		if dep, ok := extraLintCheckModule.(Dependency); ok {
			l.extraLintCheckJars = append(l.extraLintCheckJars, dep.ImplementationAndResourcesJars()...)
		} else {
			ctx.PropertyErrorf("lint.extra_check_modules",
				"%s is not a java module", ctx.OtherModuleName(extraLintCheckModule))
		}
	}

	rule := android.NewRuleBuilder()

	if l.manifest == nil {
		manifest := l.generateManifest(ctx, rule)
		l.manifest = manifest
	}

	projectXML, lintXML, cacheDir, homeDir, deps := l.writeLintProjectXML(ctx, rule)

	l.outputs.html = android.PathForModuleOut(ctx, "lint-report.html")
	l.outputs.text = android.PathForModuleOut(ctx, "lint-report.txt")
	l.outputs.xml = android.PathForModuleOut(ctx, "lint-report.xml")

	rule.Command().Text("rm -rf").Flag(cacheDir.String()).Flag(homeDir.String())
	rule.Command().Text("mkdir -p").Flag(cacheDir.String()).Flag(homeDir.String())

	rule.Command().
		Text("(").
		Flag("JAVA_OPTS=-Xmx2048m").
		FlagWithArg("ANDROID_SDK_HOME=", homeDir.String()).
		FlagWithInput("SDK_ANNOTATIONS=", annotationsZipPath(ctx)).
		FlagWithInput("LINT_OPTS=-DLINT_API_DATABASE=", apiVersionsXmlPath(ctx)).
		Tool(android.PathForSource(ctx, "prebuilts/cmdline-tools/tools/bin/lint")).
		Implicit(android.PathForSource(ctx, "prebuilts/cmdline-tools/tools/lib/lint-classpath.jar")).
		Flag("--quiet").
		FlagWithInput("--project ", projectXML).
		FlagWithInput("--config ", lintXML).
		FlagWithOutput("--html ", l.outputs.html).
		FlagWithOutput("--text ", l.outputs.text).
		FlagWithOutput("--xml ", l.outputs.xml).
		FlagWithArg("--compile-sdk-version ", l.compileSdkVersion).
		FlagWithArg("--java-language-level ", l.javaLanguageLevel).
		FlagWithArg("--kotlin-language-level ", l.kotlinLanguageLevel).
		FlagWithArg("--url ", fmt.Sprintf(".=.,%s=out", android.PathForOutput(ctx).String())).
		Flag("--exitcode").
		Flags(l.properties.Lint.Flags).
		Implicits(deps).
		Text("|| (").Text("cat").Input(l.outputs.text).Text("; exit 7)").
		Text(")")

	rule.Command().Text("rm -rf").Flag(cacheDir.String()).Flag(homeDir.String())

	rule.Build(pctx, ctx, "lint", "lint")
}

func (l *linter) lintOutputs() *lintOutputs {
	return &l.outputs
}

type lintOutputIntf interface {
	lintOutputs() *lintOutputs
}

var _ lintOutputIntf = (*linter)(nil)

type lintSingleton struct {
	htmlZip android.WritablePath
	textZip android.WritablePath
	xmlZip  android.WritablePath
}

func (l *lintSingleton) GenerateBuildActions(ctx android.SingletonContext) {
	l.generateLintReportZips(ctx)
	l.copyLintDependencies(ctx)
}

func (l *lintSingleton) copyLintDependencies(ctx android.SingletonContext) {
	if ctx.Config().UnbundledBuild() {
		return
	}

	var frameworkDocStubs android.Module
	ctx.VisitAllModules(func(m android.Module) {
		if ctx.ModuleName(m) == "framework-doc-stubs" {
			if frameworkDocStubs == nil {
				frameworkDocStubs = m
			} else {
				ctx.Errorf("lint: multiple framework-doc-stubs modules found: %s and %s",
					ctx.ModuleSubDir(m), ctx.ModuleSubDir(frameworkDocStubs))
			}
		}
	})

	if frameworkDocStubs == nil {
		if !ctx.Config().AllowMissingDependencies() {
			ctx.Errorf("lint: missing framework-doc-stubs")
		}
		return
	}

	ctx.Build(pctx, android.BuildParams{
		Rule:   android.Cp,
		Input:  android.OutputFileForModule(ctx, frameworkDocStubs, ".annotations.zip"),
		Output: annotationsZipPath(ctx),
	})

	ctx.Build(pctx, android.BuildParams{
		Rule:   android.Cp,
		Input:  android.OutputFileForModule(ctx, frameworkDocStubs, ".api_versions.xml"),
		Output: apiVersionsXmlPath(ctx),
	})
}

func annotationsZipPath(ctx android.PathContext) android.WritablePath {
	return android.PathForOutput(ctx, "lint", "annotations.zip")
}

func apiVersionsXmlPath(ctx android.PathContext) android.WritablePath {
	return android.PathForOutput(ctx, "lint", "api_versions.xml")
}

func (l *lintSingleton) generateLintReportZips(ctx android.SingletonContext) {
	var outputs []*lintOutputs
	var dirs []string
	ctx.VisitAllModules(func(m android.Module) {
		if ctx.Config().EmbeddedInMake() && !m.ExportedToMake() {
			return
		}

		if apex, ok := m.(android.ApexModule); ok && apex.NotAvailableForPlatform() && apex.IsForPlatform() {
			// There are stray platform variants of modules in apexes that are not available for
			// the platform, and they sometimes can't be built.  Don't depend on them.
			return
		}

		if l, ok := m.(lintOutputIntf); ok {
			outputs = append(outputs, l.lintOutputs())
		}
	})

	dirs = android.SortedUniqueStrings(dirs)

	zip := func(outputPath android.WritablePath, get func(*lintOutputs) android.Path) {
		var paths android.Paths

		for _, output := range outputs {
			paths = append(paths, get(output))
		}

		sort.Slice(paths, func(i, j int) bool {
			return paths[i].String() < paths[j].String()
		})

		rule := android.NewRuleBuilder()

		rule.Command().BuiltTool(ctx, "soong_zip").
			FlagWithOutput("-o ", outputPath).
			FlagWithArg("-C ", android.PathForIntermediates(ctx).String()).
			FlagWithRspFileInputList("-l ", paths)

		rule.Build(pctx, ctx, outputPath.Base(), outputPath.Base())
	}

	l.htmlZip = android.PathForOutput(ctx, "lint-report-html.zip")
	zip(l.htmlZip, func(l *lintOutputs) android.Path { return l.html })

	l.textZip = android.PathForOutput(ctx, "lint-report-text.zip")
	zip(l.textZip, func(l *lintOutputs) android.Path { return l.text })

	l.xmlZip = android.PathForOutput(ctx, "lint-report-xml.zip")
	zip(l.xmlZip, func(l *lintOutputs) android.Path { return l.xml })

	ctx.Phony("lint-check", l.htmlZip, l.textZip, l.xmlZip)
}

func (l *lintSingleton) MakeVars(ctx android.MakeVarsContext) {
	ctx.DistForGoal("lint-check", l.htmlZip, l.textZip, l.xmlZip)
}

var _ android.SingletonMakeVarsProvider = (*lintSingleton)(nil)

func init() {
	android.RegisterSingletonType("lint",
		func() android.Singleton { return &lintSingleton{} })
}
