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
	"strconv"
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

		// Number of shards to use when running the tests.
		Shards *int64
	}
}

type robolectricTest struct {
	Library

	robolectricProperties robolectricProperties

	libs  []string
	tests []string

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
		r.libs = append(r.libs, dep.(Dependency).BaseModuleName())
	}

	// TODO: this could all be removed if tradefed was used as the test runner, it will find everything
	// annotated as a test and run it.
	for _, src := range r.compiledJavaSrcs {
		s := src.Rel()
		if !strings.HasSuffix(s, "Test.java") {
			continue
		} else if strings.HasSuffix(s, "/BaseRobolectricTest.java") {
			continue
		} else if strings.HasPrefix(s, "src/") {
			s = strings.TrimPrefix(s, "src/")
		}
		r.tests = append(r.tests, s)
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

func (r *robolectricTest) AndroidMkEntries() []android.AndroidMkEntries {
	entriesList := r.Library.AndroidMkEntries()
	entries := &entriesList[0]

	entries.ExtraFooters = []android.AndroidMkExtraFootersFunc{
		func(w io.Writer, name, prefix, moduleDir string, entries *android.AndroidMkEntries) {
			if s := r.robolectricProperties.Test_options.Shards; s != nil && *s > 1 {
				numShards := int(*s)
				shardSize := (len(r.tests) + numShards - 1) / numShards
				shards := android.ShardStrings(r.tests, shardSize)
				for i, shard := range shards {
					r.writeTestRunner(w, name, "Run"+name+strconv.Itoa(i), shard)
				}

				// TODO: add rules to dist the outputs of the individual tests, or combine them together?
				fmt.Fprintln(w, "")
				fmt.Fprintln(w, ".PHONY:", "Run"+name)
				fmt.Fprintln(w, "Run"+name, ": \\")
				for i := range shards {
					fmt.Fprintln(w, "   ", "Run"+name+strconv.Itoa(i), "\\")
				}
				fmt.Fprintln(w, "")
			} else {
				r.writeTestRunner(w, name, "Run"+name, r.tests)
			}
		},
	}

	return entriesList
}

func (r *robolectricTest) writeTestRunner(w io.Writer, module, name string, tests []string) {
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "include $(CLEAR_VARS)")
	fmt.Fprintln(w, "LOCAL_MODULE :=", name)
	fmt.Fprintln(w, "LOCAL_JAVA_LIBRARIES :=", module)
	fmt.Fprintln(w, "LOCAL_JAVA_LIBRARIES += ", strings.Join(r.libs, " "))
	fmt.Fprintln(w, "LOCAL_TEST_PACKAGE :=", String(r.robolectricProperties.Instrumentation_for))
	fmt.Fprintln(w, "LOCAL_INSTRUMENT_SRCJARS :=", r.roboSrcJar.String())
	fmt.Fprintln(w, "LOCAL_ROBOTEST_FILES :=", strings.Join(tests, " "))
	if t := r.robolectricProperties.Test_options.Timeout; t != nil {
		fmt.Fprintln(w, "LOCAL_ROBOTEST_TIMEOUT :=", *t)
	}
	fmt.Fprintln(w, "-include external/robolectric-shadows/run_robotests.mk")

}

// An android_robolectric_test module compiles tests against the Robolectric framework that can run on the local host
// instead of on a device.  It also generates a rule with the name of the module prefixed with "Run" that can be
// used to run the tests.  Running the tests with build rule will eventually be deprecated and replaced with atest.
//
// The test runner considers any file listed in srcs whose name ends with Test.java to be a test class, unless
// it is named BaseRobolectricTest.java.  The path to the each source file must exactly match the package
// name, or match the package name when the prefix "src/" is removed.
func RobolectricTestFactory() android.Module {
	module := &robolectricTest{}

	module.addHostProperties()
	module.AddProperties(
		&module.Module.deviceProperties,
		&module.robolectricProperties)

	module.Module.dexpreopter.isTest = true
	module.Module.linter.test = true

	InitJavaModule(module, android.DeviceSupported)
	return module
}
