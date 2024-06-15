// Copyright 2015 Google Inc. All rights reserved.
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

package android

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/google/blueprint/proptools"
)

type strsTestCase struct {
	in  []string
	out string
	err []error
}

var commonValidatePathTestCases = []strsTestCase{
	{
		in:  []string{""},
		out: "",
	},
	{
		in:  []string{"", ""},
		out: "",
	},
	{
		in:  []string{"a", ""},
		out: "a",
	},
	{
		in:  []string{"", "a"},
		out: "a",
	},
	{
		in:  []string{"", "a", ""},
		out: "a",
	},
	{
		in:  []string{"a/b"},
		out: "a/b",
	},
	{
		in:  []string{"a/b", "c"},
		out: "a/b/c",
	},
	{
		in:  []string{"a/.."},
		out: ".",
	},
	{
		in:  []string{"."},
		out: ".",
	},
	{
		in:  []string{".."},
		out: "",
		err: []error{errors.New("Path is outside directory: ..")},
	},
	{
		in:  []string{"../a"},
		out: "",
		err: []error{errors.New("Path is outside directory: ../a")},
	},
	{
		in:  []string{"b/../../a"},
		out: "",
		err: []error{errors.New("Path is outside directory: ../a")},
	},
	{
		in:  []string{"/a"},
		out: "",
		err: []error{errors.New("Path is outside directory: /a")},
	},
	{
		in:  []string{"a", "../b"},
		out: "",
		err: []error{errors.New("Path is outside directory: ../b")},
	},
	{
		in:  []string{"a", "b/../../c"},
		out: "",
		err: []error{errors.New("Path is outside directory: ../c")},
	},
	{
		in:  []string{"a", "./.."},
		out: "",
		err: []error{errors.New("Path is outside directory: ..")},
	},
}

var validateSafePathTestCases = append(commonValidatePathTestCases, []strsTestCase{
	{
		in:  []string{"$host/../$a"},
		out: "$a",
	},
}...)

var validatePathTestCases = append(commonValidatePathTestCases, []strsTestCase{
	{
		in:  []string{"$host/../$a"},
		out: "",
		err: []error{errors.New("Path contains invalid character($): $host/../$a")},
	},
	{
		in:  []string{"$host/.."},
		out: "",
		err: []error{errors.New("Path contains invalid character($): $host/..")},
	},
}...)

func TestValidateSafePath(t *testing.T) {
	for _, testCase := range validateSafePathTestCases {
		t.Run(strings.Join(testCase.in, ","), func(t *testing.T) {
			ctx := &configErrorWrapper{}
			out, err := validateSafePath(testCase.in...)
			if err != nil {
				reportPathError(ctx, err)
			}
			check(t, "validateSafePath", p(testCase.in), out, ctx.errors, testCase.out, testCase.err)
		})
	}
}

func TestValidatePath(t *testing.T) {
	for _, testCase := range validatePathTestCases {
		t.Run(strings.Join(testCase.in, ","), func(t *testing.T) {
			ctx := &configErrorWrapper{}
			out, err := validatePath(testCase.in...)
			if err != nil {
				reportPathError(ctx, err)
			}
			check(t, "validatePath", p(testCase.in), out, ctx.errors, testCase.out, testCase.err)
		})
	}
}

func TestOptionalPath(t *testing.T) {
	var path OptionalPath
	checkInvalidOptionalPath(t, path, "unknown")

	path = OptionalPathForPath(nil)
	checkInvalidOptionalPath(t, path, "unknown")

	path = InvalidOptionalPath("foo")
	checkInvalidOptionalPath(t, path, "foo")

	path = InvalidOptionalPath("")
	checkInvalidOptionalPath(t, path, "unknown")

	path = OptionalPathForPath(PathForTesting("path"))
	checkValidOptionalPath(t, path, "path")
}

func checkInvalidOptionalPath(t *testing.T, path OptionalPath, expectedInvalidReason string) {
	t.Helper()
	if path.Valid() {
		t.Errorf("Invalid OptionalPath should not be valid")
	}
	if path.InvalidReason() != expectedInvalidReason {
		t.Errorf("Wrong invalid reason: expected %q, got %q", expectedInvalidReason, path.InvalidReason())
	}
	if path.String() != "" {
		t.Errorf("Invalid OptionalPath String() should return \"\", not %q", path.String())
	}
	paths := path.AsPaths()
	if len(paths) != 0 {
		t.Errorf("Invalid OptionalPath AsPaths() should return empty Paths, not %q", paths)
	}
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected a panic when calling Path() on an uninitialized OptionalPath")
		}
	}()
	path.Path()
}

func checkValidOptionalPath(t *testing.T, path OptionalPath, expectedString string) {
	t.Helper()
	if !path.Valid() {
		t.Errorf("Initialized OptionalPath should not be invalid")
	}
	if path.InvalidReason() != "" {
		t.Errorf("Initialized OptionalPath should not have an invalid reason, got: %q", path.InvalidReason())
	}
	if path.String() != expectedString {
		t.Errorf("Initialized OptionalPath String() should return %q, not %q", expectedString, path.String())
	}
	paths := path.AsPaths()
	if len(paths) != 1 {
		t.Errorf("Initialized OptionalPath AsPaths() should return Paths with length 1, not %q", paths)
	}
	path.Path()
}

func check(t *testing.T, testType, testString string,
	got interface{}, err []error,
	expected interface{}, expectedErr []error) {
	t.Helper()

	printedTestCase := false
	e := func(s string, expected, got interface{}) {
		t.Helper()
		if !printedTestCase {
			t.Errorf("test case %s: %s", testType, testString)
			printedTestCase = true
		}
		t.Errorf("incorrect %s", s)
		t.Errorf("  expected: %s", p(expected))
		t.Errorf("       got: %s", p(got))
	}

	if !reflect.DeepEqual(expectedErr, err) {
		e("errors:", expectedErr, err)
	}

	if !reflect.DeepEqual(expected, got) {
		e("output:", expected, got)
	}
}

