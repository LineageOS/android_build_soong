#!/bin/bash -eu

# Copyright (C) 2023 The Android Open Source Project
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o pipefail

source "$(dirname "$0")/lib.sh"

# This test verifies that adding USE_PERSISTENT_BAZEL creates a Bazel process
# that outlasts the build process.
# This test should only be run in sandboxed environments (because this test
# verifies a Bazel process using global process list, and may spawn lingering
# Bazel processes).
function test_persistent_bazel {
  setup

  # Ensure no existing Bazel process.
  if [[ -e out/bazel/output/server/server.pid.txt ]]; then
    kill $(cat out/bazel/output/server/server.pid.txt) 2>/dev/null || true
    if kill -0 $(cat out/bazel/output/server/server.pid.txt) 2>/dev/null ; then
      fail "Error killing pre-setup bazel"
    fi
  fi

  USE_PERSISTENT_BAZEL=1 run_soong nothing

  if ! kill -0 $(cat out/bazel/output/server/server.pid.txt) 2>/dev/null ; then
    fail "Persistent bazel process expected, but not found after first build"
  fi
  BAZEL_PID=$(cat out/bazel/output/server/server.pid.txt)

  USE_PERSISTENT_BAZEL=1 run_soong nothing

  if ! kill -0 $BAZEL_PID 2>/dev/null ; then
    fail "Bazel pid $BAZEL_PID was killed after second build"
  fi

  kill $BAZEL_PID 2>/dev/null
  if ! kill -0 $BAZEL_PID 2>/dev/null ; then
    fail "Error killing bazel on shutdown"
  fi
}

# Verifies that USE_PERSISTENT_BAZEL mode operates as expected in the event
# that there are Bazel failures.
function test_bazel_failure {
  setup

  # Ensure no existing Bazel process.
  if [[ -e out/bazel/output/server/server.pid.txt ]]; then
    kill $(cat out/bazel/output/server/server.pid.txt) 2>/dev/null || true
    if kill -0 $(cat out/bazel/output/server/server.pid.txt) 2>/dev/null ; then
      fail "Error killing pre-setup bazel"
    fi
  fi

  # Introduce a syntax error in a BUILD file which is used in every build
  # (Note this is a BUILD file which is copied as part of test setup, so this
  # has no effect on sources outside of this test.
  rm -rf  build/bazel/rules

  USE_PERSISTENT_BAZEL=1 run_soong nothing 1>out/failurelog.txt 2>&1 && fail "Expected build failure" || true

  if ! grep -sq "cannot load //build/bazel/rules/common/api_constants.bzl" out/failurelog.txt ; then
    fail "Expected error to contain 'cannot load //build/bazel/rules/common/api_constants.bzl', instead got:\n$(cat out/failurelog.txt)"
  fi

  kill $(cat out/bazel/output/server/server.pid.txt) 2>/dev/null || true
}

scan_and_run_tests
