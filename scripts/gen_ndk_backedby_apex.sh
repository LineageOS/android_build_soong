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
    echo "To run this script use: ./ndk_backedby_module.sh \$BINARY_IMAGE_DIRECTORY \$OUTPUT_FILE_PATH \$NDK_LIB_NAME_LIST"
    echo "For example: If all the module image files that you would like to run is under directory '/myModule' and output write to /backedby.txt then the command would be:"
    echo "./ndk_usedby_module.sh /myModule /backedby.txt /ndkLibList.txt"
    echo "If the module1 is backing lib1 then the backedby.txt would contains: "
    echo "lib1"
}

genBackedByList() {
  dir="$1"
  [[ ! -e "$2" ]] && echo "" >> "$2"
  while IFS= read -r line
  do
    soFileName=$(echo "$line" | sed 's/\(.*so\).*/\1/')
    if [[ ! -z "$soFileName" && "$soFileName" != *"#"* ]]
    then
      find "$dir" -type f -name "$soFileName" -exec echo "$soFileName" >> "$2" \;
    fi
  done < "$3"
}

if [[ "$1" == "help" ]]
then
  printHelp
elif [[ "$#" -ne 3 ]]
then
  echo "Wrong argument length. Expecting 3 argument representing image file directory, output path, path to ndk library list."
else
  [[ -e "$2" ]] && rm "$2"
  genBackedByList "$1" "$2" "$3"
fi