func p(in interface{}) string {
	if v, ok := in.([]interface{}); ok {
		s := make([]string, len(v))
		for i := range v {
			s[i] = fmt.Sprintf("%#v", v[i])
		}
		return "[" + strings.Join(s, ", ") + "]"
	} else {
		return fmt.Sprintf("%#v", in)
	}
}

func pathTestConfig(buildDir string) Config {
	return TestConfig(buildDir, nil, "", nil)
}

func TestPathForModuleInstall(t *testing.T) {
	testConfig := pathTestConfig("")

	hostTarget := Target{Os: Linux, Arch: Arch{ArchType: X86}}
	deviceTarget := Target{Os: Android, Arch: Arch{ArchType: Arm64}}

	testCases := []struct {
		name         string
		ctx          *testModuleInstallPathContext
		in           []string
		out          string
		partitionDir string
	}{
		{
			name: "host binary",
			ctx: &testModuleInstallPathContext{
				baseModuleContext: baseModuleContext{
					archModuleContext: archModuleContext{
						os:     hostTarget.Os,
						target: hostTarget,
					},
				},
			},
			in:           []string{"bin", "my_test"},
			out:          "host/linux-x86/bin/my_test",
			partitionDir: "host/linux-x86",
		},

		{
			name: "system binary",
			ctx: &testModuleInstallPathContext{
				baseModuleContext: baseModuleContext{
					archModuleContext: archModuleContext{
						os:     deviceTarget.Os,
						target: deviceTarget,
					},
				},
			},
			in:           []string{"bin", "my_test"},
			out:          "target/product/test_device/system/bin/my_test",
			partitionDir: "target/product/test_device/system",
		},
		{
			name: "vendor binary",
			ctx: &testModuleInstallPathContext{
				baseModuleContext: baseModuleContext{
					archModuleContext: archModuleContext{
						os:     deviceTarget.Os,
						target: deviceTarget,
					},
					earlyModuleContext: earlyModuleContext{
						kind: socSpecificModule,
					},
				},
			},
			in:           []string{"bin", "my_test"},
			out:          "target/product/test_device/vendor/bin/my_test",
			partitionDir: "target/product/test_device/vendor",
		},
		{
			name: "odm binary",
			ctx: &testModuleInstallPathContext{
				baseModuleContext: baseModuleContext{
					archModuleContext: archModuleContext{
						os:     deviceTarget.Os,
						target: deviceTarget,
					},
					earlyModuleContext: earlyModuleContext{
						kind: deviceSpecificModule,
					},
				},
			},
			in:           []string{"bin", "my_test"},
			out:          "target/product/test_device/odm/bin/my_test",
			partitionDir: "target/product/test_device/odm",
		},
		{
			name: "product binary",
			ctx: &testModuleInstallPathContext{
				baseModuleContext: baseModuleContext{
					archModuleContext: archModuleContext{
						os:     deviceTarget.Os,
						target: deviceTarget,
					},
					earlyModuleContext: earlyModuleContext{
						kind: productSpecificModule,
					},
				},
			},
			in:           []string{"bin", "my_test"},
			out:          "target/product/test_device/product/bin/my_test",
			partitionDir: "target/product/test_device/product",
		},
		{
			name: "system_ext binary",
			ctx: &testModuleInstallPathContext{
				baseModuleContext: baseModuleContext{
					archModuleContext: archModuleContext{
						os:     deviceTarget.Os,
						target: deviceTarget,
					},
					earlyModuleContext: earlyModuleContext{
						kind: systemExtSpecificModule,
					},
				},
			},
			in:           []string{"bin", "my_test"},
			out:          "target/product/test_device/system_ext/bin/my_test",
			partitionDir: "target/product/test_device/system_ext",
		},
		{
			name: "root binary",
			ctx: &testModuleInstallPathContext{
				baseModuleContext: baseModuleContext{
					archModuleContext: archModuleContext{
						os:     deviceTarget.Os,
						target: deviceTarget,
					},
				},
				inRoot: true,
			},
			in:           []string{"my_test"},
			out:          "target/product/test_device/root/my_test",
			partitionDir: "target/product/test_device/root",
		},
		{
			name: "recovery binary",
			ctx: &testModuleInstallPathContext{
				baseModuleContext: baseModuleContext{
					archModuleContext: archModuleContext{
						os:     deviceTarget.Os,
						target: deviceTarget,
					},
				},
				inRecovery: true,
			},
			in:           []string{"bin/my_test"},
			out:          "target/product/test_device/recovery/root/system/bin/my_test",
			partitionDir: "target/product/test_device/recovery/root/system",
		},
		{
			name: "recovery root binary",
			ctx: &testModuleInstallPathContext{
				baseModuleContext: baseModuleContext{
					archModuleContext: archModuleContext{
						os:     deviceTarget.Os,
						target: deviceTarget,
					},
				},
				inRecovery: true,
				inRoot:     true,
			},
			in:           []string{"my_test"},
			out:          "target/product/test_device/recovery/root/my_test",
			partitionDir: "target/product/test_device/recovery/root",
		},

		{
			name: "ramdisk binary",
			ctx: &testModuleInstallPathContext{
				baseModuleContext: baseModuleContext{
					archModuleContext: archModuleContext{
						os:     deviceTarget.Os,
						target: deviceTarget,
					},
				},
				inRamdisk: true,
			},
			in:           []string{"my_test"},
			out:          "target/product/test_device/ramdisk/system/my_test",
			partitionDir: "target/product/test_device/ramdisk/system",
		},
		{
			name: "ramdisk root binary",
			ctx: &testModuleInstallPathContext{
				baseModuleContext: baseModuleContext{
					archModuleContext: archModuleContext{
						os:     deviceTarget.Os,
						target: deviceTarget,
					},
				},
				inRamdisk: true,
				inRoot:    true,
			},
			in:           []string{"my_test"},
			out:          "target/product/test_device/ramdisk/my_test",
			partitionDir: "target/product/test_device/ramdisk",
		},
		{
			name: "vendor_ramdisk binary",
			ctx: &testModuleInstallPathContext{
				baseModuleContext: baseModuleContext{
					archModuleContext: archModuleContext{
						os:     deviceTarget.Os,
						target: deviceTarget,
					},
				},
				inVendorRamdisk: true,
			},
			in:           []string{"my_test"},
			out:          "target/product/test_device/vendor_ramdisk/system/my_test",
			partitionDir: "target/product/test_device/vendor_ramdisk/system",
		},
		{
			name: "vendor_ramdisk root binary",
			ctx: &testModuleInstallPathContext{
				baseModuleContext: baseModuleContext{
					archModuleContext: archModuleContext{
						os:     deviceTarget.Os,
						target: deviceTarget,
					},
				},
				inVendorRamdisk: true,
				inRoot:          true,
			},
			in:           []string{"my_test"},
			out:          "target/product/test_device/vendor_ramdisk/my_test",
			partitionDir: "target/product/test_device/vendor_ramdisk",
		},
		{
			name: "debug_ramdisk binary",
			ctx: &testModuleInstallPathContext{
				baseModuleContext: baseModuleContext{
					archModuleContext: archModuleContext{
						os:     deviceTarget.Os,
						target: deviceTarget,
					},
				},
				inDebugRamdisk: true,
			},
			in:           []string{"my_test"},
			out:          "target/product/test_device/debug_ramdisk/my_test",
			partitionDir: "target/product/test_device/debug_ramdisk",
		},
		{
			name: "system native test binary",
			ctx: &testModuleInstallPathContext{
				baseModuleContext: baseModuleContext{
					archModuleContext: archModuleContext{
						os:     deviceTarget.Os,
						target: deviceTarget,
					},
				},
				inData: true,
			},
			in:           []string{"nativetest", "my_test"},
			out:          "target/product/test_device/data/nativetest/my_test",
			partitionDir: "target/product/test_device/data",
		},
		{
			name: "vendor native test binary",
			ctx: &testModuleInstallPathContext{
				baseModuleContext: baseModuleContext{
					archModuleContext: archModuleContext{
						os:     deviceTarget.Os,
						target: deviceTarget,
					},
					earlyModuleContext: earlyModuleContext{
						kind: socSpecificModule,
					},
				},
				inData: true,
			},
			in:           []string{"nativetest", "my_test"},
			out:          "target/product/test_device/data/nativetest/my_test",
			partitionDir: "target/product/test_device/data",
		},
		{
			name: "odm native test binary",
			ctx: &testModuleInstallPathContext{
				baseModuleContext: baseModuleContext{
					archModuleContext: archModuleContext{
						os:     deviceTarget.Os,
						target: deviceTarget,
					},
					earlyModuleContext: earlyModuleContext{
						kind: deviceSpecificModule,
					},
				},
				inData: true,
			},
			in:           []string{"nativetest", "my_test"},
			out:          "target/product/test_device/data/nativetest/my_test",
			partitionDir: "target/product/test_device/data",
		},
		{
			name: "product native test binary",
			ctx: &testModuleInstallPathContext{
				baseModuleContext: baseModuleContext{
					archModuleContext: archModuleContext{
						os:     deviceTarget.Os,
						target: deviceTarget,
					},
					earlyModuleContext: earlyModuleContext{
						kind: productSpecificModule,
					},
				},
				inData: true,
			},
			in:           []string{"nativetest", "my_test"},
			out:          "target/product/test_device/data/nativetest/my_test",
			partitionDir: "target/product/test_device/data",
		},

		{
			name: "system_ext native test binary",
			ctx: &testModuleInstallPathContext{
				baseModuleContext: baseModuleContext{
					archModuleContext: archModuleContext{
						os:     deviceTarget.Os,
						target: deviceTarget,
					},
					earlyModuleContext: earlyModuleContext{
						kind: systemExtSpecificModule,
					},
				},
				inData: true,
			},
			in:           []string{"nativetest", "my_test"},
			out:          "target/product/test_device/data/nativetest/my_test",
			partitionDir: "target/product/test_device/data",
		},

		{
			name: "sanitized system binary",
			ctx: &testModuleInstallPathContext{
				baseModuleContext: baseModuleContext{
					archModuleContext: archModuleContext{
						os:     deviceTarget.Os,
						target: deviceTarget,
					},
				},
				inSanitizerDir: true,
			},
			in:           []string{"bin", "my_test"},
			out:          "target/product/test_device/data/asan/system/bin/my_test",
			partitionDir: "target/product/test_device/data/asan/system",
		},
		{
			name: "sanitized vendor binary",
			ctx: &testModuleInstallPathContext{
				baseModuleContext: baseModuleContext{
					archModuleContext: archModuleContext{
						os:     deviceTarget.Os,
						target: deviceTarget,
					},
					earlyModuleContext: earlyModuleContext{
						kind: socSpecificModule,
					},
				},
				inSanitizerDir: true,
			},
			in:           []string{"bin", "my_test"},
			out:          "target/product/test_device/data/asan/vendor/bin/my_test",
			partitionDir: "target/product/test_device/data/asan/vendor",
		},
		{
			name: "sanitized odm binary",
			ctx: &testModuleInstallPathContext{
				baseModuleContext: baseModuleContext{
					archModuleContext: archModuleContext{
						os:     deviceTarget.Os,
						target: deviceTarget,
					},
					earlyModuleContext: earlyModuleContext{
						kind: deviceSpecificModule,
					},
				},
				inSanitizerDir: true,
			},
			in:           []string{"bin", "my_test"},
			out:          "target/product/test_device/data/asan/odm/bin/my_test",
			partitionDir: "target/product/test_device/data/asan/odm",
		},
		{
			name: "sanitized product binary",
			ctx: &testModuleInstallPathContext{
				baseModuleContext: baseModuleContext{
					archModuleContext: archModuleContext{
						os:     deviceTarget.Os,
						target: deviceTarget,
					},
					earlyModuleContext: earlyModuleContext{
						kind: productSpecificModule,
					},
				},
				inSanitizerDir: true,
			},
			in:           []string{"bin", "my_test"},
			out:          "target/product/test_device/data/asan/product/bin/my_test",
			partitionDir: "target/product/test_device/data/asan/product",
		},

		{
			name: "sanitized system_ext binary",
			ctx: &testModuleInstallPathContext{
				baseModuleContext: baseModuleContext{
					archModuleContext: archModuleContext{
						os:     deviceTarget.Os,
						target: deviceTarget,
					},
					earlyModuleContext: earlyModuleContext{
						kind: systemExtSpecificModule,
					},
				},
				inSanitizerDir: true,
			},
			in:           []string{"bin", "my_test"},
			out:          "target/product/test_device/data/asan/system_ext/bin/my_test",
			partitionDir: "target/product/test_device/data/asan/system_ext",
		},

		{
			name: "sanitized system native test binary",
			ctx: &testModuleInstallPathContext{
				baseModuleContext: baseModuleContext{
					archModuleContext: archModuleContext{
						os:     deviceTarget.Os,
						target: deviceTarget,
					},
				},
				inData:         true,
				inSanitizerDir: true,
			},
			in:           []string{"nativetest", "my_test"},
			out:          "target/product/test_device/data/asan/data/nativetest/my_test",
			partitionDir: "target/product/test_device/data/asan/data",
		},
		{
			name: "sanitized vendor native test binary",
			ctx: &testModuleInstallPathContext{
				baseModuleContext: baseModuleContext{
					archModuleContext: archModuleContext{
						os:     deviceTarget.Os,
						target: deviceTarget,
					},
					earlyModuleContext: earlyModuleContext{
						kind: socSpecificModule,
					},
				},
				inData:         true,
				inSanitizerDir: true,
			},
			in:           []string{"nativetest", "my_test"},
			out:          "target/product/test_device/data/asan/data/nativetest/my_test",
			partitionDir: "target/product/test_device/data/asan/data",
		},
		{
			name: "sanitized odm native test binary",
			ctx: &testModuleInstallPathContext{
				baseModuleContext: baseModuleContext{
					archModuleContext: archModuleContext{
						os:     deviceTarget.Os,
						target: deviceTarget,
					},
					earlyModuleContext: earlyModuleContext{
						kind: deviceSpecificModule,
					},
				},
				inData:         true,
				inSanitizerDir: true,
			},
			in:           []string{"nativetest", "my_test"},
			out:          "target/product/test_device/data/asan/data/nativetest/my_test",
			partitionDir: "target/product/test_device/data/asan/data",
		},
		{
			name: "sanitized product native test binary",
			ctx: &testModuleInstallPathContext{
				baseModuleContext: baseModuleContext{
					archModuleContext: archModuleContext{
						os:     deviceTarget.Os,
						target: deviceTarget,
					},
					earlyModuleContext: earlyModuleContext{
						kind: productSpecificModule,
					},
				},
				inData:         true,
				inSanitizerDir: true,
			},
			in:           []string{"nativetest", "my_test"},
			out:          "target/product/test_device/data/asan/data/nativetest/my_test",
			partitionDir: "target/product/test_device/data/asan/data",
		},
		{
			name: "sanitized system_ext native test binary",
			ctx: &testModuleInstallPathContext{
				baseModuleContext: baseModuleContext{
					archModuleContext: archModuleContext{
						os:     deviceTarget.Os,
						target: deviceTarget,
					},
					earlyModuleContext: earlyModuleContext{
						kind: systemExtSpecificModule,
					},
				},
				inData:         true,
				inSanitizerDir: true,
			},
			in:           []string{"nativetest", "my_test"},
			out:          "target/product/test_device/data/asan/data/nativetest/my_test",
			partitionDir: "target/product/test_device/data/asan/data",
		}, {
			name: "device testcases",
			ctx: &testModuleInstallPathContext{
				baseModuleContext: baseModuleContext{
					archModuleContext: archModuleContext{
						os:     deviceTarget.Os,
						target: deviceTarget,
					},
				},
				inTestcases: true,
			},
			in:           []string{"my_test", "my_test_bin"},
			out:          "target/product/test_device/testcases/my_test/my_test_bin",
			partitionDir: "target/product/test_device/testcases",
		}, {
			name: "host testcases",
			ctx: &testModuleInstallPathContext{
				baseModuleContext: baseModuleContext{
					archModuleContext: archModuleContext{
						os:     hostTarget.Os,
						target: hostTarget,
					},
				},
				inTestcases: true,
			},
			in:           []string{"my_test", "my_test_bin"},
			out:          "host/linux-x86/testcases/my_test/my_test_bin",
			partitionDir: "host/linux-x86/testcases",
		}, {
			name: "forced host testcases",
			ctx: &testModuleInstallPathContext{
				baseModuleContext: baseModuleContext{
					archModuleContext: archModuleContext{
						os:     deviceTarget.Os,
						target: deviceTarget,
					},
				},
				inTestcases: true,
				forceOS:     &Linux,
				forceArch:   &X86,
			},
			in:           []string{"my_test", "my_test_bin"},
			out:          "host/linux-x86/testcases/my_test/my_test_bin",
			partitionDir: "host/linux-x86/testcases",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.ctx.baseModuleContext.config = testConfig
			output := PathForModuleInstall(tc.ctx, tc.in...)
			if output.basePath.path != tc.out {
				t.Errorf("unexpected path:\n got: %q\nwant: %q\n",
					output.basePath.path,
					tc.out)
			}
			if output.partitionDir != tc.partitionDir {
				t.Errorf("unexpected partitionDir:\n got: %q\nwant: %q\n",
					output.partitionDir, tc.partitionDir)
			}
		})
	}
}

