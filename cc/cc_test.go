// Copyright 2017 Google Inc. All rights reserved.
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

package cc

import (
	"android/soong/android"
	"android/soong/genrule"

	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
)

var buildDir string

func setUp() {
	var err error
	buildDir, err = ioutil.TempDir("", "soong_cc_test")
	if err != nil {
		panic(err)
	}
}

func tearDown() {
	os.RemoveAll(buildDir)
}

func TestMain(m *testing.M) {
	run := func() int {
		setUp()
		defer tearDown()

		return m.Run()
	}

	os.Exit(run())
}

func testCc(t *testing.T, bp string) *android.TestContext {
	config := android.TestArchConfig(buildDir, nil)
	config.ProductVariables.DeviceVndkVersion = StringPtr("current")

	ctx := android.NewTestArchContext()
	ctx.RegisterModuleType("cc_library", android.ModuleFactoryAdaptor(LibraryFactory))
	ctx.RegisterModuleType("cc_library_shared", android.ModuleFactoryAdaptor(LibrarySharedFactory))
	ctx.RegisterModuleType("toolchain_library", android.ModuleFactoryAdaptor(toolchainLibraryFactory))
	ctx.RegisterModuleType("llndk_library", android.ModuleFactoryAdaptor(llndkLibraryFactory))
	ctx.RegisterModuleType("cc_object", android.ModuleFactoryAdaptor(objectFactory))
	ctx.RegisterModuleType("filegroup", android.ModuleFactoryAdaptor(genrule.FileGroupFactory))
	ctx.PreDepsMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.BottomUp("image", vendorMutator).Parallel()
		ctx.BottomUp("link", linkageMutator).Parallel()
		ctx.BottomUp("vndk", vndkMutator).Parallel()
	})
	ctx.Register()

	// add some modules that are required by the compiler and/or linker
	bp = bp + `
		toolchain_library {
			name: "libatomic",
			vendor_available: true,
		}

		toolchain_library {
			name: "libcompiler_rt-extras",
			vendor_available: true,
		}

		toolchain_library {
			name: "libgcc",
			vendor_available: true,
		}

		cc_library {
			name: "libc",
			no_libgcc : true,
			nocrt : true,
			system_shared_libs: [],
		}
		llndk_library {
			name: "libc",
			symbol_file: "",
		}
		cc_library {
			name: "libm",
			no_libgcc : true,
			nocrt : true,
			system_shared_libs: [],
		}
		llndk_library {
			name: "libm",
			symbol_file: "",
		}
		cc_library {
			name: "libdl",
			no_libgcc : true,
			nocrt : true,
			system_shared_libs: [],
		}
		llndk_library {
			name: "libdl",
			symbol_file: "",
		}

		cc_object {
			name: "crtbegin_so",
		}

		cc_object {
			name: "crtend_so",
		}

		cc_library {
			name: "libprotobuf-cpp-lite",
		}

`

	ctx.MockFileSystem(map[string][]byte{
		"Android.bp": []byte(bp),
		"foo.c":      nil,
		"bar.c":      nil,
		"a.proto":    nil,
		"b.aidl":     nil,
	})

	_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
	failIfErrored(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	failIfErrored(t, errs)

	return ctx
}

func TestVendorSrc(t *testing.T) {
	ctx := testCc(t, `
		cc_library {
			name: "libTest",
			srcs: ["foo.c"],
			no_libgcc : true,
			nocrt : true,
			system_shared_libs : [],
			vendor_available: true,
			target: {
				vendor: {
					srcs: ["bar.c"],
				},
			},
		}
	`)

	ld := ctx.ModuleForTests("libTest", "android_arm_armv7-a-neon_vendor_shared").Rule("ld")
	var objs []string
	for _, o := range ld.Inputs {
		objs = append(objs, o.Base())
	}
	if len(objs) != 2 {
		t.Errorf("inputs of libTest is expected to 2, but was %d.", len(objs))
	}
	if objs[0] != "foo.o" || objs[1] != "bar.o" {
		t.Errorf("inputs of libTest must be []string{\"foo.o\", \"bar.o\"}, but was %#v.", objs)
	}
}

