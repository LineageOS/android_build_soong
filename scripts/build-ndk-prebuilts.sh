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

TOP=$(pwd)

source build/envsetup.sh
PLATFORM_SDK_VERSION=$(get_build_var PLATFORM_SDK_VERSION)
PLATFORM_VERSION_ALL_CODENAMES=$(get_build_var PLATFORM_VERSION_ALL_CODENAMES)

# PLATFORM_VERSION_ALL_CODENAMES is a comma separated list like O,P. We need to
# turn this into ["O","P"].
PLATFORM_VERSION_ALL_CODENAMES=${PLATFORM_VERSION_ALL_CODENAMES/,/'","'}
PLATFORM_VERSION_ALL_CODENAMES="[\"${PLATFORM_VERSION_ALL_CODENAMES}\"]"

# Get the list of missing <uses-library> modules and convert it to a JSON array
# (quote module names, add comma separator and wrap in brackets).
MISSING_USES_LIBRARIES="$(get_build_var INTERNAL_PLATFORM_MISSING_USES_LIBRARIES)"
MISSING_USES_LIBRARIES="[$(echo $MISSING_USES_LIBRARIES | sed -e 's/\([^ ]\+\)/\"\1\"/g' -e 's/[ ]\+/, /g')]"

SOONG_OUT=${OUT_DIR}/soong
SOONG_NDK_OUT=${OUT_DIR}/soong/ndk
rm -rf ${SOONG_OUT}
mkdir -p ${SOONG_OUT}

# We only really need to set some of these variables, but soong won't merge this
# with the defaults, so we need to write out all the defaults with our values
# added.
cat > ${SOONG_OUT}/soong.variables << EOF
{
    "Platform_sdk_version": ${PLATFORM_SDK_VERSION},
    "Platform_version_active_codenames": ${PLATFORM_VERSION_ALL_CODENAMES},

    "DeviceName": "generic_arm64",
    "HostArch": "x86_64",
    "Malloc_not_svelte": false,
    "Safestack": false,

    "Ndk_abis": true,

    "VendorVars": {
        "art_module": {
            "source_build": "true"
        }
    },

    "MissingUsesLibraries": ${MISSING_USES_LIBRARIES}
}
EOF
m --skip-make ${SOONG_OUT}/ndk.timestamp

if [ -n "${DIST_DIR}" ]; then
    mkdir -p ${DIST_DIR} || true
    tar cjf ${DIST_DIR}/ndk_platform.tar.bz2 -C ${SOONG_OUT} ndk
fi
