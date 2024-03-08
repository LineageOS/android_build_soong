#!/bin/bash

set -eu

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

# Tool to unpack an apex file and verify that the required files were extracted.
if [ $# -lt 6 ]; then
  echo "usage: $0 <deapaxer_path> <debugfs_path> <fsck.erofs_path> <apex file> <output_dir> <required_files>+" >&2
  exit 1
fi

DEAPEXER_PATH=$1
DEBUGFS_PATH=$2
FSCK_EROFS_PATH=$3
APEX_FILE=$4
OUTPUT_DIR=$5
shift 5
REQUIRED_PATHS=$@

rm -fr $OUTPUT_DIR
mkdir -p $OUTPUT_DIR

# Unpack the apex file contents.
$DEAPEXER_PATH --debugfs_path $DEBUGFS_PATH \
               --fsckerofs_path $FSCK_EROFS_PATH \
               extract $APEX_FILE $OUTPUT_DIR

# Verify that the files that the build expects to be in the .apex file actually
# exist, and make sure they have a fresh mtime to not confuse ninja.
typeset -i FAILED=0
for r in $REQUIRED_PATHS; do
  if [ ! -f $r ]; then
    echo "Required file $r not present in apex $APEX_FILE" >&2
    FAILED=$FAILED+1
  else
    # TODO(http:/b/177646343) - deapexer extracts the files with a timestamp of 1 Jan 1970.
    # touch the file so that ninja knows it has changed.
    touch $r
  fi
done

if [ $FAILED -gt 0 ]; then
  echo "$FAILED required files were missing from $APEX_FILE" >&2
  echo "Available files are:" >&2
  find $OUTPUT_DIR -type f | sed "s|^|    |" >&2
  exit 1
fi