var (
	str11 = "01234567891"
	str10 = str11[:10]
	str9  = str11[:9]
	str5  = str11[:5]
	str4  = str11[:4]
)

var splitListForSizeTestCases = []struct {
	in   []string
	out  [][]string
	size int
}{
	{
		in:   []string{str10},
		out:  [][]string{{str10}},
		size: 10,
	},
	{
		in:   []string{str9},
		out:  [][]string{{str9}},
		size: 10,
	},
	{
		in:   []string{str5},
		out:  [][]string{{str5}},
		size: 10,
	},
	{
		in:   []string{str11},
		out:  nil,
		size: 10,
	},
	{
		in:   []string{str10, str10},
		out:  [][]string{{str10}, {str10}},
		size: 10,
	},
	{
		in:   []string{str9, str10},
		out:  [][]string{{str9}, {str10}},
		size: 10,
	},
	{
		in:   []string{str10, str9},
		out:  [][]string{{str10}, {str9}},
		size: 10,
	},
	{
		in:   []string{str5, str4},
		out:  [][]string{{str5, str4}},
		size: 10,
	},
	{
		in:   []string{str5, str4, str5},
		out:  [][]string{{str5, str4}, {str5}},
		size: 10,
	},
	{
		in:   []string{str5, str4, str5, str4},
		out:  [][]string{{str5, str4}, {str5, str4}},
		size: 10,
	},
	{
		in:   []string{str5, str4, str5, str5},
		out:  [][]string{{str5, str4}, {str5}, {str5}},
		size: 10,
	},
	{
		in:   []string{str5, str5, str5, str4},
		out:  [][]string{{str5}, {str5}, {str5, str4}},
		size: 10,
	},
	{
		in:   []string{str9, str11},
		out:  nil,
		size: 10,
	},
	{
		in:   []string{str11, str9},
		out:  nil,
		size: 10,
	},
}

func TestSplitListForSize(t *testing.T) {
	for _, testCase := range splitListForSizeTestCases {
		out, _ := splitListForSize(android.PathsForTesting(testCase.in), testCase.size)

		var outStrings [][]string

		if len(out) > 0 {
			outStrings = make([][]string, len(out))
			for i, o := range out {
				outStrings[i] = o.Strings()
			}
		}

		if !reflect.DeepEqual(outStrings, testCase.out) {
			t.Errorf("incorrect output:")
			t.Errorf("     input: %#v", testCase.in)
			t.Errorf("      size: %d", testCase.size)
			t.Errorf("  expected: %#v", testCase.out)
			t.Errorf("       got: %#v", outStrings)
		}
	}
}

