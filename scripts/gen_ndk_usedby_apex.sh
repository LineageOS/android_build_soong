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
    echo "This script is used to generate the Mainline modules used-by NDK symbols."
    echo ""
    echo "To run this script use: ./ndk_usedby_module.sh \$BINARY_IMAGE_DIRECTORY \$BINARY_LLVM_PATH \$OUTPUT_FILE_PATH"
    echo "For example: If all the module image files that you would like to run is under directory '/myModule' and output write to /myModule.txt then the command would be:"
    echo "./ndk_usedby_module.sh /myModule \$BINARY_LLVM_PATH /myModule.txt"
}

parseReadelfOutput() {
  while IFS= read -r line
  do
      if [[ $line = *FUNC*GLOBAL*UND*@* ]] ;
      then
          echo "$line" | sed -r 's/.*UND (.*)@.*/\1/g' >> "$2"
      fi
  done < "$1"
  echo "" >> "$2"
}

unzipJarAndApk() {
  tmpUnzippedDir="$1"/tmpUnzipped
  [[ -e "$tmpUnzippedDir" ]] && rm -rf "$tmpUnzippedDir"
  mkdir -p "$tmpUnzippedDir"
  find "$1" -name "*.jar" -exec unzip -o {} -d "$tmpUnzippedDir" \;
  find "$1" -name "*.apk" -exec unzip -o {} -d "$tmpUnzippedDir" \;
  find "$tmpUnzippedDir" -name "*.MF" -exec rm {} \;
}

lookForExecFile() {
  dir="$1"
  readelf="$2"
  find "$dir" -type f -name "*.so"  -exec "$2" --dyn-symbols {} >> "$dir"/../tmpReadelf.txt \;
  find "$dir" -type f -perm /111 ! -name "*.so"  -exec "$2" --dyn-symbols {} >> "$dir"/../tmpReadelf.txt \;
}

if [[ "$1" == "help" ]]
then
  printHelp
elif [[ "$#" -ne 3 ]]
then
  echo "Wrong argument length. Expecting 3 argument representing image file directory, llvm-readelf tool path, output path."
else
  unzipJarAndApk "$1"
  lookForExecFile "$1" "$2"
  tmpReadelfOutput="$1/../tmpReadelf.txt"
  [[ -e "$3" ]] && rm "$3"
  parseReadelfOutput "$tmpReadelfOutput" "$3"
  [[ -e "$tmpReadelfOutput" ]] && rm "$tmpReadelfOutput"
  rm -rf "$1/tmpUnzipped"
fi