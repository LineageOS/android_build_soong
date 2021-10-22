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
    echo "This script is used to generate the native libraries backed by Mainline modules."
    echo ""
    echo "To run this script use: ./gen_ndk_backed_by_apex.sh \$OUTPUT_FILE_PATH \$MODULE_LIB1 \$MODULE_LIB2..."
    echo "For example: If output write to /backedby.txt then the command would be:"
    echo "./gen_ndk_backed_by_apex.sh /backedby.txt lib1.so lib2.so"
    echo "If the module1 is backing lib1 then the backedby.txt would contains: "
    echo "lib1.so lib2.so"
}

genAllBackedByList() {
  out="$1"
  shift
  rm -f "$out"
  touch "$out"
  echo "$@" >> "$out"
}

if [[ "$1" == "help" ]]
then
  printHelp
elif [[ "$#" -lt 1 ]]
then
  echo "Wrong argument length. Expecting at least 1 argument representing output path, followed by a list of libraries in the Mainline module."
else
  genAllBackedByList "$@"
fi
