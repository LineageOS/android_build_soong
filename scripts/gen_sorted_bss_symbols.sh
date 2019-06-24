#!/bin/bash -e

# Copyright 2019 Google Inc. All rights reserved.
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

# Script to generate a symbol ordering file that sorts bss section symbols by
# their sizes.
# Inputs:
#  Environment:
#   CROSS_COMPILE: prefix added to nm tools
#  Arguments:
#   $1: Input ELF file
#   $2: Output symbol ordering file

set -o pipefail

${CROSS_COMPILE}nm --size-sort $1 | awk '{if ($2 == "b" || $2 == "B") print $3}' > $2
