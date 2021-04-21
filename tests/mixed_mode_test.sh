#!/bin/bash -eu

set -o pipefail

# This test exercises mixed builds where Soong and Bazel cooperate in building
# Android.
#
# When the execroot is deleted, the Bazel server process will automatically
# terminate itself.

source "$(dirname "$0")/lib.sh"

function test_bazel_smoke {
  setup
  create_mock_bazel

  run_bazel info
}

test_bazel_smoke
