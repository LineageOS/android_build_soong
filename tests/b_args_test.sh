#!/bin/bash -eu

# This file tests the creation of bazel commands for b usage
set -o pipefail
source "$(dirname "$0")/../../bazel/lib.sh"

BES_UUID="blank"
OUT_DIR="arbitrary_out"
b_args=$(formulate_b_args "build --config=nonsense foo:bar")

if [[ $b_args != "build --profile=$OUT_DIR/bazel_metrics-profile --config=bp2build --invocation_id=$BES_UUID --config=metrics_data --config=nonsense foo:bar" ]]; then
   echo "b args are malformed"
   echo "Expected : build --profile=$OUT_DIR/bazel_metrics-profile --config=bp2build  --invocation_id=$BES_UUID --config=metrics_data --config=nonsense foo:bar"
   echo "Actual: $b_args"
   exit 1
fi

b_args=$(formulate_b_args "build --config=nonsense --disable_bes --package_path \"my package\" foo:bar")

if [[ $b_args != "build --profile=$OUT_DIR/bazel_metrics-profile --config=bp2build --invocation_id=$BES_UUID --config=nonsense --package_path \"my package\" foo:bar" ]]; then
   echo "b args are malformed"
   echo "Expected : build --profile=$OUT_DIR/bazel_metrics-profile --config=bp2build  --invocation_id=$BES_UUID --config=nonsense --package_path \"my package\" foo:bar"
   echo "Actual: $b_args"
   exit 1
fi

# Test with startup option
b_args=$(formulate_b_args "--batch build --config=nonsense --disable_bes --package_path \"my package\" foo:bar")
if [[ $b_args != "--batch build --profile=$OUT_DIR/bazel_metrics-profile --config=bp2build --invocation_id=$BES_UUID --config=nonsense --package_path \"my package\" foo:bar" ]]; then
   echo "b args are malformed"
   echo "Expected : --batch build --profile=$OUT_DIR/bazel_metrics-profile --config=bp2build  --invocation_id=$BES_UUID --config=nonsense --package_path \"my package\" foo:bar"
   echo "Actual: $b_args"
   exit 1
fi

OUT_DIR="mock_out"
TEST_PROFILE_OUT=$(get_profile_out_dir)
if [[ $TEST_PROFILE_OUT != "mock_out" ]]; then
   echo "Profile Out is malformed."
   echo "Expected: mock_out"
   echo "Actual: $TEST_PROFILE_OUT"
   exit 1
fi