func TestPathForModuleInstallRecoveryAsBoot(t *testing.T) {
	testConfig := pathTestConfig("")
	testConfig.TestProductVariables.BoardUsesRecoveryAsBoot = proptools.BoolPtr(true)
	testConfig.TestProductVariables.BoardMoveRecoveryResourcesToVendorBoot = proptools.BoolPtr(true)
	deviceTarget := Target{Os: Android, Arch: Arch{ArchType: Arm64}}

	testCases := []struct {
		name         string
		ctx          *testModuleInstallPathContext
		in           []string
		out          string
		partitionDir string
	}{
		{
			name: "ramdisk binary",
			ctx: &testModuleInstallPathContext{
				baseModuleContext: baseModuleContext{
					archModuleContext: archModuleContext{
						os:     deviceTarget.Os,
						target: deviceTarget,
					},
				},
				inRamdisk: true,
				inRoot:    true,
			},
			in:           []string{"my_test"},
			out:          "target/product/test_device/recovery/root/first_stage_ramdisk/my_test",
			partitionDir: "target/product/test_device/recovery/root/first_stage_ramdisk",
		},

		{
			name: "vendor_ramdisk binary",
			ctx: &testModuleInstallPathContext{
				baseModuleContext: baseModuleContext{
					archModuleContext: archModuleContext{
						os:     deviceTarget.Os,
						target: deviceTarget,
					},
				},
				inVendorRamdisk: true,
				inRoot:          true,
			},
			in:           []string{"my_test"},
			out:          "target/product/test_device/vendor_ramdisk/first_stage_ramdisk/my_test",
			partitionDir: "target/product/test_device/vendor_ramdisk/first_stage_ramdisk",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.ctx.baseModuleContext.config = testConfig
			output := PathForModuleInstall(tc.ctx, tc.in...)
			if output.basePath.path != tc.out {
				t.Errorf("unexpected path:\n got: %q\nwant: %q\n",
					output.basePath.path,
					tc.out)
			}
			if output.partitionDir != tc.partitionDir {
				t.Errorf("unexpected partitionDir:\n got: %q\nwant: %q\n",
					output.partitionDir, tc.partitionDir)
			}
		})
	}
}

