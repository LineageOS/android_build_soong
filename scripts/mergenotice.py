#!/usr/bin/env python
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
#
"""
Merges input notice files to the output file while ignoring duplicated files
This script shouldn't be confused with build/make/tools/generate-notice-files.py
which is responsible for creating the final notice file for all artifacts
installed. This script has rather limited scope; it is meant to create a merged
notice file for a set of modules that are packaged together, e.g. in an APEX.
The merged notice file does not reveal the individual files in the package.
"""

import sys
import argparse

def get_args():
  parser = argparse.ArgumentParser(description='Merge notice files.')
  parser.add_argument('--output', help='output file path.')
  parser.add_argument('inputs', metavar='INPUT', nargs='+',
                      help='input notice file')
  return parser.parse_args()

def main(argv):
  args = get_args()

  processed = set()
  with open(args.output, 'w+') as output:
    for input in args.inputs:
      with open(input, 'r') as f:
        data = f.read().strip()
        if data not in processed:
          processed.add(data)
          output.write('%s\n\n' % data)

if __name__ == '__main__':
  main(sys.argv)
