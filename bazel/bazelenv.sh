#!/bin/bash

# Copyright 2020 Google Inc. All rights reserved.
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

# Helper script for setting environment variables required for Bazel/Soong
# mixed builds prototype. For development use only.
#
# Usage:
#   export BAZEL_PATH=[some_bazel_path] && source bazelenv.sh
#
# If BAZEL_PATH is not set, `which bazel` will be used
# to locate the appropriate bazel to use.


# Function to find top of the source tree (if $TOP isn't set) by walking up the
# tree.
function gettop
{
    local TOPFILE=build/soong/root.bp
    if [ -n "${TOP-}" -a -f "${TOP-}/${TOPFILE}" ] ; then
        # The following circumlocution ensures we remove symlinks from TOP.
        (cd $TOP; PWD= /bin/pwd)
    else
        if [ -f $TOPFILE ] ; then
            # The following circumlocution (repeated below as well) ensures
            # that we record the true directory name and not one that is
            # faked up with symlink names.
            PWD= /bin/pwd
        else
            local HERE=$PWD
            T=
            while [ \( ! \( -f $TOPFILE \) \) -a \( $PWD != "/" \) ]; do
                \cd ..
                T=`PWD= /bin/pwd -P`
            done
            \cd $HERE
            if [ -f "$T/$TOPFILE" ]; then
                echo $T
            fi
        fi
    fi
}

BASE_DIR="$(mktemp -d)"

if [ -z "$BAZEL_PATH" ] ; then
    export BAZEL_PATH="$(which bazel)"
fi

export USE_BAZEL=1
export BAZEL_HOME="$BASE_DIR/bazelhome"
export BAZEL_OUTPUT_BASE="$BASE_DIR/output"
export BAZEL_WORKSPACE="$(gettop)"

echo "USE_BAZEL=${USE_BAZEL}"
echo "BAZEL_PATH=${BAZEL_PATH}"
echo "BAZEL_HOME=${BAZEL_HOME}"
echo "BAZEL_OUTPUT_BASE=${BAZEL_OUTPUT_BASE}"
echo "BAZEL_WORKSPACE=${BAZEL_WORKSPACE}"

mkdir -p $BAZEL_HOME
mkdir -p $BAZEL_OUTPUT_BASE
