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
  local readelfOutput=$1; shift
  local ndkApisOutput=$1; shift
  while IFS= read -r line
  do
      if [[ $line = *FUNC*GLOBAL*UND*@* ]] ;
      then
          echo "$line" | sed -r 's/.*UND (.*@.*)/\1/g' >> "${ndkApisOutput}"
      fi
  done < "${readelfOutput}"
  echo "" >> "${ndkApisOutput}"
}

unzipJarAndApk() {
  local dir="$1"; shift
  local tmpUnzippedDir="$1"; shift
  mkdir -p "${tmpUnzippedDir}"
  find "$dir" -name "*.jar" -exec unzip -o {} -d "${tmpUnzippedDir}" \;
  find "$dir" -name "*.apk" -exec unzip -o {} -d "${tmpUnzippedDir}" \;
  find "${tmpUnzippedDir}" -name "*.MF" -exec rm {} \;
}

lookForExecFile() {
  local dir="$1"; shift
  local readelf="$1"; shift
  local tmpOutput="$1"; shift
  find -L "$dir" -type f -name "*.so"  -exec "${readelf}" --dyn-symbols {} >> "${tmpOutput}" \;
  find -L "$dir" -type f -perm /111 ! -name "*.so" -exec "${readelf}" --dyn-symbols {} >> "${tmpOutput}" \;
}

if [[ "$1" == "help" ]]
then
  printHelp
elif [[ "$#" -ne 3 ]]
then
  echo "Wrong argument length. Expecting 3 argument representing image file directory, llvm-readelf tool path, output path."
else
  imageDir="$1"; shift
  readelf="$1"; shift
  outputFile="$1"; shift

  tmpReadelfOutput=$(mktemp /tmp/temporary-file.XXXXXXXX)
  tmpUnzippedDir=$(mktemp -d /tmp/temporary-dir.XXXXXXXX)
  trap 'rm -rf -- "${tmpReadelfOutput}" "${tmpUnzippedDir}"' EXIT

  # If there are any jars or apks, unzip them to surface native files.
  unzipJarAndApk "${imageDir}" "${tmpUnzippedDir}"
  # Analyze the unzipped files.
  lookForExecFile "${tmpUnzippedDir}" "${readelf}" "${tmpReadelfOutput}"

  # Analyze the apex image staging dir itself.
  lookForExecFile "${imageDir}" "${readelf}" "${tmpReadelfOutput}"

  [[ -e "${outputFile}" ]] && rm "${outputFile}"
  parseReadelfOutput "${tmpReadelfOutput}" "${outputFile}"
fi
