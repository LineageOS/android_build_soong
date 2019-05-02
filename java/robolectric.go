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

type robolectricProperties struct {
	// The name of the android_app module that the tests will run against.
	Instrumentation_for *string

	Test_options struct {
		// Timeout in seconds when running the tests.
		Timeout *string
	}
}

type robolectricTest struct {
	Library

	robolectricProperties robolectricProperties

	libs []string
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
