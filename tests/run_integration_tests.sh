#!/bin/bash -eu

set -o pipefail

TOP="$(readlink -f "$(dirname "$0")"/../../..)"
"$TOP/build/soong/tests/androidmk_test.sh"
"$TOP/build/soong/tests/b_args_test.sh"
"$TOP/build/soong/tests/bootstrap_test.sh"
"$TOP/build/soong/tests/mixed_mode_test.sh"
"$TOP/build/soong/tests/bp2build_bazel_test.sh"
"$TOP/build/soong/tests/persistent_bazel_test.sh"
"$TOP/build/soong/tests/soong_test.sh"
"$TOP/build/soong/tests/stale_metrics_files_test.sh"
"$TOP/build/soong/tests/symlink_forest_rerun_test.sh"
"$TOP/prebuilts/build-tools/linux-x86/bin/py3-cmd" "$TOP/build/bazel/ci/rbc_dashboard.py" aosp_arm64-userdebug

# The following tests build against the full source tree and don't rely on the
# mock client.
"$TOP/build/soong/tests/apex_comparison_tests.sh"
"$TOP/build/soong/tests/apex_comparison_tests.sh" "module_arm64only"
TEST_BAZEL=true extra_build_params=--bazel-mode-staging "$TOP/build/soong/tests/dcla_apex_comparison_test.sh"
#BUILD_BROKEN_DISABLE_BAZEL=true "$TOP/build/soong/tests/dcla_apex_comparison_test.sh"
"$TOP/build/soong/tests/apex_cc_module_arch_variant_tests.sh"
"$TOP/build/soong/tests/apex_cc_module_arch_variant_tests.sh" "aosp_arm" "armv7-a"
"$TOP/build/soong/tests/apex_cc_module_arch_variant_tests.sh" "aosp_cf_arm64_phone" "armv8-a" "cortex-a53"

"$TOP/build/bazel/ci/b_test.sh"
"$TOP/build/soong/tests/symlinks_path_test.sh"
