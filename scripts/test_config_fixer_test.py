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
"""Unit tests for test_config_fixer.py."""

import StringIO
import sys
import unittest
from xml.dom import minidom

import test_config_fixer

sys.dont_write_bytecode = True


class OverwritePackageNameTest(unittest.TestCase):
  """ Unit tests for overwrite_package_name function """

  manifest = (
      '<?xml version="1.0" encoding="utf-8"?>\n'
      '<manifest xmlns:android="http://schemas.android.com/apk/res/android"\n'
      '    package="com.android.foo">\n'
      '    <application>\n'
      '    </application>\n'
      '</manifest>\n')

  test_config = (
      '<?xml version="1.0" encoding="utf-8"?>\n'
      '<configuration description="Runs some tests.">\n'
      '    <option name="test-suite-tag" value="apct"/>\n'
      '    <target_preparer class="com.android.tradefed.targetprep.suite.SuiteApkInstaller">\n'
      '        <option name="package" value="%s"/>\n'
      '    </target_preparer>\n'
      '    <test class="com.android.tradefed.testtype.AndroidJUnitTest">\n'
      '        <option name="package" value="%s"/>\n'
      '        <option name="runtime-hint" value="20s"/>\n'
      '    </test>\n'
      '    <test class="com.android.tradefed.testtype.AndroidJUnitTest">\n'
      '        <option name="package" value="%s"/>\n'
      '        <option name="runtime-hint" value="15s"/>\n'
      '    </test>\n'
      '</configuration>\n')

  def test_all(self):
    doc = minidom.parseString(self.test_config % ("com.android.foo", "com.android.foo", "com.android.bar"))
    manifest = minidom.parseString(self.manifest)

    test_config_fixer.overwrite_package_name(doc, manifest, "com.soong.foo")
    output = StringIO.StringIO()
    test_config_fixer.write_xml(output, doc)

    # Only the matching package name in a test node should be updated.
    expected = self.test_config % ("com.android.foo", "com.soong.foo", "com.android.bar")
    self.assertEqual(expected, output.getvalue())


class OverwriteTestFileNameTest(unittest.TestCase):
  """ Unit tests for overwrite_test_file_name function """

  test_config = (
      '<?xml version="1.0" encoding="utf-8"?>\n'
      '<configuration description="Runs some tests.">\n'
      '    <target_preparer class="com.android.tradefed.targetprep.TestAppInstallSetup">\n'
      '        <option name="test-file-name" value="%s"/>\n'
      '    </target_preparer>\n'
      '    <test class="com.android.tradefed.testtype.AndroidJUnitTest">\n'
      '        <option name="package" value="com.android.foo"/>\n'
      '        <option name="runtime-hint" value="20s"/>\n'
      '    </test>\n'
      '</configuration>\n')

  def test_all(self):
    doc = minidom.parseString(self.test_config % ("foo.apk"))

    test_config_fixer.overwrite_test_file_name(doc, "bar.apk")
    output = StringIO.StringIO()
    test_config_fixer.write_xml(output, doc)

    # Only the matching package name in a test node should be updated.
    expected = self.test_config % ("bar.apk")
    self.assertEqual(expected, output.getvalue())


if __name__ == '__main__':
  unittest.main(verbosity=2)
