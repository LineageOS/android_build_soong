package cc

import (
	"android/soong/android"
	"io/ioutil"
	"os"
	"reflect"
	"testing"

	"github.com/google/blueprint/proptools"
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
	config.ProductVariables.DeviceVndkVersion = proptools.StringPtr("current")

	ctx := android.NewTestArchContext()
	ctx.RegisterModuleType("cc_library", android.ModuleFactoryAdaptor(libraryFactory))
	ctx.RegisterModuleType("toolchain_library", android.ModuleFactoryAdaptor(toolchainLibraryFactory))
	ctx.RegisterModuleType("llndk_library", android.ModuleFactoryAdaptor(llndkLibraryFactory))
	ctx.PreDepsMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.BottomUp("image", vendorMutator).Parallel()
		ctx.BottomUp("link", linkageMutator).Parallel()
		ctx.BottomUp("vndk", vndkMutator).Parallel()
	})
	ctx.Register()

	ctx.MockFileSystem(map[string][]byte{
		"Android.bp": []byte(bp),
		"foo.c":      nil,
		"bar.c":      nil,
	})

	_, errs := ctx.ParseBlueprintsFiles("Android.bp")
	fail(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	fail(t, errs)

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
		toolchain_library {
			name: "libatomic",
			vendor_available: true,
		}
		toolchain_library {
			name: "libcompiler_rt-extras",
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

var firstUniqueElementsTestCases = []struct {
	in  []string
	out []string
}{
	{
		in:  []string{"a"},
		out: []string{"a"},
	},
	{
		in:  []string{"a", "b"},
		out: []string{"a", "b"},
	},
	{
		in:  []string{"a", "a"},
		out: []string{"a"},
	},
	{
		in:  []string{"a", "b", "a"},
		out: []string{"a", "b"},
	},
	{
		in:  []string{"b", "a", "a"},
		out: []string{"b", "a"},
	},
	{
		in:  []string{"a", "a", "b"},
		out: []string{"a", "b"},
	},
	{
		in:  []string{"a", "b", "a", "b"},
		out: []string{"a", "b"},
	},
	{
		in:  []string{"liblog", "libdl", "libc++", "libdl", "libc", "libm"},
		out: []string{"liblog", "libdl", "libc++", "libc", "libm"},
	},
}

func TestFirstUniqueElements(t *testing.T) {
	for _, testCase := range firstUniqueElementsTestCases {
		out := firstUniqueElements(testCase.in)
		if !reflect.DeepEqual(out, testCase.out) {
			t.Errorf("incorrect output:")
			t.Errorf("     input: %#v", testCase.in)
			t.Errorf("  expected: %#v", testCase.out)
			t.Errorf("       got: %#v", out)
		}
	}
}

var lastUniqueElementsTestCases = []struct {
	in  []string
	out []string
}{
	{
		in:  []string{"a"},
		out: []string{"a"},
	},
	{
		in:  []string{"a", "b"},
		out: []string{"a", "b"},
	},
	{
		in:  []string{"a", "a"},
		out: []string{"a"},
	},
	{
		in:  []string{"a", "b", "a"},
		out: []string{"b", "a"},
	},
	{
		in:  []string{"b", "a", "a"},
		out: []string{"b", "a"},
	},
	{
		in:  []string{"a", "a", "b"},
		out: []string{"a", "b"},
	},
	{
		in:  []string{"a", "b", "a", "b"},
		out: []string{"a", "b"},
	},
	{
		in:  []string{"liblog", "libdl", "libc++", "libdl", "libc", "libm"},
		out: []string{"liblog", "libc++", "libdl", "libc", "libm"},
	},
}

func TestLastUniqueElements(t *testing.T) {
	for _, testCase := range lastUniqueElementsTestCases {
		out := lastUniqueElements(testCase.in)
		if !reflect.DeepEqual(out, testCase.out) {
			t.Errorf("incorrect output:")
			t.Errorf("     input: %#v", testCase.in)
			t.Errorf("  expected: %#v", testCase.out)
			t.Errorf("       got: %#v", out)
		}
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
