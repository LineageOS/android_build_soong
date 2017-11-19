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

# Script that takes a list of files on stdin and converts it to arguments to jar on stdout
# Usage:
#        find $dir -type f | sort | jar-args.sh $dir > jar_args.txt
#        jar cf out.jar @jar_args.txt

case $(uname) in
  Linux)
    extended_re=-r
    ;;
  Darwin)
    extended_re=-E
    ;;
  *) echo "unknown OS:" $(uname) >&2 && exit 1;;
esac

if [ "$1" == "--test" ]; then
  in=$(mktemp)
  expected=$(mktemp)
  out=$(mktemp)
  cat > $in <<EOF
a
a/b
a/b/'
a/b/"
a/b/\\
a/b/#
a/b/a
EOF
  cat > $expected <<EOF

-C 'a' 'b'
-C 'a' 'b/\\''
-C 'a' 'b/"'
-C 'a' 'b/\\\\'
-C 'a' 'b/#'
-C 'a' 'b/a'
EOF
  cat $in | $0 a > $out

  if cmp $out $expected; then
    status=0
    echo "PASS"
  else
    status=1
    echo "FAIL"
    echo "got:"
    cat $out
    echo "expected:"
    cat $expected
  fi
  rm -f $in $expected $out
  exit $status
fi

# In order, the regexps:
#   - Strip $1/ from the beginning of each line, and everything from lines that just have $1
#   - Escape single and double quotes, '#', ' ', and '\'
#   - Prefix each non-blank line with -C $1
sed ${extended_re} \
  -e"s,^$1(/|\$),," \
  -e"s,(['\\]),\\\\\1,g" \
  -e"s,^(.+),-C '$1' '\1',"
