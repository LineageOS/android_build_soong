#!/bin/bash -e

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

# Extracts the Java package names of all classes in the .jar files and writes a module-info.java
# file to stdout that exports all of those packages.

if [ -z "$1" ]; then
  echo "usage: $0 <module name> <jar1> [<jar2> ...]" >&2
  exit 1
fi

module_name=$1
shift

echo "module ${module_name} {"
for j in "$@"; do zipinfo -1 $j ; done \
  | grep -E '/[^/]*\.class$' \
  | sed 's|\(.*\)/[^/]*\.class$|    exports \1;|g' \
  | sed 's|/|.|g' \
  | sort -u
echo "}"
