// Copyright (C) 2021 The Android Open Source Project
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

package apex

import (
	"reflect"
	"testing"

	"android/soong/android"
	"android/soong/java"
)

// Contains tests for java.CreateClasspathElements logic from java/classpath_element.go that
// requires apexes.

// testClasspathElementContext is a ClasspathElementContext suitable for use in tests.
type testClasspathElementContext struct {
	android.OtherModuleProviderContext
	testContext *android.TestContext
	module      android.Module
	errs        []error
}

func (t *testClasspathElementContext) ModuleErrorf(fmt string, args ...interface{}) {
	t.errs = append(t.errs, t.testContext.ModuleErrorf(t.module, fmt, args...))
}

var _ java.ClasspathElementContext = (*testClasspathElementContext)(nil)

func TestCreateClasspathElements(t *testing.T) {
	preparer := android.GroupFixturePreparers(
		prepareForTestWithPlatformBootclasspath,
		prepareForTestWithArtApex,
		prepareForTestWithMyapex,
		// For otherapex.
		android.FixtureMergeMockFs(android.MockFS{
			"system/sepolicy/apex/otherapex-file_contexts": nil,
		}),
		java.PrepareForTestWithJavaSdkLibraryFiles,
		java.FixtureWithLastReleaseApis("foo", "othersdklibrary"),
		java.FixtureConfigureApexBootJars("myapex:bar"),
		android.FixtureWithRootAndroidBp(`
		apex {
			name: "com.android.art",
			key: "com.android.art.key",
 			bootclasspath_fragments: [
				"art-bootclasspath-fragment",
			],
			java_libs: [
				"othersdklibrary",
			],
			updatable: false,
		}

		apex_key {
			name: "com.android.art.key",
			public_key: "com.android.art.avbpubkey",
			private_key: "com.android.art.pem",
		}

		bootclasspath_fragment {
			name: "art-bootclasspath-fragment",
			image_name: "art",
			apex_available: [
				"com.android.art",
			],
			contents: [
				"baz",
				"quuz",
			],
			hidden_api: {
				split_packages: ["*"],
			},
		}

		java_library {
			name: "baz",
			apex_available: [
				"com.android.art",
			],
			srcs: ["b.java"],
			installable: true,
		}

		java_library {
			name: "quuz",
			apex_available: [
				"com.android.art",
			],
			srcs: ["b.java"],
			installable: true,
		}

		apex {
			name: "myapex",
			key: "myapex.key",
 			bootclasspath_fragments: [
				"mybootclasspath-fragment",
			],
			java_libs: [
				"othersdklibrary",
			],
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		bootclasspath_fragment {
			name: "mybootclasspath-fragment",
			apex_available: [
				"myapex",
			],
			contents: [
				"bar",
			],
			hidden_api: {
				split_packages: ["*"],
			},
		}

		java_library {
			name: "bar",
			srcs: ["b.java"],
			installable: true,
			apex_available: ["myapex"],
			permitted_packages: ["bar"],
		}

		java_sdk_library {
			name: "foo",
			srcs: ["b.java"],
		}

		java_sdk_library {
			name: "othersdklibrary",
			srcs: ["b.java"],
			shared_library: false,
			apex_available: [
				"com.android.art",
				"myapex",
			],
		}

		apex {
			name: "otherapex",
			key: "otherapex.key",
			java_libs: [
				"otherapexlibrary",
			],
			updatable: false,
		}

		apex_key {
			name: "otherapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		java_library {
			name: "otherapexlibrary",
			srcs: ["b.java"],
			installable: true,
			apex_available: ["otherapex"],
			permitted_packages: ["otherapexlibrary"],
		}

		platform_bootclasspath {
			name: "myplatform-bootclasspath",

			fragments: [
				{
					apex: "com.android.art",
					module: "art-bootclasspath-fragment",
				},
				{
					apex: "myapex",
					module: "mybootclasspath-fragment",
				},
			],
		}
	`),
	)

	result := preparer.RunTest(t)

	artFragment := result.Module("art-bootclasspath-fragment", "android_common_apex10000")
	artBaz := result.Module("baz", "android_common_apex10000")
	artQuuz := result.Module("quuz", "android_common_apex10000")

	myFragment := result.Module("mybootclasspath-fragment", "android_common_apex10000")
	myBar := result.Module("bar", "android_common_apex10000")

	other := result.Module("othersdklibrary", "android_common_apex10000")

	otherApexLibrary := result.Module("otherapexlibrary", "android_common_apex10000")

	platformFoo := result.Module("quuz", "android_common")

	bootclasspath := result.Module("myplatform-bootclasspath", "android_common")

	// Use a custom assertion method instead of AssertDeepEquals as the latter formats the output
	// using %#v which results in meaningless output as ClasspathElements are pointers.
	assertElementsEquals := func(t *testing.T, message string, expected, actual java.ClasspathElements) {
		if !reflect.DeepEqual(expected, actual) {
			t.Errorf("%s: expected:\n  %s\n got:\n  %s", message, expected, actual)
		}
	}

	expectFragmentElement := func(module android.Module, contents ...android.Module) java.ClasspathElement {
		return &java.ClasspathFragmentElement{module, contents}
	}
	expectLibraryElement := func(module android.Module) java.ClasspathElement {
		return &java.ClasspathLibraryElement{module}
	}

	newCtx := func() *testClasspathElementContext {
		return &testClasspathElementContext{
			OtherModuleProviderContext: result.TestContext.OtherModuleProviderAdaptor(),
			testContext:                result.TestContext,
			module:                     bootclasspath,
		}
	}

	// Verify that CreateClasspathElements works when given valid input.
	t.Run("art:baz, art:quuz, my:bar, foo", func(t *testing.T) {
		ctx := newCtx()
		elements := java.CreateClasspathElements(ctx, []android.Module{artBaz, artQuuz, myBar, platformFoo}, []android.Module{artFragment, myFragment})
		expectedElements := java.ClasspathElements{
			expectFragmentElement(artFragment, artBaz, artQuuz),
			expectFragmentElement(myFragment, myBar),
			expectLibraryElement(platformFoo),
		}
		assertElementsEquals(t, "elements", expectedElements, elements)
	})

	// Verify that CreateClasspathElements detects when an apex has multiple fragments.
	t.Run("multiple fragments for same apex", func(t *testing.T) {
		ctx := newCtx()
		elements := java.CreateClasspathElements(ctx, []android.Module{}, []android.Module{artFragment, artFragment})
		android.FailIfNoMatchingErrors(t, "apex com.android.art has multiple fragments, art-bootclasspath-fragment{.*} and art-bootclasspath-fragment{.*}", ctx.errs)
		expectedElements := java.ClasspathElements{}
		assertElementsEquals(t, "elements", expectedElements, elements)
	})

	// Verify that CreateClasspathElements detects when a library is in multiple fragments.
	t.Run("library from multiple fragments", func(t *testing.T) {
		ctx := newCtx()
		elements := java.CreateClasspathElements(ctx, []android.Module{other}, []android.Module{artFragment, myFragment})
		android.FailIfNoMatchingErrors(t, "library othersdklibrary{.*} is in two separate fragments, art-bootclasspath-fragment{.*} and mybootclasspath-fragment{.*}", ctx.errs)
		expectedElements := java.ClasspathElements{}
		assertElementsEquals(t, "elements", expectedElements, elements)
	})

	// Verify that CreateClasspathElements detects when a fragment's contents are not contiguous and
	// are separated by a library from another fragment.
	t.Run("discontiguous separated by fragment", func(t *testing.T) {
		ctx := newCtx()
		elements := java.CreateClasspathElements(ctx, []android.Module{artBaz, myBar, artQuuz, platformFoo}, []android.Module{artFragment, myFragment})
		expectedElements := java.ClasspathElements{
			expectFragmentElement(artFragment, artBaz, artQuuz),
			expectFragmentElement(myFragment, myBar),
			expectLibraryElement(platformFoo),
		}
		assertElementsEquals(t, "elements", expectedElements, elements)
		android.FailIfNoMatchingErrors(t, "libraries from the same fragment must be contiguous, however baz{.*} and quuz{os:android,arch:common,apex:apex10000} from fragment art-bootclasspath-fragment{.*} are separated by libraries from fragment mybootclasspath-fragment{.*} like bar{.*}", ctx.errs)
	})

	// Verify that CreateClasspathElements detects when a fragment's contents are not contiguous and
	// are separated by a standalone library.
	t.Run("discontiguous separated by library", func(t *testing.T) {
		ctx := newCtx()
		elements := java.CreateClasspathElements(ctx, []android.Module{artBaz, platformFoo, artQuuz, myBar}, []android.Module{artFragment, myFragment})
		expectedElements := java.ClasspathElements{
			expectFragmentElement(artFragment, artBaz, artQuuz),
			expectLibraryElement(platformFoo),
			expectFragmentElement(myFragment, myBar),
		}
		assertElementsEquals(t, "elements", expectedElements, elements)
		android.FailIfNoMatchingErrors(t, "libraries from the same fragment must be contiguous, however baz{.*} and quuz{os:android,arch:common,apex:apex10000} from fragment art-bootclasspath-fragment{.*} are separated by library quuz{.*}", ctx.errs)
	})

	// Verify that CreateClasspathElements detects when there a library on the classpath that
	// indicates it is from an apex the supplied fragments list does not contain a fragment for that
	// apex.
	t.Run("no fragment for apex", func(t *testing.T) {
		ctx := newCtx()
		elements := java.CreateClasspathElements(ctx, []android.Module{artBaz, otherApexLibrary}, []android.Module{artFragment})
		expectedElements := java.ClasspathElements{
			expectFragmentElement(artFragment, artBaz),
		}
		assertElementsEquals(t, "elements", expectedElements, elements)
		android.FailIfNoMatchingErrors(t, `library otherapexlibrary{.*} is from apexes \[otherapex\] which have no corresponding fragment in \[art-bootclasspath-fragment{.*}\]`, ctx.errs)
	})
}
