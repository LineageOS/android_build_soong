#!/bin/bash -e

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

# Generates NDK API txt file used by Mainline modules. NDK APIs would have value
# "UND" in Ndx column and have suffix "@LIB_NAME" in Name column.
# For example, current line llvm-readelf output is:
# 1: 00000000     0     FUNC      GLOBAL  DEFAULT   UND   dlopen@LIBC
# After the parse function below "dlopen" would be write to the output file.
printHelp() {
    echo "**************************** Usage Instructions ****************************"
    echo "This script is used to generate the Mainline modules backed-by NDK symbols."
    echo ""
    echo "To run this script use: ./gen_ndk_backed_by_apex.sh \$OUTPUT_FILE_PATH \$NDK_LIB_NAME_LIST \$MODULE_LIB1 \$MODULE_LIB2..."
    echo "For example: If output write to /backedby.txt then the command would be:"
    echo "./gen_ndk_backed_by_apex.sh /backedby.txt /ndkLibList.txt lib1.so lib2.so"
    echo "If the module1 is backing lib1 then the backedby.txt would contains: "
    echo "lib1"
}

contains() {
  val="$1"
  shift
  for x in "$@"; do
    if [ "$x" = "$val" ]; then
      return 0
    fi
  done
  return 1
}


genBackedByList() {
  out="$1"
  shift
  ndk_list="$1"
  shift
  rm -f "$out"
  touch "$out"
  while IFS= read -r line
  do
    soFileName=$(echo "$line" | sed 's/\(.*so\).*/\1/')
    if [[ ! -z "$soFileName" && "$soFileName" != *"#"* ]]
    then
      if contains "$soFileName" "$@"; then
        echo "$soFileName" >> "$out"
      fi
    fi
  done < "$ndk_list"
}

if [[ "$1" == "help" ]]
then
  printHelp
elif [[ "$#" -lt 2 ]]
then
  echo "Wrong argument length. Expecting at least 2 argument representing output path, path to ndk library list, followed by a list of libraries in the Mainline module."
else
  genBackedByList "$@"
fi
