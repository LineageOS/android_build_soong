#!/bin/bash -e
#
# Copyright 2019 Google Inc. All rights reserved.
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

#
# This is just a helper to run the tests under a few different environments
#

if [ -z $ANDROID_HOST_OUT ]; then
  echo "Must be run after running lunch"
  exit 1
fi

if [[ ( ! -f $ANDROID_HOST_OUT/nativetest64/par_test/par_test ) ||
      ( ! -f $ANDROID_HOST_OUT/bin/py3-cmd )]]; then
  echo "Run 'm par_test py2-cmd py3-cmd' first"
  exit 1
fi
if [ $(uname -s) = Linux ]; then
  if [[ ! -f $ANDROID_HOST_OUT/bin/py2-cmd ]]; then
    echo "Run 'm par_test py2-cmd py3-cmd' first"
    exit 1
  fi
fi

export LD_LIBRARY_PATH=$ANDROID_HOST_OUT/lib64

set -x

PYTHONHOME= PYTHONPATH= $ANDROID_HOST_OUT/nativetest64/par_test/par_test
PYTHONHOME=/usr $ANDROID_HOST_OUT/nativetest64/par_test/par_test
PYTHONPATH=/usr $ANDROID_HOST_OUT/nativetest64/par_test/par_test

ARGTEST=true $ANDROID_HOST_OUT/nativetest64/par_test/par_test --arg1 arg2

cd $(dirname ${BASH_SOURCE[0]})

if [ $(uname -s) = Linux ]; then
  PYTHONPATH=/extra $ANDROID_HOST_OUT/bin/py2-cmd py-cmd_test.py
fi
PYTHONPATH=/extra $ANDROID_HOST_OUT/bin/py3-cmd py-cmd_test.py

if [ $(uname -s) = Linux ]; then
  ARGTEST=true PYTHONPATH=/extra $ANDROID_HOST_OUT/bin/py2-cmd py-cmd_test.py arg1 arg2
  ARGTEST2=true PYTHONPATH=/extra $ANDROID_HOST_OUT/bin/py2-cmd py-cmd_test.py --arg1 arg2
fi

ARGTEST=true PYTHONPATH=/extra $ANDROID_HOST_OUT/bin/py3-cmd py-cmd_test.py arg1 arg2
ARGTEST2=true PYTHONPATH=/extra $ANDROID_HOST_OUT/bin/py3-cmd py-cmd_test.py --arg1 arg2

echo "Passed!"
