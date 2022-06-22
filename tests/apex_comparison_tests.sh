#!/bin/bash

# Copyright (C) 2022 The Android Open Source Project
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

set -euo pipefail

# Soong/Bazel integration test for building unbundled apexes in the real source tree.
#
# These tests build artifacts from head and compares their contents.

if [ ! -e "build/make/core/Makefile" ]; then
  echo "$0 must be run from the top of the Android source tree."
  exit 1
fi

############
# Test Setup
############

OUTPUT_DIR="$(mktemp -d)"
SOONG_OUTPUT_DIR="$OUTPUT_DIR/soong"
BAZEL_OUTPUT_DIR="$OUTPUT_DIR/bazel"

function cleanup {
  # call bazel clean because some bazel outputs don't have w bits.
  call_bazel clean
  rm -rf "${OUTPUT_DIR}"
}
trap cleanup EXIT

###########
# Run Soong
###########
export UNBUNDLED_BUILD_SDKS_FROM_SOURCE=true # don't rely on prebuilts
export TARGET_BUILD_APPS="com.android.adbd com.android.tzdata build.bazel.examples.apex.minimal"
packages/modules/common/build/build_unbundled_mainline_module.sh \
  --product module_arm \
  --dist_dir "$SOONG_OUTPUT_DIR"

######################
# Run bp2build / Bazel
######################
build/soong/soong_ui.bash --make-mode BP2BUILD_VERBOSE=1 --skip-soong-tests bp2build

function call_bazel() {
  tools/bazel --output_base="$BAZEL_OUTPUT_DIR" $@
}
BAZEL_OUT="$(call_bazel info output_path)"

call_bazel build --config=bp2build --config=ci --config=android_arm \
  //packages/modules/adb/apex:com.android.adbd \
  //system/timezone/apex:com.android.tzdata \
  //build/bazel/examples/apex/minimal:build.bazel.examples.apex.minimal.apex

# Build debugfs separately, as it's not a dep of apexer, but needs to be an explicit arg.
call_bazel build --config=bp2build --config=linux_x86_64 //external/e2fsprogs/debugfs
DEBUGFS_PATH="$BAZEL_OUT/linux_x86_64-fastbuild/bin/external/e2fsprogs/debugfs/debugfs"

function run_deapexer() {
  call_bazel run --config=bp2build --config=linux_x86_64 //system/apex/tools:deapexer \
    -- \
    --debugfs_path="$DEBUGFS_PATH" \
    $@
}

#######
# Tests
#######

function compare_deapexer_list() {
  local APEX_DIR=$1; shift
  local APEX=$1; shift

  # Compare the outputs of `deapexer list`, which lists the contents of the apex filesystem image.
  local SOONG_APEX="$SOONG_OUTPUT_DIR/$APEX"
  local BAZEL_APEX="$BAZEL_OUT/android_arm-fastbuild/bin/$APEX_DIR/$APEX"

  local SOONG_LIST="$OUTPUT_DIR/soong.list"
  local BAZEL_LIST="$OUTPUT_DIR/bazel.list"

  run_deapexer list "$SOONG_APEX" > "$SOONG_LIST"
  run_deapexer list "$BAZEL_APEX" > "$BAZEL_LIST"

  if cmp -s "$SOONG_LIST" "$BAZEL_LIST"
  then
    echo "ok: $APEX"
  else
    echo "contents of $APEX are different between Soong and Bazel:"
    echo
    echo expected
    echo
    cat "$SOONG_LIST"
    echo
    echo got
    echo
    cat "$BAZEL_LIST"
    exit 1
  fi
}

compare_deapexer_list packages/modules/adb/apex com.android.adbd.apex
compare_deapexer_list system/timezone/apex com.android.tzdata.apex
compare_deapexer_list build/bazel/examples/apex/minimal build.bazel.examples.apex.minimal.apex
