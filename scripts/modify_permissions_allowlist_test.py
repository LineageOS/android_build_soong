#!/usr/bin/env python
#
# Copyright (C) 2022 The Android Open Source Project
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
"""Unit tests for modify_permissions_allowlist.py."""

from __future__ import print_function

import unittest

from xml.dom import minidom

from modify_permissions_allowlist import InvalidRootNodeException, InvalidNumberOfPrivappPermissionChildren, modify_allowlist


class ModifyPermissionsAllowlistTest(unittest.TestCase):

  def test_invalid_root(self):
    xml_data = '<foo></foo>'
    xml_dom = minidom.parseString(xml_data)
    self.assertRaises(InvalidRootNodeException, modify_allowlist, xml_dom, 'x')

  def test_no_packages(self):
    xml_data = '<permissions></permissions>'
    xml_dom = minidom.parseString(xml_data)
    self.assertRaises(
        InvalidNumberOfPrivappPermissionChildren, modify_allowlist, xml_dom, 'x'
    )

  def test_multiple_packages(self):
    xml_data = (
        '<permissions>'
        '  <privapp-permissions package="foo.bar"></privapp-permissions>'
        '  <privapp-permissions package="bar.baz"></privapp-permissions>'
        '</permissions>'
    )
    xml_dom = minidom.parseString(xml_data)
    self.assertRaises(
        InvalidNumberOfPrivappPermissionChildren, modify_allowlist, xml_dom, 'x'
    )

  def test_modify_package_name(self):
    xml_data = (
        '<permissions>'
        '  <privapp-permissions package="foo.bar">'
        '    <permission name="myperm1"/>'
        '  </privapp-permissions>'
        '</permissions>'
    )
    xml_dom = minidom.parseString(xml_data)
    modify_allowlist(xml_dom, 'bar.baz')
    expected_data = (
        '<?xml version="1.0" ?>'
        '<permissions>'
        '  <privapp-permissions package="bar.baz">'
        '    <permission name="myperm1"/>'
        '  </privapp-permissions>'
        '</permissions>'
    )
    self.assertEqual(expected_data, xml_dom.toxml())


if __name__ == '__main__':
  unittest.main(verbosity=2)
