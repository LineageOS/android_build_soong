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

printHelp() {
    echo "**************************** Usage Instructions ****************************"
    echo "This script is used to generate the Mainline modules used-by Java symbols."
    echo ""
    echo "To run this script use: ./gen_java_usedby_apex.sh \$BINARY_DEXDEPS_PATH \$OUTPUT_FILE_PATH \$JAR_AND_APK_LIST"
    echo "For example: If all jar and apk files are '/myJar.jar /myApk.apk' and output write to /myModule.txt then the command would be:"
    echo "./gen_java_usedby_apex.sh \$BINARY_DEXDEPS_PATH /myModule.txt /myJar.jar /myApk.apk"
}

genUsedByList() {
  dexdeps="$1"
  shift
  out="$1"
  shift
  rm -f "$out"
  touch "$out"
  echo "<externals>" >> "$out"
  for x in "$@"; do
    "$dexdeps" "$x" >> "$out" || echo "</external>" >> "$out"
  done
  echo "</externals>" >> "$out"
}

if [[ "$1" == "help" ]]
then
  printHelp
elif [[ "$#" -lt 2 ]]
then
  echo "Wrong argument length. Expecting at least 2 argument representing dexdeps path, output path, followed by a list of jar or apk files in the Mainline module."
else
  genUsedByList "$@"
fi