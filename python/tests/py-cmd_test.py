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

import os
import site
import sys

# This file checks the visible python state against expected values when run
# using a prebuilt python.

failed = False
def assert_equal(what, a, b):
    global failed
    if a != b:
        print("Expected %s('%s') == '%s'" % (what, a, b))
        failed = True

assert_equal("__name__", __name__, "__main__")
assert_equal("os.path.basename(__file__)", os.path.basename(__file__), "py-cmd_test.py")

if os.getenv('ARGTEST', False):
    assert_equal("len(sys.argv)", len(sys.argv), 3)
    assert_equal("sys.argv[1]", sys.argv[1], "arg1")
    assert_equal("sys.argv[2]", sys.argv[2], "arg2")
elif os.getenv('ARGTEST2', False):
    assert_equal("len(sys.argv)", len(sys.argv), 3)
    assert_equal("sys.argv[1]", sys.argv[1], "--arg1")
    assert_equal("sys.argv[2]", sys.argv[2], "arg2")
else:
    assert_equal("len(sys.argv)", len(sys.argv), 1)

if os.getenv('ARGTEST_ONLY', False):
    if failed:
        sys.exit(1)
    sys.exit(0)

assert_equal("__package__", __package__, None)
assert_equal("sys.argv[0]", sys.argv[0], 'py-cmd_test.py')
if sys.version_info[0] == 2:
    assert_equal("basename(sys.executable)", os.path.basename(sys.executable), 'py2-cmd')
else:
    assert_equal("basename(sys.executable)", os.path.basename(sys.executable), 'py3-cmd')
assert_equal("sys.exec_prefix", sys.exec_prefix, sys.executable)
assert_equal("sys.prefix", sys.prefix, sys.executable)
assert_equal("site.ENABLE_USER_SITE", site.ENABLE_USER_SITE, None)

major = sys.version_info.major
minor = sys.version_info.minor

if major == 2:
    assert_equal("len(sys.path)", len(sys.path), 4)
    assert_equal("sys.path[0]", sys.path[0], os.path.abspath(os.path.dirname(__file__)))
    assert_equal("sys.path[1]", sys.path[1], "/extra")
    assert_equal("sys.path[2]", sys.path[2], os.path.join(sys.executable, "internal"))
    assert_equal("sys.path[3]", sys.path[3], os.path.join(sys.executable, "internal", "stdlib"))
else:
    assert_equal("len(sys.path)", len(sys.path), 5)
    assert_equal("sys.path[0]", sys.path[0], os.path.abspath(os.path.dirname(__file__)))
    assert_equal("sys.path[1]", sys.path[1], "/extra")
    assert_equal("sys.path[2]", sys.path[2], os.path.join(sys.executable, 'internal', 'python' + str(sys.version_info[0]) + str(sys.version_info[1]) + '.zip'))
    assert_equal("sys.path[3]", sys.path[3], os.path.join(sys.executable, 'internal', 'python' + str(sys.version_info[0]) + '.' + str(sys.version_info[1])))
    assert_equal("sys.path[4]", sys.path[4], os.path.join(sys.executable, 'internal', 'python' + str(sys.version_info[0]) + '.' + str(sys.version_info[1]), 'lib-dynload'))

if failed:
    sys.exit(1)

import testpkg.pycmd_test
