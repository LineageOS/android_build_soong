#!/bin/bash -eu

set -o pipefail

TOP="$(readlink -f "$(dirname "$0")"/../../..)"
"$TOP/build/soong/tests/androidmk_test.sh"
"$TOP/build/soong/tests/bootstrap_test.sh"
"$TOP/build/soong/tests/soong_test.sh"
"$TOP/prebuilts/build-tools/linux-x86/bin/py3-cmd" "$TOP/build/bazel/ci/rbc_dashboard.py" aosp_arm64-userdebug
