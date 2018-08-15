#!/bin/bash -e

# Copyright 2018 Google Inc. All rights reserved.
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

# Wrapper script to remove clang compiler flags rejected by clang-tidy.
# Inputs:
#  Environment:
#   CLANG_TIDY: path to the real clang-tidy program

# clang-tidy doesn't recognize every flag that clang compiler does.
# It gives clang-diagnostic-unused-command-line-argument warnings
# to -Wa,* flags.
# The -flto flags caused clang-tidy to ignore the -I flags,
# see https://bugs.llvm.org/show_bug.cgi?id=38332.
# -fsanitize and -fwhole-program-vtables need -flto.
args=("${@}")
n=${#args[@]}
for ((i=0; i<$n; ++i)); do
  case ${args[i]} in
    -Wa,*|-flto|-flto=*|-fsanitize=*|-fsanitize-*|-fwhole-program-vtables)
      unset args[i]
      ;;
  esac
done
${CLANG_TIDY} "${args[@]}"