var staticLinkDepOrderTestCases = []struct {
	// This is a string representation of a map[moduleName][]moduleDependency .
	// It models the dependencies declared in an Android.bp file.
	inStatic string

	// This is a string representation of a map[moduleName][]moduleDependency .
	// It models the dependencies declared in an Android.bp file.
	inShared string

	// allOrdered is a string representation of a map[moduleName][]moduleDependency .
	// The keys of allOrdered specify which modules we would like to check.
	// The values of allOrdered specify the expected result (of the transitive closure of all
	// dependencies) for each module to test
	allOrdered string

	// outOrdered is a string representation of a map[moduleName][]moduleDependency .
	// The keys of outOrdered specify which modules we would like to check.
	// The values of outOrdered specify the expected result (of the ordered linker command line)
	// for each module to test.
	outOrdered string
}{
	// Simple tests
	{
		inStatic:   "",
		outOrdered: "",
	},
	{
		inStatic:   "a:",
		outOrdered: "a:",
	},
	{
		inStatic:   "a:b; b:",
		outOrdered: "a:b; b:",
	},
	// Tests of reordering
	{
		// diamond example
		inStatic:   "a:d,b,c; b:d; c:d; d:",
		outOrdered: "a:b,c,d; b:d; c:d; d:",
	},
	{
		// somewhat real example
		inStatic:   "bsdiff_unittest:b,c,d,e,f,g,h,i; e:b",
		outOrdered: "bsdiff_unittest:c,d,e,b,f,g,h,i; e:b",
	},
	{
		// multiple reorderings
		inStatic:   "a:b,c,d,e; d:b; e:c",
		outOrdered: "a:d,b,e,c; d:b; e:c",
	},
	{
		// should reorder without adding new transitive dependencies
		inStatic:   "bin:lib2,lib1;             lib1:lib2,liboptional",
		allOrdered: "bin:lib1,lib2,liboptional; lib1:lib2,liboptional",
		outOrdered: "bin:lib1,lib2;             lib1:lib2,liboptional",
	},
	{
		// multiple levels of dependencies
		inStatic:   "a:b,c,d,e,f,g,h; f:b,c,d; b:c,d; c:d",
		allOrdered: "a:e,f,b,c,d,g,h; f:b,c,d; b:c,d; c:d",
		outOrdered: "a:e,f,b,c,d,g,h; f:b,c,d; b:c,d; c:d",
	},
	// shared dependencies
	{
		// Note that this test doesn't recurse, to minimize the amount of logic it tests.
		// So, we don't actually have to check that a shared dependency of c will change the order
		// of a library that depends statically on b and on c.  We only need to check that if c has
		// a shared dependency on b, that that shows up in allOrdered.
		inShared:   "c:b",
		allOrdered: "c:b",
		outOrdered: "c:",
	},
	{
		// This test doesn't actually include any shared dependencies but it's a reminder of what
		// the second phase of the above test would look like
		inStatic:   "a:b,c; c:b",
		allOrdered: "a:c,b; c:b",
		outOrdered: "a:c,b; c:b",
	},
	// tiebreakers for when two modules specifying different orderings and there is no dependency
	// to dictate an order
	{
		// if the tie is between two modules at the end of a's deps, then a's order wins
		inStatic:   "a1:b,c,d,e; a2:b,c,e,d; b:d,e; c:e,d",
		outOrdered: "a1:b,c,d,e; a2:b,c,e,d; b:d,e; c:e,d",
	},
	{
		// if the tie is between two modules at the start of a's deps, then c's order is used
		inStatic:   "a1:d,e,b1,c1; b1:d,e; c1:e,d;   a2:d,e,b2,c2; b2:d,e; c2:d,e",
		outOrdered: "a1:b1,c1,e,d; b1:d,e; c1:e,d;   a2:b2,c2,d,e; b2:d,e; c2:d,e",
	},
	// Tests involving duplicate dependencies
	{
		// simple duplicate
		inStatic:   "a:b,c,c,b",
		outOrdered: "a:c,b",
	},
	{
		// duplicates with reordering
		inStatic:   "a:b,c,d,c; c:b",
		outOrdered: "a:d,c,b",
	},
	// Tests to confirm the nonexistence of infinite loops.
	// These cases should never happen, so as long as the test terminates and the
	// result is deterministic then that should be fine.
	{
		inStatic:   "a:a",
		outOrdered: "a:a",
	},
	{
		inStatic:   "a:b;   b:c;   c:a",
		allOrdered: "a:b,c; b:c,a; c:a,b",
		outOrdered: "a:b;   b:c;   c:a",
	},
	{
		inStatic:   "a:b,c;   b:c,a;   c:a,b",
		allOrdered: "a:c,a,b; b:a,b,c; c:b,c,a",
		outOrdered: "a:c,b;   b:a,c;   c:b,a",
	},
}

