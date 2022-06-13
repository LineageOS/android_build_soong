#!/bin/bash -eu

set -o pipefail

# How to run: bash path-to-script/androidmk_test.sh
# Tests of converting license functionality of the androidmk tool
REAL_TOP="$(readlink -f "$(dirname "$0")"/../../..)"
"$REAL_TOP/build/soong/soong_ui.bash" --make-mode androidmk

source "$(dirname "$0")/lib.sh"

# Expect to create a new license module
function test_rewrite_license_property_inside_current_directory {
  setup

  # Create an Android.mk file
  mkdir -p a/b
  cat > a/b/Android.mk <<'EOF'
include $(CLEAR_VARS)
LOCAL_MODULE := foo
LOCAL_LICENSE_KINDS := license_kind1 license_kind2
LOCAL_LICENSE_CONDITIONS := license_condition
LOCAL_NOTICE_FILE := $(LOCAL_PATH)/license_notice1 $(LOCAL_PATH)/license_notice2
include $(BUILD_PACKAGE)
EOF

  # Create an expected Android.bp file for the module "foo"
  cat > a/b/Android.bp <<'EOF'
package {
    // See: http://go/android-license-faq
    default_applicable_licenses: [
        "a_b_license",
    ],
}

license {
    name: "a_b_license",
    visibility: [":__subpackages__"],
    license_kinds: [
        "license_kind1",
        "license_kind2",
    ],
    license_text: [
        "license_notice1",
        "license_notice2",
    ],
}

android_app {
    name: "foo",
}
EOF

  run_androidmk_test "a/b/Android.mk" "a/b/Android.bp"
}

# Expect to reference to an existing license module
function test_rewrite_license_property_outside_current_directory {
  setup

  # Create an Android.mk file
  mkdir -p a/b/c/d
  cat > a/b/c/d/Android.mk <<'EOF'
include $(CLEAR_VARS)
LOCAL_MODULE := foo
LOCAL_LICENSE_KINDS := license_kind1 license_kind2
LOCAL_LICENSE_CONDITIONS := license_condition
LOCAL_NOTICE_FILE := $(LOCAL_PATH)/../../license_notice1 $(LOCAL_PATH)/../../license_notice2
include $(BUILD_PACKAGE)
EOF

  # Create an expected (input) Android.bp file at a/b/
  cat > a/b/Android.bp <<'EOF'
package {
    // See: http://go/android-license-faq
    default_applicable_licenses: [
        "a_b_license",
    ],
}

license {
    name: "a_b_license",
    visibility: [":__subpackages__"],
    license_kinds: [
        "license_kind1",
        "license_kind2",
    ],
    license_text: [
        "license_notice1",
        "license_notice2",
    ],
}

android_app {
    name: "bar",
}
EOF

  # Create an expected (output) Android.bp file for the module "foo"
  cat > a/b/c/d/Android.bp <<'EOF'
package {
    // See: http://go/android-license-faq
    default_applicable_licenses: [
        "a_b_license",
    ],
}

android_app {
    name: "foo",
}
EOF

  run_androidmk_test "a/b/c/d/Android.mk" "a/b/c/d/Android.bp"
}

function run_androidmk_test {
  export ANDROID_BUILD_TOP="$MOCK_TOP"
  local -r androidmk=("$REAL_TOP"/*/host/*/bin/androidmk)
  if [[ ${#androidmk[@]} -ne 1 ]]; then
    fail "Multiple androidmk binaries found: ${androidmk[*]}"
  fi
  local -r out=$("${androidmk[0]}" "$1")
  local -r expected=$(<"$2")

  if [[ "$out" != "$expected" ]]; then
    ANDROID_BUILD_TOP="$REAL_TOP"
    cleanup_mock_top
    fail "The output is not the same as the expected"
  fi

  ANDROID_BUILD_TOP="$REAL_TOP"
  cleanup_mock_top
  echo "Succeeded"
}

test_rewrite_license_property_inside_current_directory

test_rewrite_license_property_outside_current_directory
