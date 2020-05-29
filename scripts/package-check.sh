#!/bin/bash
#
# Copyright (C) 2019 The Android Open Source Project
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

set -e

if [[ $# -le 1 ]]; then
  cat <<EOF
Usage:
  package-check.sh <jar-file> <package-list>
Checks that the class files in the <jar file> are in the <package-list> or
sub-packages.
EOF
  exit 1
fi

jar_file=$1
shift
if [[ ! -f ${jar_file} ]]; then
  echo "jar file \"${jar_file}\" does not exist."
  exit 1
fi

prefixes=()
while [[ $# -ge 1 ]]; do
  package="$1"
  if [[ "${package}" = */* ]]; then
    echo "Invalid package \"${package}\". Use dot notation for packages."
    exit 1
  fi
  # Transform to a slash-separated path and add a trailing slash to enforce
  # package name boundary.
  prefixes+=("${package//\./\/}/")
  shift
done

# Get the file names from the jar file.
zip_contents=`zipinfo -1 $jar_file`

# Check all class file names against the expected prefixes.
old_ifs=${IFS}
IFS=$'\n'
failed=false
for zip_entry in ${zip_contents}; do
  # Check the suffix.
  if [[ "${zip_entry}" = *.class ]]; then
    # Match against prefixes.
    found=false
    for prefix in ${prefixes[@]}; do
      if [[ "${zip_entry}" = "${prefix}"* ]]; then
        found=true
        break
      fi
    done
    if [[ "${found}" == "false" ]]; then
      echo "Class file ${zip_entry} is outside specified packages."
      failed=true
    fi
  fi
done
if [[ "${failed}" == "true" ]]; then
  exit 1
fi
IFS=${old_ifs}