func TestBaseDirForInstallPath(t *testing.T) {
	testConfig := pathTestConfig("")
	deviceTarget := Target{Os: Android, Arch: Arch{ArchType: Arm64}}

	ctx := &testModuleInstallPathContext{
		baseModuleContext: baseModuleContext{
			archModuleContext: archModuleContext{
				os:     deviceTarget.Os,
				target: deviceTarget,
			},
		},
	}
	ctx.baseModuleContext.config = testConfig

	actual := PathForModuleInstall(ctx, "foo", "bar")
	expectedBaseDir := "target/product/test_device/system"
	if actual.partitionDir != expectedBaseDir {
		t.Errorf("unexpected partitionDir:\n got: %q\nwant: %q\n", actual.partitionDir, expectedBaseDir)
	}
	expectedRelPath := "foo/bar"
	if actual.Rel() != expectedRelPath {
		t.Errorf("unexpected Rel():\n got: %q\nwant: %q\n", actual.Rel(), expectedRelPath)
	}

	actualAfterJoin := actual.Join(ctx, "baz")
	// partitionDir is preserved even after joining
	if actualAfterJoin.partitionDir != expectedBaseDir {
		t.Errorf("unexpected partitionDir after joining:\n got: %q\nwant: %q\n", actualAfterJoin.partitionDir, expectedBaseDir)
	}
	// Rel() is updated though
	expectedRelAfterJoin := "baz"
	if actualAfterJoin.Rel() != expectedRelAfterJoin {
		t.Errorf("unexpected Rel() after joining:\n got: %q\nwant: %q\n", actualAfterJoin.Rel(), expectedRelAfterJoin)
	}
}