// converts from a string like "a:b,c; d:e" to (["a","b"], {"a":["b","c"], "d":["e"]}, [{"a", "a.o"}, {"b", "b.o"}])
func parseModuleDeps(text string) (modulesInOrder []android.Path, allDeps map[android.Path][]android.Path) {
	// convert from "a:b,c; d:e" to "a:b,c;d:e"
	strippedText := strings.Replace(text, " ", "", -1)
	if len(strippedText) < 1 {
		return []android.Path{}, make(map[android.Path][]android.Path, 0)
	}
	allDeps = make(map[android.Path][]android.Path, 0)

	// convert from "a:b,c;d:e" to ["a:b,c", "d:e"]
	moduleTexts := strings.Split(strippedText, ";")

	outputForModuleName := func(moduleName string) android.Path {
		return android.PathForTesting(moduleName)
	}

	for _, moduleText := range moduleTexts {
		// convert from "a:b,c" to ["a", "b,c"]
		components := strings.Split(moduleText, ":")
		if len(components) != 2 {
			panic(fmt.Sprintf("illegal module dep string %q from larger string %q; must contain one ':', not %v", moduleText, text, len(components)-1))
		}
		moduleName := components[0]
		moduleOutput := outputForModuleName(moduleName)
		modulesInOrder = append(modulesInOrder, moduleOutput)

		depString := components[1]
		// convert from "b,c" to ["b", "c"]
		depNames := strings.Split(depString, ",")
		if len(depString) < 1 {
			depNames = []string{}
		}
		var deps []android.Path
		for _, depName := range depNames {
			deps = append(deps, outputForModuleName(depName))
		}
		allDeps[moduleOutput] = deps
	}
	return modulesInOrder, allDeps
}

func TestLinkReordering(t *testing.T) {
	for _, testCase := range staticLinkDepOrderTestCases {
		errs := []string{}

		// parse testcase
		_, givenTransitiveDeps := parseModuleDeps(testCase.inStatic)
		expectedModuleNames, expectedTransitiveDeps := parseModuleDeps(testCase.outOrdered)
		if testCase.allOrdered == "" {
			// allow the test case to skip specifying allOrdered
			testCase.allOrdered = testCase.outOrdered
		}
		_, expectedAllDeps := parseModuleDeps(testCase.allOrdered)
		_, givenAllSharedDeps := parseModuleDeps(testCase.inShared)

		// For each module whose post-reordered dependencies were specified, validate that
		// reordering the inputs produces the expected outputs.
		for _, moduleName := range expectedModuleNames {
			moduleDeps := givenTransitiveDeps[moduleName]
			givenSharedDeps := givenAllSharedDeps[moduleName]
			orderedAllDeps, orderedDeclaredDeps := orderDeps(moduleDeps, givenSharedDeps, givenTransitiveDeps)

			correctAllOrdered := expectedAllDeps[moduleName]
			if !reflect.DeepEqual(orderedAllDeps, correctAllOrdered) {
				errs = append(errs, fmt.Sprintf("orderDeps returned incorrect orderedAllDeps."+
					"\nin static:%q"+
					"\nin shared:%q"+
					"\nmodule:   %v"+
					"\nexpected: %s"+
					"\nactual:   %s",
					testCase.inStatic, testCase.inShared, moduleName, correctAllOrdered, orderedAllDeps))
			}

			correctOutputDeps := expectedTransitiveDeps[moduleName]
			if !reflect.DeepEqual(correctOutputDeps, orderedDeclaredDeps) {
				errs = append(errs, fmt.Sprintf("orderDeps returned incorrect orderedDeclaredDeps."+
					"\nin static:%q"+
					"\nin shared:%q"+
					"\nmodule:   %v"+
					"\nexpected: %s"+
					"\nactual:   %s",
					testCase.inStatic, testCase.inShared, moduleName, correctOutputDeps, orderedDeclaredDeps))
			}
		}

		if len(errs) > 0 {
			sort.Strings(errs)
			for _, err := range errs {
				t.Error(err)
			}
		}
	}
}
func failIfErrored(t *testing.T, errs []error) {
	if len(errs) > 0 {
		for _, err := range errs {
			t.Error(err)
		}
		t.FailNow()
	}
}

