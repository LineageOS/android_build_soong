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

package java

import (
	"fmt"
	"io"
	"strings"

	"android/soong/android"
)

func init() {
	android.RegisterModuleType("android_robolectric_test", RobolectricTestFactory)
}

var robolectricDefaultLibs = []string{
	"robolectric_android-all-stub",
	"Robolectric_all-target",
	"mockito-robolectric-prebuilt",
	"truth-prebuilt",
}

var (
	roboCoverageLibsTag = dependencyTag{name: "roboSrcs"}
)

type robolectricProperties struct {
	// The name of the android_app module that the tests will run against.
	Instrumentation_for *string

	// Additional libraries for which coverage data should be generated
	Coverage_libs []string

	Test_options struct {
		// Timeout in seconds when running the tests.
		Timeout *int64
	}
}

type robolectricTest struct {
	Library

	robolectricProperties robolectricProperties

	libs []string

	roboSrcJar android.Path
}

func (r *robolectricTest) DepsMutator(ctx android.BottomUpMutatorContext) {
	r.Library.DepsMutator(ctx)

	if r.robolectricProperties.Instrumentation_for != nil {
		ctx.AddVariationDependencies(nil, instrumentationForTag, String(r.robolectricProperties.Instrumentation_for))
	} else {
		ctx.PropertyErrorf("instrumentation_for", "missing required instrumented module")
	}

	ctx.AddVariationDependencies(nil, libTag, robolectricDefaultLibs...)

	ctx.AddVariationDependencies(nil, roboCoverageLibsTag, r.robolectricProperties.Coverage_libs...)
}

func (r *robolectricTest) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	roboTestConfig := android.PathForModuleGen(ctx, "robolectric").
		Join(ctx, "com/android/tools/test_config.properties")

	// TODO: this inserts paths to built files into the test, it should really be inserting the contents.
	instrumented := ctx.GetDirectDepsWithTag(instrumentationForTag)

	if len(instrumented) != 1 {
		panic(fmt.Errorf("expected exactly 1 instrumented dependency, got %d", len(instrumented)))
	}

	instrumentedApp, ok := instrumented[0].(*AndroidApp)
	if !ok {
		ctx.PropertyErrorf("instrumentation_for", "dependency must be an android_app")
	}

	generateRoboTestConfig(ctx, roboTestConfig, instrumentedApp)
	r.extraResources = android.Paths{roboTestConfig}

	r.Library.GenerateAndroidBuildActions(ctx)

	roboSrcJar := android.PathForModuleGen(ctx, "robolectric", ctx.ModuleName()+".srcjar")
	r.generateRoboSrcJar(ctx, roboSrcJar, instrumentedApp)
	r.roboSrcJar = roboSrcJar

	for _, dep := range ctx.GetDirectDepsWithTag(libTag) {
		r.libs = append(r.libs, ctx.OtherModuleName(dep))
	}
}

func generateRoboTestConfig(ctx android.ModuleContext, outputFile android.WritablePath, instrumentedApp *AndroidApp) {
	manifest := instrumentedApp.mergedManifestFile
	resourceApk := instrumentedApp.outputFile

	rule := android.NewRuleBuilder()

	rule.Command().Text("rm -f").Output(outputFile)
	rule.Command().
		Textf(`echo "android_merged_manifest=%s" >>`, manifest.String()).Output(outputFile).Text("&&").
		Textf(`echo "android_resource_apk=%s" >>`, resourceApk.String()).Output(outputFile).
		// Make it depend on the files to which it points so the test file's timestamp is updated whenever the
		// contents change
		Implicit(manifest).
		Implicit(resourceApk)

	rule.Build(pctx, ctx, "generate_test_config", "generate test_config.properties")
}

func (r *robolectricTest) generateRoboSrcJar(ctx android.ModuleContext, outputFile android.WritablePath,
	instrumentedApp *AndroidApp) {

	srcJarArgs := copyOf(instrumentedApp.srcJarArgs)
	srcJarDeps := append(android.Paths(nil), instrumentedApp.srcJarDeps...)

	for _, m := range ctx.GetDirectDepsWithTag(roboCoverageLibsTag) {
		if dep, ok := m.(Dependency); ok {
			depSrcJarArgs, depSrcJarDeps := dep.SrcJarArgs()
			srcJarArgs = append(srcJarArgs, depSrcJarArgs...)
			srcJarDeps = append(srcJarDeps, depSrcJarDeps...)
		}
	}

	TransformResourcesToJar(ctx, outputFile, srcJarArgs, srcJarDeps)
}

func (r *robolectricTest) AndroidMk() android.AndroidMkData {
	data := r.Library.AndroidMk()

	data.Custom = func(w io.Writer, name, prefix, moduleDir string, data android.AndroidMkData) {
		android.WriteAndroidMkData(w, data)

		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "include $(CLEAR_VARS)")
		fmt.Fprintln(w, "LOCAL_MODULE := Run"+name)
		fmt.Fprintln(w, "LOCAL_JAVA_LIBRARIES :=", name)
		fmt.Fprintln(w, "LOCAL_JAVA_LIBRARIES += ", strings.Join(r.libs, " "))
		fmt.Fprintln(w, "LOCAL_TEST_PACKAGE :=", String(r.robolectricProperties.Instrumentation_for))
		fmt.Fprintln(w, "LOCAL_INSTRUMENT_SRCJARS :=", r.roboSrcJar.String())
		if t := r.robolectricProperties.Test_options.Timeout; t != nil {
			fmt.Fprintln(w, "LOCAL_ROBOTEST_TIMEOUT :=", *t)
		}
		fmt.Fprintln(w, "-include external/robolectric-shadows/run_robotests.mk")
	}

	return data
}

// An android_robolectric_test module compiles tests against the Robolectric framework that can run on the local host
// instead of on a device.  It also generates a rule with the name of the module prefixed with "Run" that can be
// used to run the tests.  Running the tests with build rule will eventually be deprecated and replaced with atest.
func RobolectricTestFactory() android.Module {
	module := &robolectricTest{}

	module.AddProperties(
		&module.Module.properties,
		&module.Module.protoProperties,
		&module.robolectricProperties)

	module.Module.dexpreopter.isTest = true

	InitJavaModule(module, android.DeviceSupported)
	return module
}
