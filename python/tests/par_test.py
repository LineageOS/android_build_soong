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

import os
import site
import sys

# This file checks the visible python state against expected values when run
# inside a hermetic par file.

failed = False
def assert_equal(what, a, b):
    global failed
    if a != b:
        print("Expected %s('%s') == '%s'" % (what, a, b))
        failed = True

assert_equal("__name__", __name__, "__main__")
fileName = os.path.basename(__file__)
if fileName.endswith('.pyc'):
    fileName = fileName[:-1]
assert_equal("os.path.basename(__file__)", fileName, "par_test.py")

archive = os.path.dirname(__file__)
major = sys.version_info.major
minor = sys.version_info.minor

assert_equal("__package__", __package__, "")
assert_equal("sys.argv[0]", sys.argv[0], archive)
assert_equal("sys.executable", sys.executable, None)
assert_equal("sys.exec_prefix", sys.exec_prefix, archive)
assert_equal("sys.prefix", sys.prefix, archive)
assert_equal("__loader__.archive", __loader__.archive, archive)
assert_equal("site.ENABLE_USER_SITE", site.ENABLE_USER_SITE, None)

assert_equal("len(sys.path)", len(sys.path), 4)
assert_equal("sys.path[0]", sys.path[0], archive)
assert_equal("sys.path[1]", sys.path[1], os.path.join(archive, "internal", f"python{major}{minor}.zip"))
assert_equal("sys.path[2]", sys.path[2], os.path.join(archive, "internal", f"python{major}.{minor}"))
assert_equal("sys.path[3]", sys.path[3], os.path.join(archive, "internal", f"python{major}.{minor}", "lib-dynload"))

if os.getenv('ARGTEST', False):
    assert_equal("len(sys.argv)", len(sys.argv), 3)
    assert_equal("sys.argv[1]", sys.argv[1], "--arg1")
    assert_equal("sys.argv[2]", sys.argv[2], "arg2")
else:
    assert_equal("len(sys.argv)", len(sys.argv), 1)

if failed:
    sys.exit(1)

import testpkg.par_test