func getOutputPaths(ctx *android.TestContext, variant string, moduleNames []string) (paths android.Paths) {
	for _, moduleName := range moduleNames {
		module := ctx.ModuleForTests(moduleName, variant).Module().(*Module)
		output := module.outputFile.Path()
		paths = append(paths, output)
	}
	return paths
}

func TestStaticLibDepReordering(t *testing.T) {
	ctx := testCc(t, `
	cc_library {
		name: "a",
		static_libs: ["b", "c", "d"],
	}
	cc_library {
		name: "b",
	}
	cc_library {
		name: "c",
		static_libs: ["b"],
	}
	cc_library {
		name: "d",
	}

	`)

	variant := "android_arm64_armv8-a_core_static"
	moduleA := ctx.ModuleForTests("a", variant).Module().(*Module)
	actual := moduleA.depsInLinkOrder
	expected := getOutputPaths(ctx, variant, []string{"c", "b", "d"})

	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("staticDeps orderings were not propagated correctly"+
			"\nactual:   %v"+
			"\nexpected: %v",
			actual,
			expected,
		)
	}
}

func TestStaticLibDepReorderingWithShared(t *testing.T) {
	ctx := testCc(t, `
	cc_library {
		name: "a",
		static_libs: ["b", "c"],
	}
	cc_library {
		name: "b",
	}
	cc_library {
		name: "c",
		shared_libs: ["b"],
	}

	`)

	variant := "android_arm64_armv8-a_core_static"
	moduleA := ctx.ModuleForTests("a", variant).Module().(*Module)
	actual := moduleA.depsInLinkOrder
	expected := getOutputPaths(ctx, variant, []string{"c", "b"})

	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("staticDeps orderings did not account for shared libs"+
			"\nactual:   %v"+
			"\nexpected: %v",
			actual,
			expected,
		)
	}
}

var compilerFlagsTestCases = []struct {
	in  string
	out bool
}{
	{
		in:  "a",
		out: false,
	},
	{
		in:  "-a",
		out: true,
	},
	{
		in:  "-Ipath/to/something",
		out: false,
	},
	{
		in:  "-isystempath/to/something",
		out: false,
	},
	{
		in:  "--coverage",
		out: false,
	},
	{
		in:  "-include a/b",
		out: true,
	},
	{
		in:  "-include a/b c/d",
		out: false,
	},
	{
		in:  "-DMACRO",
		out: true,
	},
	{
		in:  "-DMAC RO",
		out: false,
	},
	{
		in:  "-a -b",
		out: false,
	},
	{
		in:  "-DMACRO=definition",
		out: true,
	},
	{
		in:  "-DMACRO=defi nition",
		out: true, // TODO(jiyong): this should be false
	},
	{
		in:  "-DMACRO(x)=x + 1",
		out: true,
	},
	{
		in:  "-DMACRO=\"defi nition\"",
		out: true,
	},
}

type mockContext struct {
	BaseModuleContext
	result bool
}

func (ctx *mockContext) PropertyErrorf(property, format string, args ...interface{}) {
	// CheckBadCompilerFlags calls this function when the flag should be rejected
	ctx.result = false
}

func TestCompilerFlags(t *testing.T) {
	for _, testCase := range compilerFlagsTestCases {
		ctx := &mockContext{result: true}
		CheckBadCompilerFlags(ctx, "", []string{testCase.in})
		if ctx.result != testCase.out {
			t.Errorf("incorrect output:")
			t.Errorf("     input: %#v", testCase.in)
			t.Errorf("  expected: %#v", testCase.out)
			t.Errorf("       got: %#v", ctx.result)
		}
	}
}
