# Copyright 2018 Google Inc. All rights reserved.
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

import os
import sys

# This file checks the visible python state against expected values when run
# inside a hermetic par file.

failed = False
def assert_equal(what, a, b):
    global failed
    if a != b:
        print("Expected %s('%s') == '%s'" % (what, a, b))
        failed = True

archive = sys.modules["__main__"].__loader__.archive

assert_equal("__name__", __name__, "testpkg.par_test")
assert_equal("__file__", __file__, os.path.join(archive, "testpkg/par_test.py"))

# Python3 is returning None here for me, and I haven't found any problems caused by this.
if sys.version_info[0] == 2:
  assert_equal("__package__", __package__, "testpkg")
else:
  assert_equal("__package__", __package__, None)

assert_equal("__loader__.archive", __loader__.archive, archive)
assert_equal("__loader__.prefix", __loader__.prefix, "testpkg/")

if failed:
    sys.exit(1)
