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

# Set of utility functions to build and run go code with microfactory
#
# Inputs:
#  ${TOP}: The top of the android source tree
#  ${OUT_DIR}: The output directory location (defaults to ${TOP}/out)
#  ${OUT_DIR_COMMON_BASE}: Change the default out directory to
#    ${OUT_DIR_COMMON_BASE}/$(basename ${TOP})

# Ensure GOROOT is set to the in-tree version.
case $(uname) in
    Linux)
        export GOROOT="${TOP}/prebuilts/go/linux-x86/"
        ;;
    Darwin)
        export GOROOT="${TOP}/prebuilts/go/darwin-x86/"
        ;;
    *) echo "unknown OS:" $(uname) >&2 && exit 1;;
esac

# Find the output directory
function getoutdir
{
    local out_dir="${OUT_DIR-}"
    if [ -z "${out_dir}" ]; then
        if [ "${OUT_DIR_COMMON_BASE-}" ]; then
            out_dir="${OUT_DIR_COMMON_BASE}/$(basename ${TOP})"
        else
            out_dir="out"
        fi
    fi
    if [[ "${out_dir}" != /* ]]; then
        out_dir="${TOP}/${out_dir}"
    fi
    echo "${out_dir}"
}

# Bootstrap microfactory from source if necessary and use it to build the
# requested binary.
#
# Arguments:
#  $1: name of the requested binary
#  $2: package name
function soong_build_go
{
    BUILDDIR=$(getoutdir) \
      SRCDIR=${TOP} \
      BLUEPRINTDIR=${TOP}/build/blueprint \
      EXTRA_ARGS="-pkg-path android/soong=${TOP}/build/soong -pkg-path github.com/golang/protobuf=${TOP}/external/golang-protobuf" \
      build_go $@
}

source ${TOP}/build/blueprint/microfactory/microfactory.bash
