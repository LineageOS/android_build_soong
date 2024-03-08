#!/bin/bash

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

set -euo pipefail

# Soong/Bazel integration test to build the mainline modules in mixed build and
# compare the DCLA libs extracted from those modules to ensure they are identical.

if [ ! -e "build/make/core/Makefile" ]; then
  echo "$0 must be run from the top of the Android source tree."
  exit 1
fi

TARGET_PRODUCTS=(
  module_arm64
  module_x86_64
)

MODULES=(
  # These modules depend on the DCLA libs
  com.android.adbd
  com.android.art
  com.android.art.debug
  com.android.art.testing
  com.android.btservices
  com.android.conscrypt
  com.android.i18n
  com.android.media
  com.android.media.swcodec
  com.android.resolv
  com.android.runtime
  com.android.tethering
)

BAZEL_TARGETS=(
  //packages/modules/adb/apex:com.android.adbd
  //frameworks/av/apex:com.android.media.swcodec
)

DCLA_LIBS=(
  libbase.so
  libc++.so
  libcrypto.so
  libcutils.so
  libstagefright_flacdec.so
  libutils.so
)

if [[ -z ${OUT_DIR+x} ]]; then
  OUT_DIR="out"
fi

if [[ -z ${ANDROID_HOST_OUT+x} ]]; then
  export ANDROID_HOST_OUT="out/host/linux-x86"
fi

######################
# Build deapexer and debugfs
######################
DEAPEXER="${ANDROID_HOST_OUT}/bin/deapexer"
DEBUGFS="${ANDROID_HOST_OUT}/bin/debugfs"
if [[ ! -f "${DEAPEXER}" ]] || [[ ! -f "${DEBUGFS}" ]]; then
  build/soong/soong_ui.bash --make-mode --skip-soong-tests deapexer debugfs
fi

DEAPEXER="${DEAPEXER} --debugfs_path=${DEBUGFS}"

############
# Test Setup
############
OUTPUT_DIR="$(mktemp -d tmp.XXXXXX)"

function call_bazel() {
  build/bazel/bin/bazel $@
}

function cleanup {
  rm -rf "${OUTPUT_DIR}"
}
trap cleanup EXIT

#######
# Tests
#######

function extract_dcla_libs() {
  local product=$1; shift
  local modules=("$@"); shift

  for module in "${modules[@]}"; do
    local apex="${OUTPUT_DIR}/${product}/${module}.apex"
    local extract_dir="${OUTPUT_DIR}/${product}/${module}/extract"

    $DEAPEXER extract "${apex}" "${extract_dir}"
  done
}

function compare_dcla_libs() {
  local product=$1; shift
  local modules=("$@"); shift

  for lib in "${DCLA_LIBS[@]}"; do
    for arch in lib lib64; do
      local prev_sha=""
      for module in "${modules[@]}"; do
        local file="${OUTPUT_DIR}/${product}/${module}/extract/${arch}/${lib}"
        if [[ ! -f "${file}" ]]; then
          # not all libs are present in a module
          echo "file doesn't exist: ${file}"
          continue
        fi
        sha=$(sha1sum ${file})
        sha="${sha% *}"
        if [ "${prev_sha}" == "" ]; then
          prev_sha="${sha}"
        elif [ "${sha}" != "${prev_sha}" ] && { [ "${lib}" != "libcrypto.so" ] || [[ "${module}" != *"com.android.tethering" ]]; }; then
          echo "Test failed, ${lib} has different hash value"
          exit 1
        fi
      done
    done
  done
}

export UNBUNDLED_BUILD_SDKS_FROM_SOURCE=true # don't rely on prebuilts
export TARGET_BUILD_APPS="${MODULES[@]}"
for product in "${TARGET_PRODUCTS[@]}"; do
  ###########
  # Build the mainline modules
  ###########
  packages/modules/common/build/build_unbundled_mainline_module.sh \
    --product "${product}" \
    --dist_dir "${OUTPUT_DIR}/${product}"

  bazel_apexes=()
  if [[ -n ${TEST_BAZEL+x} ]] && [ "${TEST_BAZEL}" = true ]; then
    export TARGET_PRODUCT="${product/module/aosp}"
    call_bazel build --config=bp2build --config=ci --config=android "${BAZEL_TARGETS[@]}"
    for target in "${BAZEL_TARGETS[@]}"; do
      apex_path="$(realpath $(call_bazel cquery --config=bp2build --config=android --config=ci --output=files $target))"
      mkdir -p ${OUTPUT_DIR}/${product}
      bazel_apex="bazel_$(basename $apex_path)"
      mv $apex_path ${OUTPUT_DIR}/${product}/${bazel_apex}
      bazel_apexes+=(${bazel_apex%".apex"})
    done
  fi

  all_modeuls=(${MODULES[@]} ${bazel_apexes[@]})
  extract_dcla_libs "${product}" "${all_modeuls[@]}"
  compare_dcla_libs "${product}" "${all_modeuls[@]}"
done

echo "Test passed"
