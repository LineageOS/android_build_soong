#!/bin/bash -eu

# This test exercises mixed builds where Soong and Bazel cooperate in building
# Android.
#
# When the execroot is deleted, the Bazel server process will automatically
# terminate itself.

source "$(dirname "$0")/lib.sh"

function create_mock_bazel() {
  copy_directory build/bazel

  symlink_directory prebuilts/bazel
  symlink_directory prebuilts/jdk

  symlink_file WORKSPACE
  symlink_file tools/bazel
}

function test_bazel_smoke {
  setup
  create_mock_bazel

  tools/bazel info
}

test_bazel_smoke
