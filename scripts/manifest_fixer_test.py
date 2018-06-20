#!/usr/bin/env python
#
# Copyright (C) 2018 The Android Open Source Project
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
"""Unit tests for manifest_fixer_test.py."""

import StringIO
import sys
import unittest
from xml.dom import minidom

import manifest_fixer

sys.dont_write_bytecode = True


class CompareVersionGtTest(unittest.TestCase):
  """Unit tests for compare_version_gt function."""

  def test_sdk(self):
    """Test comparing sdk versions."""
    self.assertTrue(manifest_fixer.compare_version_gt('28', '27'))
    self.assertFalse(manifest_fixer.compare_version_gt('27', '28'))
    self.assertFalse(manifest_fixer.compare_version_gt('28', '28'))

  def test_codename(self):
    """Test comparing codenames."""
    self.assertTrue(manifest_fixer.compare_version_gt('Q', 'P'))
    self.assertFalse(manifest_fixer.compare_version_gt('P', 'Q'))
    self.assertFalse(manifest_fixer.compare_version_gt('Q', 'Q'))

  def test_sdk_codename(self):
    """Test comparing sdk versions with codenames."""
    self.assertTrue(manifest_fixer.compare_version_gt('Q', '28'))
    self.assertFalse(manifest_fixer.compare_version_gt('28', 'Q'))

  def test_compare_numeric(self):
    """Test that numbers are compared in numeric and not lexicographic order."""
    self.assertTrue(manifest_fixer.compare_version_gt('18', '8'))


class RaiseMinSdkVersionTest(unittest.TestCase):
  """Unit tests for raise_min_sdk_version function."""

  def raise_min_sdk_version_test(self, input_manifest, min_sdk_version):
    doc = minidom.parseString(input_manifest)
    manifest_fixer.raise_min_sdk_version(doc, min_sdk_version)
    output = StringIO.StringIO()
    manifest_fixer.write_xml(output, doc)
    return output.getvalue()

  manifest_tmpl = (
      '<?xml version="1.0" encoding="utf-8"?>\n'
      '<manifest xmlns:android="http://schemas.android.com/apk/res/android">\n'
      '%s'
      '</manifest>\n')

  def uses_sdk(self, v, extra=''):
    if extra:
      extra = ' ' + extra
    return '    <uses-sdk android:minSdkVersion="%s"%s/>\n' % (v, extra)

  def test_no_uses_sdk(self):
    """Tests inserting a uses-sdk element into a manifest."""

    manifest_input = self.manifest_tmpl % ''
    expected = self.manifest_tmpl % self.uses_sdk('28')
    output = self.raise_min_sdk_version_test(manifest_input, '28')
    self.assertEqual(output, expected)

  def test_no_min(self):
    """Tests inserting a minSdkVersion attribute into a uses-sdk element."""

    manifest_input = self.manifest_tmpl % '    <uses-sdk extra="foo"/>\n'
    expected = self.manifest_tmpl % self.uses_sdk('28', 'extra="foo"')
    output = self.raise_min_sdk_version_test(manifest_input, '28')
    self.assertEqual(output, expected)

  def test_raise_min(self):
    """Tests inserting a minSdkVersion attribute into a uses-sdk element."""

    manifest_input = self.manifest_tmpl % self.uses_sdk('27')
    expected = self.manifest_tmpl % self.uses_sdk('28')
    output = self.raise_min_sdk_version_test(manifest_input, '28')
    self.assertEqual(output, expected)

  def test_raise(self):
    """Tests raising a minSdkVersion attribute."""

    manifest_input = self.manifest_tmpl % self.uses_sdk('27')
    expected = self.manifest_tmpl % self.uses_sdk('28')
    output = self.raise_min_sdk_version_test(manifest_input, '28')
    self.assertEqual(output, expected)

  def test_no_raise_min(self):
    """Tests a minSdkVersion that doesn't need raising."""

    manifest_input = self.manifest_tmpl % self.uses_sdk('28')
    expected = manifest_input
    output = self.raise_min_sdk_version_test(manifest_input, '27')
    self.assertEqual(output, expected)

  def test_raise_codename(self):
    """Tests raising a minSdkVersion attribute to a codename."""

    manifest_input = self.manifest_tmpl % self.uses_sdk('28')
    expected = self.manifest_tmpl % self.uses_sdk('P')
    output = self.raise_min_sdk_version_test(manifest_input, 'P')
    self.assertEqual(output, expected)

  def test_no_raise_codename(self):
    """Tests a minSdkVersion codename that doesn't need raising."""

    manifest_input = self.manifest_tmpl % self.uses_sdk('P')
    expected = manifest_input
    output = self.raise_min_sdk_version_test(manifest_input, '28')
    self.assertEqual(output, expected)

  def test_extra(self):
    """Tests that extra attributes and elements are maintained."""

    manifest_input = self.manifest_tmpl % (
        '    <!-- comment -->\n'
        '    <uses-sdk android:minSdkVersion="27" extra="foo"/>\n'
        '    <application/>\n')

    expected = self.manifest_tmpl % (
        '    <!-- comment -->\n'
        '    <uses-sdk android:minSdkVersion="28" extra="foo"/>\n'
        '    <application/>\n')

    output = self.raise_min_sdk_version_test(manifest_input, '28')

    self.assertEqual(output, expected)

  def test_indent(self):
    """Tests that an inserted element copies the existing indentation."""

    manifest_input = self.manifest_tmpl % '  <!-- comment -->\n'

    expected = self.manifest_tmpl % (
        '  <uses-sdk android:minSdkVersion="28"/>\n'
        '  <!-- comment -->\n')

    output = self.raise_min_sdk_version_test(manifest_input, '28')

    self.assertEqual(output, expected)