func TestDirectorySortedPaths(t *testing.T) {
	config := TestConfig("out", nil, "", map[string][]byte{
		"Android.bp": nil,
		"a.txt":      nil,
		"a/txt":      nil,
		"a/b/c":      nil,
		"a/b/d":      nil,
		"b":          nil,
		"b/b.txt":    nil,
		"a/a.txt":    nil,
	})

	ctx := PathContextForTesting(config)

	makePaths := func() Paths {
		return Paths{
			PathForSource(ctx, "a.txt"),
			PathForSource(ctx, "a/txt"),
			PathForSource(ctx, "a/b/c"),
			PathForSource(ctx, "a/b/d"),
			PathForSource(ctx, "b"),
			PathForSource(ctx, "b/b.txt"),
			PathForSource(ctx, "a/a.txt"),
		}
	}

	expected := []string{
		"a.txt",
		"a/a.txt",
		"a/b/c",
		"a/b/d",
		"a/txt",
		"b",
		"b/b.txt",
	}

	paths := makePaths()
	reversePaths := ReversePaths(paths)

	sortedPaths := PathsToDirectorySortedPaths(paths)
	reverseSortedPaths := PathsToDirectorySortedPaths(reversePaths)

	if !reflect.DeepEqual(Paths(sortedPaths).Strings(), expected) {
		t.Fatalf("sorted paths:\n %#v\n != \n %#v", paths.Strings(), expected)
	}

	if !reflect.DeepEqual(Paths(reverseSortedPaths).Strings(), expected) {
		t.Fatalf("sorted reversed paths:\n %#v\n !=\n %#v", reversePaths.Strings(), expected)
	}

	expectedA := []string{
		"a/a.txt",
		"a/b/c",
		"a/b/d",
		"a/txt",
	}

	inA := sortedPaths.PathsInDirectory("a")
	if !reflect.DeepEqual(inA.Strings(), expectedA) {
		t.Errorf("FilesInDirectory(a):\n %#v\n != \n %#v", inA.Strings(), expectedA)
	}

	expectedA_B := []string{
		"a/b/c",
		"a/b/d",
	}

	inA_B := sortedPaths.PathsInDirectory("a/b")
	if !reflect.DeepEqual(inA_B.Strings(), expectedA_B) {
		t.Errorf("FilesInDirectory(a/b):\n %#v\n != \n %#v", inA_B.Strings(), expectedA_B)
	}

	expectedB := []string{
		"b/b.txt",
	}

	inB := sortedPaths.PathsInDirectory("b")
	if !reflect.DeepEqual(inB.Strings(), expectedB) {
		t.Errorf("FilesInDirectory(b):\n %#v\n != \n %#v", inA.Strings(), expectedA)
	}
}

