// Copyright 2016 Google Inc. All rights reserved.
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

package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"android/soong/bpfix/bpfix"
)

var testCases = []struct {
	desc     string
	in       string
	expected string
}{
	{
		desc: "basic cc_library_shared with comments",
		in: `
#
# Copyright
#

# Module Comment
include $(CLEAR_VARS)
# Name Comment
LOCAL_MODULE := test
# Source comment
LOCAL_SRC_FILES_EXCLUDE := a.c
# Second source comment
LOCAL_SRC_FILES_EXCLUDE += b.c
include $(BUILD_SHARED_LIBRARY)`,
		expected: `
//
// Copyright
//

// Module Comment
cc_library_shared {
    // Name Comment
    name: "test",
    // Source comment
    exclude_srcs: ["a.c"] + ["b.c"], // Second source comment

}`,
	},
	{
		desc: "split local/global include_dirs (1)",
		in: `
include $(CLEAR_VARS)
LOCAL_C_INCLUDES := $(LOCAL_PATH)
include $(BUILD_SHARED_LIBRARY)`,
		expected: `
cc_library_shared {
    local_include_dirs: ["."],
}`,
	},
	{
		desc: "split local/global include_dirs (2)",
		in: `
include $(CLEAR_VARS)
LOCAL_C_INCLUDES := $(LOCAL_PATH)/include
include $(BUILD_SHARED_LIBRARY)`,
		expected: `
cc_library_shared {
    local_include_dirs: ["include"],
}`,
	},
	{
		desc: "split local/global include_dirs (3)",
		in: `
include $(CLEAR_VARS)
LOCAL_C_INCLUDES := system/core/include
include $(BUILD_SHARED_LIBRARY)`,
		expected: `
cc_library_shared {
    include_dirs: ["system/core/include"],
}`,
	},
	{
		desc: "split local/global include_dirs (4)",
		in: `
input := testing/include
include $(CLEAR_VARS)
# Comment 1
LOCAL_C_INCLUDES := $(LOCAL_PATH) $(LOCAL_PATH)/include system/core/include $(input)
# Comment 2
LOCAL_C_INCLUDES += $(TOP)/system/core/include $(LOCAL_PATH)/test/include
# Comment 3
include $(BUILD_SHARED_LIBRARY)`,
		expected: `
input = ["testing/include"]
cc_library_shared {
    // Comment 1
    include_dirs: ["system/core/include"] + input + ["system/core/include"], // Comment 2
    local_include_dirs: ["."] + ["include"] + ["test/include"],
    // Comment 3
}`,
	},
	{
		desc: "LOCAL_MODULE_STEM",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE := libtest
LOCAL_MODULE_STEM := $(LOCAL_MODULE).so
include $(BUILD_SHARED_LIBRARY)

include $(CLEAR_VARS)
LOCAL_MODULE := libtest2
LOCAL_MODULE_STEM := testing.so
include $(BUILD_SHARED_LIBRARY)
`,
		expected: `
cc_library_shared {
    name: "libtest",
    suffix: ".so",
}

cc_library_shared {
    name: "libtest2",
    stem: "testing.so",
}
`,
	},
	{
		desc: "LOCAL_MODULE_HOST_OS",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE := libtest
LOCAL_MODULE_HOST_OS := linux darwin windows
include $(BUILD_SHARED_LIBRARY)

include $(CLEAR_VARS)
LOCAL_MODULE := libtest2
LOCAL_MODULE_HOST_OS := linux
include $(BUILD_SHARED_LIBRARY)
`,
		expected: `
cc_library_shared {
    name: "libtest",
    target: {
        windows: {
            enabled: true,
        }
    }
}

cc_library_shared {
    name: "libtest2",
    target: {
        darwin: {
            enabled: false,
        }
    }
}
`,
	},
	{
		desc: "LOCAL_RTTI_VALUE",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE := libtest
LOCAL_RTTI_FLAG := # Empty flag
include $(BUILD_SHARED_LIBRARY)

include $(CLEAR_VARS)
LOCAL_MODULE := libtest2
LOCAL_RTTI_FLAG := -frtti
include $(BUILD_SHARED_LIBRARY)
`,
		expected: `
cc_library_shared {
    name: "libtest",
    rtti: false, // Empty flag
}

cc_library_shared {
    name: "libtest2",
    rtti: true,
}
`,
	},
	{
		desc: "LOCAL_ARM_MODE",
		in: `
include $(CLEAR_VARS)
LOCAL_ARM_MODE := arm
include $(BUILD_SHARED_LIBRARY)
`,
		expected: `
cc_library_shared {
    arch: {
        arm: {
            instruction_set: "arm",
        },
    },
}
`,
	},
	{
		desc: "_<OS> suffixes",
		in: `
include $(CLEAR_VARS)
LOCAL_SRC_FILES_darwin := darwin.c
LOCAL_SRC_FILES_linux := linux.c
LOCAL_SRC_FILES_windows := windows.c
include $(BUILD_SHARED_LIBRARY)
`,
		expected: `
cc_library_shared {
    target: {
        darwin: {
            srcs: ["darwin.c"],
        },
        linux_glibc: {
            srcs: ["linux.c"],
        },
        windows: {
            srcs: ["windows.c"],
        },
    },
}
`,
	},
	{
		desc: "LOCAL_SANITIZE := never",
		in: `
include $(CLEAR_VARS)
LOCAL_SANITIZE := never
include $(BUILD_SHARED_LIBRARY)
`,
		expected: `
cc_library_shared {
    sanitize: {
        never: true,
    },
}
`,
	},
	{
		desc: "LOCAL_SANITIZE unknown parameter",
		in: `
include $(CLEAR_VARS)
LOCAL_SANITIZE := thread cfi asdf
LOCAL_SANITIZE_DIAG := cfi
LOCAL_SANITIZE_RECOVER := qwert
include $(BUILD_SHARED_LIBRARY)
`,
		expected: `
cc_library_shared {
    sanitize: {
        thread: true,
        cfi: true,
        misc_undefined: ["asdf"],
        diag: {
            cfi: true,
        },
        recover: ["qwert"],
    },
}
`,
	},
	{
		desc: "LOCAL_SANITIZE_RECOVER",
		in: `
include $(CLEAR_VARS)
LOCAL_SANITIZE_RECOVER := shift-exponent
include $(BUILD_SHARED_LIBRARY)
`,
		expected: `
cc_library_shared {
    sanitize: {
	recover: ["shift-exponent"],
    },
}
`,
	},
	{
		desc: "version_script in LOCAL_LDFLAGS",
		in: `
include $(CLEAR_VARS)
LOCAL_LDFLAGS := -Wl,--link-opt -Wl,--version-script,$(LOCAL_PATH)/exported32.map
include $(BUILD_SHARED_LIBRARY)
`,
		expected: `
cc_library_shared {
    ldflags: ["-Wl,--link-opt"],
    version_script: "exported32.map",
}
`,
	},
	{
		desc: "Handle TOP",
		in: `
include $(CLEAR_VARS)
LOCAL_C_INCLUDES := $(TOP)/system/core/include $(TOP)
include $(BUILD_SHARED_LIBRARY)
`,
		expected: `
cc_library_shared {
	include_dirs: ["system/core/include", "."],
}
`,
	},
	{
		desc: "Remove LOCAL_MODULE_TAGS optional",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE_TAGS := optional
include $(BUILD_SHARED_LIBRARY)
`,

		expected: `
cc_library_shared {

}
`,
	},
	{
		desc: "Keep LOCAL_MODULE_TAGS non-optional",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE_TAGS := debug
include $(BUILD_SHARED_LIBRARY)
`,

		expected: `
cc_library_shared {
	tags: ["debug"],
}
`,
	},

	{
		desc: "Input containing escaped quotes",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE:= libsensorservice
LOCAL_CFLAGS:= -DLOG_TAG=\"-DDontEscapeMe\"
LOCAL_SRC_FILES := \"EscapeMe.cc\"
include $(BUILD_SHARED_LIBRARY)
`,

		expected: `
cc_library_shared {
    name: "libsensorservice",
    cflags: ["-DLOG_TAG=\"-DDontEscapeMe\""],
    srcs: ["\\\"EscapeMe.cc\\\""],
}
`,
	},
	{

		desc: "Don't fail on missing CLEAR_VARS",
		in: `
LOCAL_MODULE := iAmAModule
include $(BUILD_SHARED_LIBRARY)`,

		expected: `
// ANDROIDMK TRANSLATION WARNING: No 'include $(CLEAR_VARS)' detected before first assignment; clearing vars now
cc_library_shared {
  name: "iAmAModule",

}`,
	},
	{

		desc: "LOCAL_AIDL_INCLUDES",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE := iAmAModule
LOCAL_AIDL_INCLUDES := $(LOCAL_PATH)/src/main/java system/core
include $(BUILD_SHARED_LIBRARY)`,

		expected: `
cc_library_shared {
  name: "iAmAModule",
  aidl: {
    include_dirs: ["system/core"],
    local_include_dirs: ["src/main/java"],
  }
}`,
	},
	{
		// the important part of this test case is that it confirms that androidmk doesn't
		// panic in this case
		desc: "multiple directives inside recipe",
		in: `
ifeq ($(a),true)
ifeq ($(b),false)
imABuildStatement: somefile
	echo begin
endif # a==true
	echo middle
endif # b==false
	echo end
`,
		expected: `
// ANDROIDMK TRANSLATION ERROR: unsupported conditional
// ifeq ($(a),true)

// ANDROIDMK TRANSLATION ERROR: unsupported conditional
// ifeq ($(b),false)

// ANDROIDMK TRANSLATION ERROR: unsupported line
// rule:       imABuildStatement: somefile
// echo begin
//  # a==true
// echo middle
//  # b==false
// echo end
//
// ANDROIDMK TRANSLATION ERROR: endif from unsupported contitional
// endif
// ANDROIDMK TRANSLATION ERROR: endif from unsupported contitional
// endif
		`,
	},
	{
		desc: "ignore all-makefiles-under",
		in: `
include $(call all-makefiles-under,$(LOCAL_PATH))
`,
		expected: ``,
	},
	{
		desc: "proguard options for java library",
		in: `
			include $(CLEAR_VARS)
			# Empty
			LOCAL_PROGUARD_ENABLED :=
			# Disabled
			LOCAL_PROGUARD_ENABLED := disabled
			# Full
			LOCAL_PROGUARD_ENABLED := full
			# Obfuscation and optimization
			LOCAL_PROGUARD_ENABLED := obfuscation optimization
			# Custom
			LOCAL_PROGUARD_ENABLED := custom
			include $(BUILD_JAVA_LIBRARY)
		`,
		expected: `
			java_library {
				// Empty

				// Disabled
				optimize: {
					enabled: false,
					// Full
					enabled: true,
					// Obfuscation and optimization
					obfuscate: true,
					optimize: true,
					enabled: true,
					// Custom
					no_aapt_flags: true,
					enabled: true,
				},
			}
		`,
	},
	{
		desc: "errorprone options for java library",
		in: `
			include $(CLEAR_VARS)
			LOCAL_ERROR_PRONE_FLAGS := -Xep:AsyncCallableReturnsNull:ERROR -Xep:AsyncFunctionReturnsNull:ERROR
			include $(BUILD_JAVA_LIBRARY)
		`,
		expected: `
			java_library {
				errorprone: {
					javacflags: [
						"-Xep:AsyncCallableReturnsNull:ERROR",
						"-Xep:AsyncFunctionReturnsNull:ERROR",
					],
				},
			}
		`,
	},
	{
		desc: "java prebuilt",
		in: `
			include $(CLEAR_VARS)
			LOCAL_SRC_FILES := test.jar
			LOCAL_MODULE_CLASS := JAVA_LIBRARIES
			LOCAL_STATIC_ANDROID_LIBRARIES :=
			include $(BUILD_PREBUILT)
		`,
		expected: `
			java_import {
				jars: ["test.jar"],

			}
		`,
	},
	{
		desc: "aar prebuilt",
		in: `
			include $(CLEAR_VARS)
			LOCAL_SRC_FILES := test.aar
			LOCAL_MODULE_CLASS := JAVA_LIBRARIES
			include $(BUILD_PREBUILT)
		`,
		expected: `
			android_library_import {
				aars: ["test.aar"],

			}
		`,
	},

	{
		desc: "aar",
		in: `
			include $(CLEAR_VARS)
			LOCAL_SRC_FILES := test.java
			LOCAL_RESOURCE_DIR := res
			include $(BUILD_STATIC_JAVA_LIBRARY)

			include $(CLEAR_VARS)
			LOCAL_SRC_FILES := test.java
			LOCAL_STATIC_LIBRARIES := foo
			LOCAL_STATIC_ANDROID_LIBRARIES := bar
			include $(BUILD_STATIC_JAVA_LIBRARY)

			include $(CLEAR_VARS)
			LOCAL_SRC_FILES := test.java
			LOCAL_SHARED_LIBRARIES := foo
			LOCAL_SHARED_ANDROID_LIBRARIES := bar
			include $(BUILD_STATIC_JAVA_LIBRARY)

			include $(CLEAR_VARS)
			LOCAL_SRC_FILES := test.java
			LOCAL_STATIC_ANDROID_LIBRARIES :=
			include $(BUILD_STATIC_JAVA_LIBRARY)
		`,
		expected: `
			android_library {
				srcs: ["test.java"],
				resource_dirs: ["res"],
			}

			android_library {
				srcs: ["test.java"],
				static_libs: [
					"foo",
					"bar",
				],
			}

			android_library {
				srcs: ["test.java"],
				libs: [
					"foo",
					"bar",
				],
			}

			java_library_static {
				srcs: ["test.java"],
				static_libs: [],
			}
		`,
	},
	{
		desc: "cc_library shared_libs",
		in: `
			include $(CLEAR_VARS)
			LOCAL_SHARED_LIBRARIES := libfoo
			include $(BUILD_SHARED_LIBRARY)
		`,
		expected: `
			cc_library_shared {
				shared_libs: ["libfoo"],
			}
		`,
	},
}

func TestEndToEnd(t *testing.T) {
	for i, test := range testCases {
		expected, err := bpfix.Reformat(test.expected)
		if err != nil {
			t.Error(err)
		}

		got, errs := convertFile(fmt.Sprintf("<testcase %d>", i), bytes.NewBufferString(test.in))
		if len(errs) > 0 {
			t.Errorf("Unexpected errors: %q", errs)
			continue
		}

		if got != expected {
			t.Errorf("failed testcase '%s'\ninput:\n%s\n\nexpected:\n%s\ngot:\n%s\n", test.desc, strings.TrimSpace(test.in), expected, got)
		}
	}
}
