#!/bin/bash -eu

TOP="$(readlink -f "$(dirname "$0")"/../../..)"
"$TOP/build/soong/tests/bootstrap_test.sh"
"$TOP/build/soong/tests/mixed_mode_test.sh"