class AddUsesLibrariesTest(unittest.TestCase):
  """Unit tests for add_uses_libraries function."""

  def run_test(self, input_manifest, new_uses_libraries):
    doc = minidom.parseString(input_manifest)
    manifest_fixer.add_uses_libraries(doc, new_uses_libraries)
    output = StringIO.StringIO()
    manifest_fixer.write_xml(output, doc)
    return output.getvalue()

  manifest_tmpl = (
      '<?xml version="1.0" encoding="utf-8"?>\n'
      '<manifest xmlns:android="http://schemas.android.com/apk/res/android">\n'
      '    <application>\n'
      '%s'
      '    </application>\n'
      '</manifest>\n')

  def uses_libraries(self, name_required_pairs):
    ret = ''
    for name, required in name_required_pairs:
      ret += (
          '        <uses-library android:name="%s" android:required="%s"/>\n'
      ) % (name, required)

    return ret

  def test_empty(self):
    """Empty new_uses_libraries must not touch the manifest."""
    manifest_input = self.manifest_tmpl % self.uses_libraries([
        ('foo', 'true'),
        ('bar', 'false')])
    expected = manifest_input
    output = self.run_test(manifest_input, [])
    self.assertEqual(output, expected)

  def test_not_overwrite(self):
    """new_uses_libraries must not overwrite existing tags."""
    manifest_input = self.manifest_tmpl % self.uses_libraries([
        ('foo', 'true'),
        ('bar', 'false')])
    expected = manifest_input
    output = self.run_test(manifest_input, ['foo', 'bar'])
    self.assertEqual(output, expected)

  def test_add(self):
    """New names are added with 'required:true'."""
    manifest_input = self.manifest_tmpl % self.uses_libraries([
        ('foo', 'true'),
        ('bar', 'false')])
    expected = self.manifest_tmpl % self.uses_libraries([
        ('foo', 'true'),
        ('bar', 'false'),
        ('baz', 'true'),
        ('qux', 'true')])
    output = self.run_test(manifest_input, ['bar', 'baz', 'qux'])
    self.assertEqual(output, expected)

  def test_no_application(self):
    """When there is no <application> tag, the tag is added."""
    manifest_input = (
        '<?xml version="1.0" encoding="utf-8"?>\n'
        '<manifest xmlns:android='
        '"http://schemas.android.com/apk/res/android">\n'
        '</manifest>\n')
    expected = self.manifest_tmpl % self.uses_libraries([
        ('foo', 'true'),
        ('bar', 'true')])
    output = self.run_test(manifest_input, ['foo', 'bar'])
    self.assertEqual(output, expected)

  def test_empty_application(self):
    """Even when here is an empty <application/> tag, the libs are added."""
    manifest_input = (
        '<?xml version="1.0" encoding="utf-8"?>\n'
        '<manifest xmlns:android='
        '"http://schemas.android.com/apk/res/android">\n'
        '    <application/>\n'
        '</manifest>\n')
    expected = self.manifest_tmpl % self.uses_libraries([
        ('foo', 'true'),
        ('bar', 'true')])
    output = self.run_test(manifest_input, ['foo', 'bar'])
    self.assertEqual(output, expected)


if __name__ == '__main__':
  unittest.main()
