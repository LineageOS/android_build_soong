#!/bin/bash -eu

set -o pipefail

TOP="$(readlink -f "$(dirname "$0")"/../../..)"
"$TOP/build/soong/tests/bootstrap_test.sh"
"$TOP/build/soong/tests/mixed_mode_test.sh"
"$TOP/build/soong/tests/bp2build_bazel_test.sh"
