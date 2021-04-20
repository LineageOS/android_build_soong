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
#

"""Unit tests for lint_project_xml.py."""

import unittest
from xml.dom import minidom

import lint_project_xml


class CheckBaselineForDisallowedIssuesTest(unittest.TestCase):
  """Unit tests for check_baseline_for_disallowed_issues function."""

  baseline_xml = minidom.parseString(
      '<?xml version="1.0" encoding="utf-8"?>\n'
      '<issues format="5" by="lint 4.1.0" client="cli" variant="all" version="4.1.0">\n'
      '    <issue id="foo" message="foo is evil" errorLine1="foo()">\n'
      '        <location file="a/b/c.java" line="3" column="10"/>\n'
      '    </issue>\n'
      '    <issue id="bar" message="bar is known to be evil" errorLine1="bar()">\n'
      '        <location file="a/b/c.java" line="5" column="12"/>\n'
      '    </issue>\n'
      '    <issue id="baz" message="baz may be evil" errorLine1="a = baz()">\n'
      '        <location file="a/b/c.java" line="10" column="10"/>\n'
      '    </issue>\n'
      '    <issue id="foo" message="foo is evil" errorLine1="b = foo()">\n'
      '        <location file="a/d/e.java" line="100" column="4"/>\n'
      '    </issue>\n'
      '</issues>\n')

  def test_check_baseline_for_disallowed_issues(self):
    disallowed_issues = lint_project_xml.check_baseline_for_disallowed_issues(self.baseline_xml, ["foo", "bar", "qux"])
    self.assertEqual({"foo", "bar"}, disallowed_issues)


if __name__ == '__main__':
  unittest.main(verbosity=2)
