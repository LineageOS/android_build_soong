#!/bin/bash -e

# Copyright (C) 2019 The Android Open Source Project
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

# This script freezes APIs of a sysprop_library after checking compatibility
# between latest API and current API.
#
# Usage: freeze-sysprop-api-files.sh <modulePath> <moduleName>
#
# <modulePath>: the directory, either relative or absolute, which holds the
# Android.bp file defining sysprop_library.
#
# <moduleName>: the name of sysprop_library to freeze API.
#
# Example:
# $ . build/envsetup.sh && lunch aosp_arm64-user
# $ . build/soong/scripts/freeze-sysprop-api-files.sh \
#       system/libsysprop/srcs PlatformProperties

if [[ -z "$1" || -z "$2" ]]; then
  echo "usage: $0 <modulePath> <moduleName>" >&2
  exit 1
fi

api_dir=$1/api

m "$2-check-api" && cp -f "${api_dir}/$2-current.txt" "${api_dir}/$2-latest.txt"
