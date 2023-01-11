#!/bin/bash -eu

set -o pipefail

TOP="$(readlink -f "$(dirname "$0")"/../../..)"
"$TOP/build/soong/tests/androidmk_test.sh"
"$TOP/build/soong/tests/bootstrap_test.sh"
"$TOP/build/soong/tests/mixed_mode_test.sh"
"$TOP/build/soong/tests/bp2build_bazel_test.sh"
"$TOP/build/soong/tests/soong_test.sh"
"$TOP/build/bazel/ci/rbc_regression_test.sh" aosp_arm64-userdebug

# The following tests build against the full source tree and don't rely on the
# mock client.
"$TOP/build/soong/tests/apex_comparison_tests.sh"
"$TOP/build/soong/tests/apex_comparison_tests.sh" "module_arm64only"

"$TOP/build/soong/tests/apex_cc_module_arch_variant_tests.sh"
"$TOP/build/soong/tests/apex_cc_module_arch_variant_tests.sh" "aosp_arm" "armv7-a"
"$TOP/build/soong/tests/apex_cc_module_arch_variant_tests.sh" "aosp_cf_arm64_phone" "armv8-a" "cortex-a53"