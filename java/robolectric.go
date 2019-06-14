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

type robolectricProperties struct {
	// The name of the android_app module that the tests will run against.
	Instrumentation_for *string

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
}

func (r *robolectricTest) DepsMutator(ctx android.BottomUpMutatorContext) {
	r.Library.DepsMutator(ctx)

	if r.robolectricProperties.Instrumentation_for != nil {
		ctx.AddVariationDependencies(nil, instrumentationForTag, String(r.robolectricProperties.Instrumentation_for))
	} else {
		ctx.PropertyErrorf("instrumentation_for", "missing required instrumented module")
	}

	ctx.AddVariationDependencies(nil, libTag, robolectricDefaultLibs...)
}

func (r *robolectricTest) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	r.Library.GenerateAndroidBuildActions(ctx)

	for _, dep := range ctx.GetDirectDepsWithTag(libTag) {
		r.libs = append(r.libs, ctx.OtherModuleName(dep))
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

func shardTests(paths []string, shards int) [][]string {
	if shards > len(paths) {
		shards = len(paths)
	}
	if shards == 0 {
		return nil
	}
	ret := make([][]string, 0, shards)
	shardSize := (len(paths) + shards - 1) / shards
	for len(paths) > shardSize {
		ret = append(ret, paths[0:shardSize])
		paths = paths[shardSize:]
	}
	if len(paths) > 0 {
		ret = append(ret, paths)
	}
	return ret
}

func (r *robolectricTest) AndroidMk() android.AndroidMkData {
	data := r.Library.AndroidMk()

	data.Custom = func(w io.Writer, name, prefix, moduleDir string, data android.AndroidMkData) {
		android.WriteAndroidMkData(w, data)

		if s := r.robolectricProperties.Test_options.Shards; s != nil && *s > 1 {
			shards := shardTests(r.tests, int(*s))
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
	}

	return data
}

func (r *robolectricTest) writeTestRunner(w io.Writer, module, name string, tests []string) {
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "include $(CLEAR_VARS)")
	fmt.Fprintln(w, "LOCAL_MODULE :=", name)
	fmt.Fprintln(w, "LOCAL_JAVA_LIBRARIES :=", module)
	fmt.Fprintln(w, "LOCAL_JAVA_LIBRARIES += ", strings.Join(r.libs, " "))
	fmt.Fprintln(w, "LOCAL_TEST_PACKAGE :=", String(r.robolectricProperties.Instrumentation_for))
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

	module.AddProperties(
		&module.Module.properties,
		&module.Module.protoProperties,
		&module.robolectricProperties)

	module.Module.dexpreopter.isTest = true

	InitJavaModule(module, android.DeviceSupported)
	return module
}
