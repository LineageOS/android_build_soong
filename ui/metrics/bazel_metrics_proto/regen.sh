#!/bin/bash -e

# Copyright 2022 Google Inc. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#   http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Generates the golang source file of bp2build_metrics.proto protobuf file.

function die() { echo "ERROR: $1" >&2; exit 1; }

readonly error_msg="Maybe you need to run 'lunch aosp_arm-eng && m aprotoc blueprint_tools'?"

if ! hash aprotoc &>/dev/null; then
  die "could not find aprotoc. ${error_msg}"
fi

if ! aprotoc --go_out=paths=source_relative:. bazel_metrics.proto; then
  die "build failed. ${error_msg}"
fi