func TestMaybeRel(t *testing.T) {
	testCases := []struct {
		name   string
		base   string
		target string
		out    string
		isRel  bool
	}{
		{
			name:   "normal",
			base:   "a/b/c",
			target: "a/b/c/d",
			out:    "d",
			isRel:  true,
		},
		{
			name:   "parent",
			base:   "a/b/c/d",
			target: "a/b/c",
			isRel:  false,
		},
		{
			name:   "not relative",
			base:   "a/b",
			target: "c/d",
			isRel:  false,
		},
		{
			name:   "abs1",
			base:   "/a",
			target: "a",
			isRel:  false,
		},
		{
			name:   "abs2",
			base:   "a",
			target: "/a",
			isRel:  false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := &configErrorWrapper{}
			out, isRel := MaybeRel(ctx, testCase.base, testCase.target)
			if len(ctx.errors) > 0 {
				t.Errorf("MaybeRel(..., %s, %s) reported unexpected errors %v",
					testCase.base, testCase.target, ctx.errors)
			}
			if isRel != testCase.isRel || out != testCase.out {
				t.Errorf("MaybeRel(..., %s, %s) want %v, %v got %v, %v",
					testCase.base, testCase.target, testCase.out, testCase.isRel, out, isRel)
			}
		})
	}
}

func TestPathForSource(t *testing.T) {
	testCases := []struct {
		name     string
		buildDir string
		src      string
		err      string
	}{
		{
			name:     "normal",
			buildDir: "out",
			src:      "a/b/c",
		},
		{
			name:     "abs",
			buildDir: "out",
			src:      "/a/b/c",
			err:      "is outside directory",
		},
		{
			name:     "in out dir",
			buildDir: "out",
			src:      "out/soong/a/b/c",
			err:      "is in output",
		},
	}

	funcs := []struct {
		name string
		f    func(ctx PathContext, pathComponents ...string) (SourcePath, error)
	}{
		{"pathForSource", pathForSource},
		{"safePathForSource", safePathForSource},
	}

	for _, f := range funcs {
		t.Run(f.name, func(t *testing.T) {
			for _, test := range testCases {
				t.Run(test.name, func(t *testing.T) {
					testConfig := pathTestConfig(test.buildDir)
					ctx := &configErrorWrapper{config: testConfig}
					_, err := f.f(ctx, test.src)
					if len(ctx.errors) > 0 {
						t.Fatalf("unexpected errors %v", ctx.errors)
					}
					if err != nil {
						if test.err == "" {
							t.Fatalf("unexpected error %q", err.Error())
						} else if !strings.Contains(err.Error(), test.err) {
							t.Fatalf("incorrect error, want substring %q got %q", test.err, err.Error())
						}
					} else {
						if test.err != "" {
							t.Fatalf("missing error %q", test.err)
						}
					}
				})
			}
		})
	}
}

type pathForModuleSrcTestModule struct {
	ModuleBase
	props struct {
		Srcs         []string `android:"path"`
		Exclude_srcs []string `android:"path"`

		Src *string `android:"path"`

		Module_handles_missing_deps bool
	}

	src string
	rel string

	srcs []string
	rels []string

	missingDeps []string
}

func pathForModuleSrcTestModuleFactory() Module {
	module := &pathForModuleSrcTestModule{}
	module.AddProperties(&module.props)
	InitAndroidModule(module)
	return module
}

func (p *pathForModuleSrcTestModule) GenerateAndroidBuildActions(ctx ModuleContext) {
	var srcs Paths
	if p.props.Module_handles_missing_deps {
		srcs, p.missingDeps = PathsAndMissingDepsForModuleSrcExcludes(ctx, p.props.Srcs, p.props.Exclude_srcs)
	} else {
		srcs = PathsForModuleSrcExcludes(ctx, p.props.Srcs, p.props.Exclude_srcs)
	}
	p.srcs = srcs.Strings()

	for _, src := range srcs {
		p.rels = append(p.rels, src.Rel())
	}

	if p.props.Src != nil {
		src := PathForModuleSrc(ctx, *p.props.Src)
		if src != nil {
			p.src = src.String()
			p.rel = src.Rel()
		}
	}

	if !p.props.Module_handles_missing_deps {
		p.missingDeps = ctx.GetMissingDependencies()
	}

	ctx.Build(pctx, BuildParams{
		Rule:   Touch,
		Output: PathForModuleOut(ctx, "output"),
	})
}

type pathForModuleSrcOutputFileProviderModule struct {
	ModuleBase
	props struct {
		Outs   []string
		Tagged []string
	}

	outs   Paths
	tagged Paths
}

func pathForModuleSrcOutputFileProviderModuleFactory() Module {
	module := &pathForModuleSrcOutputFileProviderModule{}
	module.AddProperties(&module.props)
	InitAndroidModule(module)
	return module
}

func (p *pathForModuleSrcOutputFileProviderModule) GenerateAndroidBuildActions(ctx ModuleContext) {
	for _, out := range p.props.Outs {
		p.outs = append(p.outs, PathForModuleOut(ctx, out))
	}

	for _, tagged := range p.props.Tagged {
		p.tagged = append(p.tagged, PathForModuleOut(ctx, tagged))
	}
}

func (p *pathForModuleSrcOutputFileProviderModule) OutputFiles(tag string) (Paths, error) {
	switch tag {
	case "":
		return p.outs, nil
	case ".tagged":
		return p.tagged, nil
	default:
		return nil, fmt.Errorf("unsupported tag %q", tag)
	}
}

type pathForModuleSrcTestCase struct {
	name string
	bp   string
	srcs []string
	rels []string
	src  string
	rel  string

	// Make test specific preparations to the test fixture.
	preparer FixturePreparer

	// A test specific error handler.
	errorHandler FixtureErrorHandler
}

