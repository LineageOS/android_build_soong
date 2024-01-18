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

package androidmk

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
		desc: "Convert to local path",
		in: `
include $(CLEAR_VARS)
LOCAL_RESOURCE_DIR := $(LOCAL_PATH)/res $(LOCAL_PATH)/res2
LOCAL_ASSET_DIR := $(LOCAL_PATH)/asset
LOCAL_JARJAR_RULES := $(LOCAL_PATH)/jarjar-rules.txt
include $(BUILD_PACKAGE)
	`,
		expected: `
android_app {
	resource_dirs: [
		"res",
		"res2",
	],
	asset_dirs: ["asset"],
	jarjar_rules: "jarjar-rules.txt",
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
		desc: "Warn for LOCAL_MODULE_TAGS non-optional",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE_TAGS := debug
include $(BUILD_SHARED_LIBRARY)
`,

		expected: `
cc_library_shared {
	// WARNING: Module tags are not supported in Soong.
	// Add this module to PRODUCT_PACKAGES_DEBUG in your product file if you want to
	// force installation for -userdebug and -eng builds.
}
`,
	},
	{
		desc: "Custom warning for LOCAL_MODULE_TAGS tests",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE_TAGS := debug tests
include $(BUILD_SHARED_LIBRARY)
`,

		expected: `
cc_library_shared {
	// WARNING: Module tags are not supported in Soong.
	// Add this module to PRODUCT_PACKAGES_DEBUG in your product file if you want to
	// force installation for -userdebug and -eng builds.
	// WARNING: Module tags are not supported in Soong.
	// To make a shared library only for tests, use the "cc_test_library" module
	// type. If you don't use gtest, set "gtest: false".
}
`,
	},
	{
		desc: "Ignore LOCAL_MODULE_TAGS tests for cc_test",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE_TAGS := tests
include $(BUILD_NATIVE_TEST)
`,

		expected: `
cc_test {
}
`,
	},
	{
		desc: "Convert LOCAL_MODULE_TAGS tests to java_test",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE_TAGS := tests
include $(BUILD_JAVA_LIBRARY)

include $(CLEAR_VARS)
LOCAL_MODULE_TAGS := tests
include $(BUILD_PACKAGE)

include $(CLEAR_VARS)
LOCAL_MODULE_TAGS := tests
include $(BUILD_HOST_JAVA_LIBRARY)
`,

		expected: `
java_test {
}

android_test {
}

java_test_host {
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
// ANDROIDMK TRANSLATION ERROR: endif from unsupported conditional
// endif
// ANDROIDMK TRANSLATION ERROR: endif from unsupported conditional
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
			include $(BUILD_STATIC_JAVA_LIBRARY)
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
		desc: "java library",
		in: `
			include $(CLEAR_VARS)
			LOCAL_SRC_FILES := a.java
			include $(BUILD_STATIC_JAVA_LIBRARY)

			include $(CLEAR_VARS)
			LOCAL_SRC_FILES := b.java
			include $(BUILD_JAVA_LIBRARY)

			include $(CLEAR_VARS)
			LOCAL_SRC_FILES := c.java
			LOCAL_UNINSTALLABLE_MODULE := true
			include $(BUILD_JAVA_LIBRARY)

			include $(CLEAR_VARS)
			LOCAL_SRC_FILES := d.java
			LOCAL_UNINSTALLABLE_MODULE := false
			include $(BUILD_JAVA_LIBRARY)

			include $(CLEAR_VARS)
			LOCAL_SRC_FILES := $(call all-java-files-under, src gen)
			include $(BUILD_STATIC_JAVA_LIBRARY)

			include $(CLEAR_VARS)
			LOCAL_JAVA_RESOURCE_FILES := foo bar
			include $(BUILD_STATIC_JAVA_LIBRARY)
		`,
		expected: `
			java_library {
				srcs: ["a.java"],
			}

			java_library {
				installable: true,
				srcs: ["b.java"],
			}

			java_library {
				installable: false,
				srcs: ["c.java"],
			}

			java_library {
				installable: true,
				srcs: ["d.java"],
			}

			java_library {
				srcs: [
					"src/**/*.java",
					"gen/**/*.java",
				],
			}

			java_library {
				java_resources: [
					"foo",
					"bar",
				],
			}
		`,
	},
	{
		desc: "errorprone options for java library",
		in: `
			include $(CLEAR_VARS)
			LOCAL_ERROR_PRONE_FLAGS := -Xep:AsyncCallableReturnsNull:ERROR -Xep:AsyncFunctionReturnsNull:ERROR
			include $(BUILD_STATIC_JAVA_LIBRARY)
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
			LOCAL_JETIFIER_ENABLED := true
			include $(BUILD_PREBUILT)
		`,
		expected: `
			java_import {
				jars: ["test.jar"],

				jetifier: true,
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
			LOCAL_RESOURCE_DIR := $(LOCAL_PATH)/res
			LOCAL_JACK_COVERAGE_INCLUDE_FILTER := foo.*
			include $(BUILD_STATIC_JAVA_LIBRARY)

			include $(CLEAR_VARS)
			LOCAL_SRC_FILES := test.java
			LOCAL_STATIC_LIBRARIES := foo
			LOCAL_STATIC_ANDROID_LIBRARIES := bar
			LOCAL_JACK_COVERAGE_EXCLUDE_FILTER := bar.*
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

				jacoco: {
					include_filter: ["foo.*"],
				},
			}

			android_library {
				srcs: ["test.java"],
				static_libs: [
					"foo",
					"bar",
				],
				jacoco: {
					exclude_filter: ["bar.*"],
				},
			}

			android_library {
				srcs: ["test.java"],
				libs: [
					"foo",
					"bar",
				],
			}

			java_library {
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
	{
		desc: "LOCAL_STRIP_MODULE",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE := libtest
LOCAL_STRIP_MODULE := false
include $(BUILD_SHARED_LIBRARY)

include $(CLEAR_VARS)
LOCAL_MODULE := libtest2
LOCAL_STRIP_MODULE := true
include $(BUILD_SHARED_LIBRARY)

include $(CLEAR_VARS)
LOCAL_MODULE := libtest3
LOCAL_STRIP_MODULE := keep_symbols
include $(BUILD_SHARED_LIBRARY)
`,
		expected: `
cc_library_shared {
    name: "libtest",
    strip: {
        none: true,
    }
}

cc_library_shared {
    name: "libtest2",
    strip: {
        all: true,
    }
}

cc_library_shared {
    name: "libtest3",
    strip: {
        keep_symbols: true,
    }
}
`,
	},
	{
		desc: "BUILD_CTS_SUPPORT_PACKAGE",
		in: `
include $(CLEAR_VARS)
LOCAL_PACKAGE_NAME := FooTest
LOCAL_COMPATIBILITY_SUITE := cts
LOCAL_MODULE_PATH := $(TARGET_OUT_DATA_APPS)
include $(BUILD_CTS_SUPPORT_PACKAGE)
`,
		expected: `
android_test_helper_app {
    name: "FooTest",
    defaults: ["cts_support_defaults"],
    test_suites: ["cts"],

}
`,
	},
	{
		desc: "BUILD_CTS_PACKAGE",
		in: `
include $(CLEAR_VARS)
LOCAL_PACKAGE_NAME := FooTest
LOCAL_COMPATIBILITY_SUITE := cts
LOCAL_CTS_TEST_PACKAGE := foo.bar
LOCAL_COMPATIBILITY_SUPPORT_FILES := file1
include $(BUILD_CTS_PACKAGE)
`,
		expected: `
android_test {
    name: "FooTest",
    defaults: ["cts_defaults"],
    test_suites: ["cts"],

    data: ["file1"],
}
`,
	},
	{
		desc: "IGNORE_LOCAL_XTS_TEST_PACKAGE",
		in: `
include $(CLEAR_VARS)
LOCAL_PACKAGE_NAME := FooTest
LOCAL_COMPATIBILITY_SUITE := cts
LOCAL_XTS_TEST_PACKAGE := foo.bar
LOCAL_COMPATIBILITY_SUPPORT_FILES := file1
include $(BUILD_CTS_PACKAGE)
`,
		expected: `
android_test {
    name: "FooTest",
    defaults: ["cts_defaults"],
    test_suites: ["cts"],

    data: ["file1"],
}
`,
	},
	{
		desc: "BUILD_CTS_*_JAVA_LIBRARY",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE := foolib
include $(BUILD_CTS_TARGET_JAVA_LIBRARY)

include $(CLEAR_VARS)
LOCAL_MODULE := foolib-host
include $(BUILD_CTS_HOST_JAVA_LIBRARY)
`,
		expected: `
java_library {
    name: "foolib",
    defaults: ["cts_defaults"],
}

java_library_host {
    name: "foolib-host",
    defaults: ["cts_defaults"],
}
`,
	},
	{
		desc: "LOCAL_ANNOTATION_PROCESSORS",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE := foolib
LOCAL_ANNOTATION_PROCESSORS := bar
LOCAL_ANNOTATION_PROCESSOR_CLASSES := com.bar
include $(BUILD_STATIC_JAVA_LIBRARY)
`,
		expected: `
java_library {
    name: "foolib",
    plugins: ["bar"],

}
`,
	},
	{
		desc: "prebuilt_etc_TARGET_OUT_ETC",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE := etc.test1
LOCAL_SRC_FILES := mymod
LOCAL_MODULE_CLASS := ETC
LOCAL_MODULE_PATH := $(TARGET_OUT_ETC)/foo/bar
include $(BUILD_PREBUILT)
`,
		expected: `
prebuilt_etc {
	name: "etc.test1",
	src: "mymod",
	relative_install_path: "foo/bar",

}
`,
	},
	{
		desc: "prebuilt_etc_PRODUCT_OUT/system/etc",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE := etc.test1
LOCAL_MODULE_CLASS := ETC
LOCAL_MODULE_PATH := $(PRODUCT_OUT)/system/etc/foo/bar
LOCAL_SRC_FILES := $(LOCAL_MODULE)
include $(BUILD_PREBUILT)
`,
		expected: `
prebuilt_etc {
	name: "etc.test1",

	src: "etc.test1",
	relative_install_path: "foo/bar",

}
`,
	},
	{
		desc: "prebuilt_etc_TARGET_OUT_ODM/etc",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE := etc.test1
LOCAL_MODULE_CLASS := ETC
LOCAL_MODULE_PATH := $(TARGET_OUT_ODM)/etc/foo/bar
include $(BUILD_PREBUILT)
`,
		expected: `
prebuilt_etc {
	name: "etc.test1",
	relative_install_path: "foo/bar",
    device_specific: true,

}
`,
	},
	{
		desc: "prebuilt_etc_TARGET_OUT_PRODUCT/etc",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE := etc.test1
LOCAL_MODULE_CLASS := ETC
LOCAL_MODULE_PATH := $(TARGET_OUT_PRODUCT)/etc/foo/bar
include $(BUILD_PREBUILT)
`,
		expected: `
prebuilt_etc {
	name: "etc.test1",
	relative_install_path: "foo/bar",
	product_specific: true,


}
`,
	},
	{
		desc: "prebuilt_etc_TARGET_OUT_PRODUCT_ETC",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE := etc.test1
LOCAL_MODULE_CLASS := ETC
LOCAL_MODULE_PATH := $(TARGET_OUT_PRODUCT_ETC)/foo/bar
include $(BUILD_PREBUILT)
`,
		expected: `
prebuilt_etc {
	name: "etc.test1",
	relative_install_path: "foo/bar",
	product_specific: true,

}
`,
	},
	{
		desc: "prebuilt_etc_TARGET_OUT_SYSTEM_EXT/etc",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE := etc.test1
LOCAL_MODULE_CLASS := ETC
LOCAL_MODULE_PATH := $(TARGET_OUT_SYSTEM_EXT)/etc/foo/bar
include $(BUILD_PREBUILT)
`,
		expected: `
prebuilt_etc {
	name: "etc.test1",
	relative_install_path: "foo/bar",
	system_ext_specific: true,

}
`,
	},
	{
		desc: "prebuilt_etc_TARGET_OUT_SYSTEM_EXT_ETC",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE := etc.test1
LOCAL_MODULE_CLASS := ETC
LOCAL_MODULE_PATH := $(TARGET_OUT_SYSTEM_EXT_ETC)/foo/bar
include $(BUILD_PREBUILT)
`,
		expected: `
prebuilt_etc {
	name: "etc.test1",
	relative_install_path: "foo/bar",
	system_ext_specific: true,


}
`,
	},
	{
		desc: "prebuilt_etc_TARGET_OUT_VENDOR/etc",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE := etc.test1
LOCAL_MODULE_CLASS := ETC
LOCAL_MODULE_PATH := $(TARGET_OUT_VENDOR)/etc/foo/bar
include $(BUILD_PREBUILT)
`,
		expected: `
prebuilt_etc {
	name: "etc.test1",
	relative_install_path: "foo/bar",
	proprietary: true,

}
`,
	},
	{
		desc: "prebuilt_etc_PRODUCT_OUT/TARGET_COPY_OUT_VENDOR/etc",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE := etc.test1
LOCAL_MODULE_CLASS := ETC
LOCAL_MODULE_PATH := $(PRODUCT_OUT)/$(TARGET_COPY_OUT_VENDOR)/etc/foo/bar
include $(BUILD_PREBUILT)
`,
		expected: `
prebuilt_etc {
	name: "etc.test1",
	relative_install_path: "foo/bar",
	proprietary: true,

}
`,
	},
	{
		desc: "prebuilt_etc_TARGET_OUT_VENDOR_ETC",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE := etc.test1
LOCAL_MODULE_CLASS := ETC
LOCAL_MODULE_PATH := $(TARGET_OUT_VENDOR_ETC)/foo/bar
include $(BUILD_PREBUILT)
`,
		expected: `
prebuilt_etc {
	name: "etc.test1",
	relative_install_path: "foo/bar",
	proprietary: true,

}
`,
	},
	{
		desc: "prebuilt_etc_TARGET_RECOVERY_ROOT_OUT/system/etc",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE := etc.test1
LOCAL_MODULE_CLASS := ETC
LOCAL_MODULE_PATH := $(TARGET_RECOVERY_ROOT_OUT)/system/etc/foo/bar
include $(BUILD_PREBUILT)
`,
		expected: `
prebuilt_etc {
	name: "etc.test1",
	relative_install_path: "foo/bar",
	recovery: true,

}
`,
	},
	{
		desc: "prebuilt_usr_share",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE := foo
LOCAL_MODULE_CLASS := ETC
LOCAL_MODULE_PATH := $(TARGET_OUT)/usr/share
LOCAL_SRC_FILES := foo.txt
include $(BUILD_PREBUILT)
`,
		expected: `
prebuilt_usr_share {
	name: "foo",

	src: "foo.txt",
}
`,
	},
	{
		desc: "prebuilt_usr_share subdir_bar",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE := foo
LOCAL_MODULE_CLASS := ETC
LOCAL_MODULE_PATH := $(TARGET_OUT)/usr/share/bar
LOCAL_SRC_FILES := foo.txt
include $(BUILD_PREBUILT)
`,
		expected: `
prebuilt_usr_share {
	name: "foo",

	src: "foo.txt",
	relative_install_path: "bar",
}
`,
	},
	{
		desc: "prebuilt_usr_share_host",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE := foo
LOCAL_MODULE_CLASS := ETC
LOCAL_MODULE_PATH := $(HOST_OUT)/usr/share
LOCAL_SRC_FILES := foo.txt
include $(BUILD_PREBUILT)
`,
		expected: `
prebuilt_usr_share_host {
	name: "foo",

	src: "foo.txt",
}
`,
	},
	{
		desc: "prebuilt_root_host",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE := foo
LOCAL_MODULE_CLASS := ETC
LOCAL_MODULE_PATH := $(HOST_OUT)/subdir
LOCAL_SRC_FILES := foo.txt
include $(BUILD_PREBUILT)
`,
		expected: `
prebuilt_root_host {
	name: "foo",

	src: "foo.txt",
	relative_install_path: "subdir",
}
`,
	},
	{
		desc: "prebuilt_font",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE := font.ttf
LOCAL_SRC_FILES := $(LOCAL_MODULE)
LOCAL_MODULE_CLASS := ETC
LOCAL_MODULE_TAGS := optional
LOCAL_MODULE_PATH := $(TARGET_OUT)/fonts
include $(BUILD_PREBUILT)
`,
		expected: `
prebuilt_font {
	name: "font.ttf",
	src: "font.ttf",

}
`,
	},
	{
		desc: "prebuilt_font",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE := font.ttf
LOCAL_SRC_FILES := $(LOCAL_MODULE)
LOCAL_MODULE_CLASS := ETC
LOCAL_MODULE_TAGS := optional
LOCAL_MODULE_PATH := $(TARGET_OUT_PRODUCT)/fonts
include $(BUILD_PREBUILT)
`,
		expected: `
prebuilt_font {
	name: "font.ttf",
	src: "font.ttf",
	product_specific: true,

}
`,
	},
	{
		desc: "prebuilt_usr_share_host subdir_bar",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE := foo
LOCAL_MODULE_CLASS := ETC
LOCAL_MODULE_PATH := $(HOST_OUT)/usr/share/bar
LOCAL_SRC_FILES := foo.txt
include $(BUILD_PREBUILT)
`,
		expected: `
prebuilt_usr_share_host {
	name: "foo",

	src: "foo.txt",
	relative_install_path: "bar",
}
`,
	},
	{
		desc: "prebuilt_firmware subdir_bar in $(TARGET_OUT_ETC)",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE := foo
LOCAL_MODULE_CLASS := ETC
LOCAL_MODULE_PATH := $(TARGET_OUT_ETC)/firmware/bar
LOCAL_SRC_FILES := foo.fw
include $(BUILD_PREBUILT)
`,
		expected: `
prebuilt_firmware {
	name: "foo",

	src: "foo.fw",
	relative_install_path: "bar",
}
`,
	},
	{
		desc: "prebuilt_firmware subdir_bar in $(TARGET_OUT)",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE := foo
LOCAL_MODULE_CLASS := ETC
LOCAL_MODULE_PATH := $(TARGET_OUT)/etc/firmware/bar
LOCAL_SRC_FILES := foo.fw
include $(BUILD_PREBUILT)
`,
		expected: `
prebuilt_firmware {
	name: "foo",

	src: "foo.fw",
	relative_install_path: "bar",
}
`,
	},
	{
		desc: "prebuilt_firmware subdir_bar in $(TARGET_OUT_VENDOR)",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE := foo
LOCAL_MODULE_CLASS := ETC
LOCAL_MODULE_PATH := $(TARGET_OUT_VENDOR)/firmware/bar
LOCAL_SRC_FILES := foo.fw
include $(BUILD_PREBUILT)
`,
		expected: `
prebuilt_firmware {
	name: "foo",

	src: "foo.fw",
	relative_install_path: "bar",
	proprietary: true,
}
`,
	},
	{
		desc: "prebuilt_firmware subdir_bar in $(TARGET_OUT)/vendor",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE := foo
LOCAL_MODULE_CLASS := ETC
LOCAL_MODULE_PATH := $(TARGET_OUT)/vendor/firmware/bar
LOCAL_SRC_FILES := foo.fw
include $(BUILD_PREBUILT)
`,
		expected: `
prebuilt_firmware {
	name: "foo",

	src: "foo.fw",
	relative_install_path: "bar",
	proprietary: true,
}
`,
	},
	{
		desc: "comment with ESC",
		in: `
# Comment line 1 \
 Comment line 2
`,
		expected: `
// Comment line 1
// Comment line 2
`,
	},
	{
		desc: "Merge with variable reference",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE := foo
LOCAL_STATIC_ANDROID_LIBRARIES := $(FOO)
LOCAL_STATIC_JAVA_LIBRARIES := javalib
LOCAL_JAVA_RESOURCE_DIRS := $(FOO)
include $(BUILD_PACKAGE)
`,
		expected: `
android_app {
	name: "foo",
	static_libs: FOO,
	static_libs: ["javalib"],
	java_resource_dirs: FOO,
}
`,
	},
	{
		desc: "LOCAL_JACK_ENABLED and LOCAL_JACK_FLAGS skipped",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE := foo
LOCAL_JACK_ENABLED := incremental
LOCAL_JACK_FLAGS := --multi-dex native
include $(BUILD_PACKAGE)
		`,
		expected: `
android_app {
	name: "foo",

}
		`,
	},
	{
		desc: "android_app_import",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE := foo
LOCAL_SRC_FILES := foo.apk
LOCAL_PRIVILEGED_MODULE := true
LOCAL_MODULE_CLASS := APPS
LOCAL_MODULE_TAGS := optional
LOCAL_DEX_PREOPT := false
include $(BUILD_PREBUILT)
`,
		expected: `
android_app_import {
	name: "foo",

	privileged: true,

	dex_preopt: {
		enabled: false,
	},
	apk: "foo.apk",

}
`,
	},
	{
		desc: "android_test_import prebuilt",
		in: `
		include $(CLEAR_VARS)
		LOCAL_MODULE := foo
		LOCAL_SRC_FILES := foo.apk
		LOCAL_MODULE_CLASS := APPS
		LOCAL_MODULE_TAGS := tests
		LOCAL_MODULE_SUFFIX := .apk
		LOCAL_CERTIFICATE := PRESIGNED
		LOCAL_REPLACE_PREBUILT_APK_INSTALLED := $(LOCAL_PATH)/foo.apk
		LOCAL_COMPATIBILITY_SUITE := cts
		include $(BUILD_PREBUILT)
		`,
		expected: `
android_test_import {
	name: "foo",
	srcs: ["foo.apk"],

	certificate: "PRESIGNED",
	preprocessed: true,
	test_suites: ["cts"],
}
`,
	},
	{
		desc: "dashed_variable gets renamed",
		in: `
		include $(CLEAR_VARS)

		dashed-variable:= a.cpp

		LOCAL_MODULE:= test
		LOCAL_SRC_FILES:= $(dashed-variable)
		include $(BUILD_EXECUTABLE)
		`,
		expected: `

// ANDROIDMK TRANSLATION WARNING: Variable names cannot contain: "-". Renamed "dashed-variable" to "dashed_dash_variable"
dashed_dash_variable = ["a.cpp"]
cc_binary {

    name: "test",
    srcs: dashed_dash_variable,
}
`,
	},
	{
		desc: "variableReassigned",
		in: `
include $(CLEAR_VARS)

src_files:= a.cpp

LOCAL_SRC_FILES:= $(src_files)
LOCAL_MODULE:= test
include $(BUILD_EXECUTABLE)

# clear locally used variable
src_files:=
`,
		expected: `


src_files = ["a.cpp"]
cc_binary {
    name: "test",

    srcs: src_files,
}

// clear locally used variable
// ANDROIDMK TRANSLATION ERROR: cannot assign a variable multiple times: "src_files"
// src_files :=
`,
	},
	{
		desc: "undefined_boolean_var",
		in: `
include $(CLEAR_VARS)
LOCAL_SRC_FILES:= a.cpp
LOCAL_MODULE:= test
LOCAL_32_BIT_ONLY := $(FLAG)
include $(BUILD_EXECUTABLE)
`,
		expected: `
cc_binary {
    name: "test",
    srcs: ["a.cpp"],
    // ANDROIDMK TRANSLATION ERROR: value should evaluate to boolean literal
    // LOCAL_32_BIT_ONLY := $(FLAG)

}
`,
	},
	{
		desc: "runtime_resource_overlay",
		in: `
include $(CLEAR_VARS)
LOCAL_PACKAGE_NAME := foo
LOCAL_PRODUCT_MODULE := true
LOCAL_RESOURCE_DIR := $(LOCAL_PATH)/res
LOCAL_SDK_VERSION := current
LOCAL_TARGET_SDK_VERSION := target_version
LOCAL_RRO_THEME := FooTheme

include $(BUILD_RRO_PACKAGE)
`,
		expected: `
runtime_resource_overlay {
	name: "foo",
	product_specific: true,

	sdk_version: "current",
	target_sdk_version: "target_version",
	theme: "FooTheme",

}
`,
	},
	{
		desc: "LOCAL_ENFORCE_USES_LIBRARIES",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE := foo
LOCAL_ENFORCE_USES_LIBRARIES := false
LOCAL_ENFORCE_USES_LIBRARIES := true
include $(BUILD_PACKAGE)
`,
		expected: `
android_app {
    name: "foo",
    enforce_uses_libs: false,
    enforce_uses_libs: true,
}
`,
	},
	{
		desc: "LOCAL_CERTIFICATE_LINEAGE",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE := foo
LOCAL_MODULE_TAGS := tests
LOCAL_CERTIFICATE_LINEAGE := lineage
include $(BUILD_PACKAGE)
`,
		expected: `
android_test {
    name: "foo",
    lineage: "lineage",
}
`,
	},
	{
		desc: "LOCAL_USES_LIBRARIES",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE := foo
LOCAL_USES_LIBRARIES := foo.test bar.test baz.test
include $(BUILD_PACKAGE)
`,
		expected: `
android_app {
    name: "foo",
    uses_libs: [
        "foo.test",
        "bar.test",
        "baz.test",
    ],
}
`,
	},
	{
		desc: "LOCAL_OPTIONAL_USES_LIBRARIES",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE := foo
LOCAL_OPTIONAL_USES_LIBRARIES := foo.test bar.test baz.test
include $(BUILD_PACKAGE)
`,
		expected: `
android_app {
    name: "foo",
    optional_uses_libs: [
        "foo.test",
        "bar.test",
        "baz.test",
    ],
}
`,
	},
	{
		desc: "Obsolete LOCAL_MODULE_PATH",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE := foo
LOCAL_MODULE_PATH := $(TARGET_OUT_DATA_APPS)
LOCAL_MODULE_PATH := $(TARGET_OUT_OPTIONAL_EXECUTABLES)
LOCAL_CTS_TEST_PACKAGE := bar
LOCAL_USE_AAPT2 := blah
include $(BUILD_PACKAGE)
`,
		expected: `
android_app {
  name: "foo",

}
`,
	},
	{
		desc: "LOCAL_LICENSE_KINDS, LOCAL_LICENSE_CONDITIONS, LOCAL_NOTICE_FILE",
		// When "android_license_files" is valid, the test requires an Android.mk file
		// outside the current (and an Android.bp file is required as well if the license
		// files locates directory), thus a mock file system is needed. The integration
		// test cases for these scenarios have been added in
		// $(ANDROID_BUILD_TOP)/build/soong/tests/androidmk_test.sh.
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE := foo
LOCAL_LICENSE_KINDS := license_kind
LOCAL_LICENSE_CONDITIONS := license_condition
LOCAL_NOTICE_FILE := license_notice
include $(BUILD_PACKAGE)
`,
		expected: `
package {
    // See: http://go/android-license-faq
    default_applicable_licenses: [
	"Android-Apache-2.0",
    ],
}

android_app {
    name: "foo",
    // ANDROIDMK TRANSLATION ERROR: Only $(LOCAL_PATH)/.. values are allowed
    // LOCAL_NOTICE_FILE := license_notice

}
`,
	},
	{
		desc: "LOCAL_CHECK_ELF_FILES",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE := foo
LOCAL_SRC_FILES := test.c
LOCAL_MODULE_CLASS := SHARED_LIBRARIES
LOCAL_CHECK_ELF_FILES := false
include $(BUILD_PREBUILT)
		`,
		expected: `
cc_prebuilt_library_shared {
	name: "foo",
	srcs: ["test.c"],

	check_elf_files: false,
}
`,
	},
	{
		desc: "Drop default resource and asset dirs from bp",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE := foo
LOCAL_ASSET_DIR := $(LOCAL_PATH)/assets
LOCAL_RESOURCE_DIR := $(LOCAL_PATH)/res
include $(BUILD_PACKAGE)
`,
		expected: `
android_app {
		name: "foo",

}
`,
	},
	{
		desc: "LOCAL_GENERATED_SOURCES",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE := foo
LOCAL_SRC_FILES := src1, src2, src3
LOCAL_GENERATED_SOURCES := gen_src1, gen_src2, gen_src3
include $(BUILD_PACKAGE)
		`,
		expected: `
android_app {
	name: "foo",
	srcs: [
		"src1,",
		"src2,",
		"src3",
	],
	generated_sources: [
		"gen_src1,",
		"gen_src2,",
		"gen_src3",
	],
}
`,
	},
	{
		desc: "LOCAL_DISABLE_AUTO_GENERATE_TEST_CONFIG is true",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE := foo
LOCAL_DISABLE_AUTO_GENERATE_TEST_CONFIG := true
include $(BUILD_PACKAGE)
		`,
		expected: `
android_app {
	name: "foo",
	auto_gen_config: false,
}
`,
	},
	{
		desc: "LOCAL_DISABLE_AUTO_GENERATE_TEST_CONFIG is false",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE := foo
LOCAL_DISABLE_AUTO_GENERATE_TEST_CONFIG := false
include $(BUILD_PACKAGE)
		`,
		expected: `
android_app {
	name: "foo",
	auto_gen_config: true,
}
`,
	},
	{
		desc: "privileged app",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE := foo
LOCAL_MODULE_PATH := $(PRODUCT_OUT)/system/priv-app
include $(BUILD_PACKAGE)
		`,
		expected: `
android_app {
	name: "foo",
	privileged: true
}
`,
	},
	{
		desc: "convert android_app to android_test when having test_suites",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE := foo
LOCAL_COMPATIBILITY_SUITE := bar
include $(BUILD_PACKAGE)
		`,
		expected: `
android_test {
	name: "foo",
	test_suites: ["bar"],
}
`,
	},
	{
		desc: "LOCAL_PROTO_JAVA_OUTPUT_PARAMS",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE := foo
LOCAL_PROTO_JAVA_OUTPUT_PARAMS := enum_style=java, parcelable_messages=true
LOCAL_PROTOC_FLAGS := --proto_path=$(LOCAL_PATH)/protos
include $(BUILD_PACKAGE)
		`,
		expected: `
android_app {
	name: "foo",
	proto: {
        output_params: [
            "enum_style=java",
            "parcelable_messages=true",
        ],
		local_include_dirs: ["protos"],
    },
}
`,
	},
}

func TestEndToEnd(t *testing.T) {
	// Skip checking Android.mk path with cleaning "ANDROID_BUILD_TOP"
	t.Setenv("ANDROID_BUILD_TOP", "")

	for i, test := range testCases {
		expected, err := bpfix.Reformat(test.expected)
		if err != nil {
			t.Error(err)
		}

		got, errs := ConvertFile(fmt.Sprintf("<testcase %d>", i), bytes.NewBufferString(test.in))
		if len(errs) > 0 {
			t.Errorf("Unexpected errors: %q", errs)
			continue
		}

		if got != expected {
			t.Errorf("failed testcase '%s'\ninput:\n%s\n\nexpected:\n%s\ngot:\n%s\n", test.desc, strings.TrimSpace(test.in), expected, got)
		}
	}
}
