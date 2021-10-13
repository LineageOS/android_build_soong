#!/usr/bin/env python3
#
# Copyright (C) 2021 The Android Open Source Project
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
"""A tool to report the current clang version used during build"""

import os
import re
import sys


ANDROID_BUILD_TOP = os.environ.get("ANDROID_BUILD_TOP", ".")
LLVM_PREBUILTS_VERSION = os.environ.get("LLVM_PREBUILTS_VERSION")

def get_clang_prebuilts_version(global_go):
  if LLVM_PREBUILTS_VERSION:
    return LLVM_PREBUILTS_VERSION

  # TODO(b/187231324): Get clang version from the json file once it is no longer
  # hard-coded in global.go
  if global_go is None:
      global_go = ANDROID_BUILD_TOP + '/build/soong/cc/config/global.go'
  with open(global_go) as infile:
    contents = infile.read()

  regex_rev = r'\tClangDefaultVersion\s+= "(?P<rev>clang-.*)"'
  match_rev = re.search(regex_rev, contents)
  if match_rev is None:
    raise RuntimeError('Parsing clang info failed')
  return match_rev.group('rev')


def main():
  global_go = sys.argv[1] if len(sys.argv) > 1 else None
  print(get_clang_prebuilts_version(global_go));


if __name__ == '__main__':
  main()