func testPathForModuleSrc(t *testing.T, tests []pathForModuleSrcTestCase) {
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fgBp := `
				filegroup {
					name: "a",
					srcs: ["src/a"],
				}
			`

			ofpBp := `
				output_file_provider {
					name: "b",
					outs: ["gen/b"],
					tagged: ["gen/c"],
				}
			`

			mockFS := MockFS{
				"fg/Android.bp":     []byte(fgBp),
				"foo/Android.bp":    []byte(test.bp),
				"ofp/Android.bp":    []byte(ofpBp),
				"fg/src/a":          nil,
				"foo/src/b":         nil,
				"foo/src/c":         nil,
				"foo/src/d":         nil,
				"foo/src/e/e":       nil,
				"foo/src_special/$": nil,
			}

			errorHandler := test.errorHandler
			if errorHandler == nil {
				errorHandler = FixtureExpectsNoErrors
			}

			result := GroupFixturePreparers(
				FixtureRegisterWithContext(func(ctx RegistrationContext) {
					ctx.RegisterModuleType("test", pathForModuleSrcTestModuleFactory)
					ctx.RegisterModuleType("output_file_provider", pathForModuleSrcOutputFileProviderModuleFactory)
				}),
				PrepareForTestWithFilegroup,
				PrepareForTestWithNamespace,
				mockFS.AddToFixture(),
				OptionalFixturePreparer(test.preparer),
			).
				ExtendWithErrorHandler(errorHandler).
				RunTest(t)

			m := result.ModuleForTests("foo", "").Module().(*pathForModuleSrcTestModule)

			AssertStringPathsRelativeToTopEquals(t, "srcs", result.Config, test.srcs, m.srcs)
			AssertStringPathsRelativeToTopEquals(t, "rels", result.Config, test.rels, m.rels)
			AssertStringPathRelativeToTopEquals(t, "src", result.Config, test.src, m.src)
			AssertStringPathRelativeToTopEquals(t, "rel", result.Config, test.rel, m.rel)
		})
	}
}

func TestPathsForModuleSrc(t *testing.T) {
	tests := []pathForModuleSrcTestCase{
		{
			name: "path",
			bp: `
			test {
				name: "foo",
				srcs: ["src/b"],
			}`,
			srcs: []string{"foo/src/b"},
			rels: []string{"src/b"},
		},
		{
			name: "glob",
			bp: `
			test {
				name: "foo",
				srcs: [
					"src/*",
					"src/e/*",
				],
			}`,
			srcs: []string{"foo/src/b", "foo/src/c", "foo/src/d", "foo/src/e/e"},
			rels: []string{"src/b", "src/c", "src/d", "src/e/e"},
		},
		{
			name: "recursive glob",
			bp: `
			test {
				name: "foo",
				srcs: ["src/**/*"],
			}`,
			srcs: []string{"foo/src/b", "foo/src/c", "foo/src/d", "foo/src/e/e"},
			rels: []string{"src/b", "src/c", "src/d", "src/e/e"},
		},
		{
			name: "filegroup",
			bp: `
			test {
				name: "foo",
				srcs: [":a"],
			}`,
			srcs: []string{"fg/src/a"},
			rels: []string{"src/a"},
		},
		{
			name: "output file provider",
			bp: `
			test {
				name: "foo",
				srcs: [":b"],
			}`,
			srcs: []string{"out/soong/.intermediates/ofp/b/gen/b"},
			rels: []string{"gen/b"},
		},
		{
			name: "output file provider tagged",
			bp: `
			test {
				name: "foo",
				srcs: [":b{.tagged}"],
			}`,
			srcs: []string{"out/soong/.intermediates/ofp/b/gen/c"},
			rels: []string{"gen/c"},
		},
		{
			name: "output file provider with exclude",
			bp: `
			test {
				name: "foo",
				srcs: [":b", ":c"],
				exclude_srcs: [":c"]
			}
			output_file_provider {
				name: "c",
				outs: ["gen/c"],
			}`,
			srcs: []string{"out/soong/.intermediates/ofp/b/gen/b"},
			rels: []string{"gen/b"},
		},
		{
			name: "special characters glob",
			bp: `
			test {
				name: "foo",
				srcs: ["src_special/*"],
			}`,
			srcs: []string{"foo/src_special/$"},
			rels: []string{"src_special/$"},
		},
	}

	testPathForModuleSrc(t, tests)
}

func TestPathForModuleSrc(t *testing.T) {
	tests := []pathForModuleSrcTestCase{
		{
			name: "path",
			bp: `
			test {
				name: "foo",
				src: "src/b",
			}`,
			src: "foo/src/b",
			rel: "src/b",
		},
		{
			name: "glob",
			bp: `
			test {
				name: "foo",
				src: "src/e/*",
			}`,
			src: "foo/src/e/e",
			rel: "src/e/e",
		},
		{
			name: "filegroup",
			bp: `
			test {
				name: "foo",
				src: ":a",
			}`,
			src: "fg/src/a",
			rel: "src/a",
		},
		{
			name: "output file provider",
			bp: `
			test {
				name: "foo",
				src: ":b",
			}`,
			src: "out/soong/.intermediates/ofp/b/gen/b",
			rel: "gen/b",
		},
		{
			name: "output file provider tagged",
			bp: `
			test {
				name: "foo",
				src: ":b{.tagged}",
			}`,
			src: "out/soong/.intermediates/ofp/b/gen/c",
			rel: "gen/c",
		},
		{
			name: "special characters glob",
			bp: `
			test {
				name: "foo",
				src: "src_special/*",
			}`,
			src: "foo/src_special/$",
			rel: "src_special/$",
		},
		{
			// This test makes sure that an unqualified module name cannot contain characters that make
			// it appear as a qualified module name.
			name: "output file provider, invalid fully qualified name",
			bp: `
			test {
				name: "foo",
				src: "://other:b",
				srcs: ["://other:c"],
			}`,
			preparer: FixtureAddTextFile("other/Android.bp", `
				soong_namespace {}

				output_file_provider {
					name: "b",
					outs: ["gen/b"],
				}

				output_file_provider {
					name: "c",
					outs: ["gen/c"],
				}
			`),
			src:  "foo/:/other:b",
			rel:  ":/other:b",
			srcs: []string{"foo/:/other:c"},
			rels: []string{":/other:c"},
		},
		{
			name: "output file provider, missing fully qualified name",
			bp: `
			test {
				name: "foo",
				src: "//other:b",
				srcs: ["//other:c"],
			}`,
			errorHandler: FixtureExpectsAllErrorsToMatchAPattern([]string{
				`"foo" depends on undefined module "//other:b"`,
				`"foo" depends on undefined module "//other:c"`,
			}),
		},
		{
			name: "output file provider, fully qualified name",
			bp: `
			test {
				name: "foo",
				src: "//other:b",
				srcs: ["//other:c"],
			}`,
			src:  "out/soong/.intermediates/other/b/gen/b",
			rel:  "gen/b",
			srcs: []string{"out/soong/.intermediates/other/c/gen/c"},
			rels: []string{"gen/c"},
			preparer: FixtureAddTextFile("other/Android.bp", `
				soong_namespace {}

				output_file_provider {
					name: "b",
					outs: ["gen/b"],
				}

				output_file_provider {
					name: "c",
					outs: ["gen/c"],
				}
			`),
		},
	}

	testPathForModuleSrc(t, tests)
}

