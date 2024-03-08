#!/bin/bash -ex

# Copyright 2017 Google Inc. All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

if [ -z "${OUT_DIR}" ]; then
    echo Must set OUT_DIR
    exit 1
fi

# Note: NDK doesn't support flagging APIs, so we hardcode it to trunk_staging.
# TODO: remove ALLOW_MISSING_DEPENDENCIES=true when all the riscv64
# dependencies exist (currently blocked by http://b/273792258).
# TODO: remove BUILD_BROKEN_DISABLE_BAZEL=1 when bazel supports riscv64 (http://b/262192655).
TARGET_RELEASE=trunk_staging \
ALLOW_MISSING_DEPENDENCIES=true \
BUILD_BROKEN_DISABLE_BAZEL=1 \
    TARGET_PRODUCT=ndk build/soong/soong_ui.bash --make-mode --soong-only ${OUT_DIR}/soong/ndk.timestamp

if [ -n "${DIST_DIR}" ]; then
    mkdir -p ${DIST_DIR} || true
    tar cjf ${DIST_DIR}/ndk_platform.tar.bz2 -C ${OUT_DIR}/soong ndk
fi