func TestPathsForModuleSrc_AllowMissingDependencies(t *testing.T) {
	bp := `
		test {
			name: "foo",
			srcs: [":a"],
			exclude_srcs: [":b"],
			src: ":c",
		}

		test {
			name: "bar",
			srcs: [":d"],
			exclude_srcs: [":e"],
			module_handles_missing_deps: true,
		}
	`

	result := GroupFixturePreparers(
		PrepareForTestWithAllowMissingDependencies,
		FixtureRegisterWithContext(func(ctx RegistrationContext) {
			ctx.RegisterModuleType("test", pathForModuleSrcTestModuleFactory)
		}),
		FixtureWithRootAndroidBp(bp),
	).RunTest(t)

	foo := result.ModuleForTests("foo", "").Module().(*pathForModuleSrcTestModule)

	AssertArrayString(t, "foo missing deps", []string{"a", "b", "c"}, foo.missingDeps)
	AssertArrayString(t, "foo srcs", []string{}, foo.srcs)
	AssertStringEquals(t, "foo src", "", foo.src)

	bar := result.ModuleForTests("bar", "").Module().(*pathForModuleSrcTestModule)

	AssertArrayString(t, "bar missing deps", []string{"d", "e"}, bar.missingDeps)
	AssertArrayString(t, "bar srcs", []string{}, bar.srcs)
}

func TestPathRelativeToTop(t *testing.T) {
	testConfig := pathTestConfig("/tmp/build/top")
	deviceTarget := Target{Os: Android, Arch: Arch{ArchType: Arm64}}

	ctx := &testModuleInstallPathContext{
		baseModuleContext: baseModuleContext{
			archModuleContext: archModuleContext{
				os:     deviceTarget.Os,
				target: deviceTarget,
			},
		},
	}
	ctx.baseModuleContext.config = testConfig

	t.Run("install for soong", func(t *testing.T) {
		p := PathForModuleInstall(ctx, "install/path")
		AssertPathRelativeToTopEquals(t, "install path for soong", "out/soong/target/product/test_device/system/install/path", p)
	})
	t.Run("install for make", func(t *testing.T) {
		p := PathForModuleInstall(ctx, "install/path")
		p.makePath = true
		AssertPathRelativeToTopEquals(t, "install path for make", "out/target/product/test_device/system/install/path", p)
	})
	t.Run("output", func(t *testing.T) {
		p := PathForOutput(ctx, "output/path")
		AssertPathRelativeToTopEquals(t, "output path", "out/soong/output/path", p)
	})
	t.Run("source", func(t *testing.T) {
		p := PathForSource(ctx, "source/path")
		AssertPathRelativeToTopEquals(t, "source path", "source/path", p)
	})
	t.Run("mixture", func(t *testing.T) {
		paths := Paths{
			PathForModuleInstall(ctx, "install/path"),
			PathForOutput(ctx, "output/path"),
			PathForSource(ctx, "source/path"),
		}

		expected := []string{
			"out/soong/target/product/test_device/system/install/path",
			"out/soong/output/path",
			"source/path",
		}
		AssertPathsRelativeToTopEquals(t, "mixture", expected, paths)
	})
}

func ExampleOutputPath_ReplaceExtension() {
	ctx := &configErrorWrapper{
		config: TestConfig("out", nil, "", nil),
	}
	p := PathForOutput(ctx, "system/framework").Join(ctx, "boot.art")
	p2 := p.ReplaceExtension(ctx, "oat")
	fmt.Println(p, p2)
	fmt.Println(p.Rel(), p2.Rel())

	// Output:
	// out/soong/system/framework/boot.art out/soong/system/framework/boot.oat
	// boot.art boot.oat
}

func ExampleOutputPath_InSameDir() {
	ctx := &configErrorWrapper{
		config: TestConfig("out", nil, "", nil),
	}
	p := PathForOutput(ctx, "system/framework").Join(ctx, "boot.art")
	p2 := p.InSameDir(ctx, "oat", "arm", "boot.vdex")
	fmt.Println(p, p2)
	fmt.Println(p.Rel(), p2.Rel())

	// Output:
	// out/soong/system/framework/boot.art out/soong/system/framework/oat/arm/boot.vdex
	// boot.art oat/arm/boot.vdex
}

func BenchmarkFirstUniquePaths(b *testing.B) {
	implementations := []struct {
		name string
		f    func(Paths) Paths
	}{
		{
			name: "list",
			f:    firstUniquePathsList,
		},
		{
			name: "map",
			f:    firstUniquePathsMap,
		},
	}
	const maxSize = 1024
	uniquePaths := make(Paths, maxSize)
	for i := range uniquePaths {
		uniquePaths[i] = PathForTesting(strconv.Itoa(i))
	}
	samePath := make(Paths, maxSize)
	for i := range samePath {
		samePath[i] = uniquePaths[0]
	}

	f := func(b *testing.B, imp func(Paths) Paths, paths Paths) {
		for i := 0; i < b.N; i++ {
			b.ReportAllocs()
			paths = append(Paths(nil), paths...)
			imp(paths)
		}
	}

	for n := 1; n <= maxSize; n <<= 1 {
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			for _, implementation := range implementations {
				b.Run(implementation.name, func(b *testing.B) {
					b.Run("same", func(b *testing.B) {
						f(b, implementation.f, samePath[:n])
					})
					b.Run("unique", func(b *testing.B) {
						f(b, implementation.f, uniquePaths[:n])
					})
				})
			}
		})
	}
}
