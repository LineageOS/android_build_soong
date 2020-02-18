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
"""Unit tests for manifest_fixer.py."""

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

  def raise_min_sdk_version_test(self, input_manifest, min_sdk_version,
                                 target_sdk_version, library):
    doc = minidom.parseString(input_manifest)
    manifest_fixer.raise_min_sdk_version(doc, min_sdk_version,
                                         target_sdk_version, library)
    output = StringIO.StringIO()
    manifest_fixer.write_xml(output, doc)
    return output.getvalue()

  manifest_tmpl = (
      '<?xml version="1.0" encoding="utf-8"?>\n'
      '<manifest xmlns:android="http://schemas.android.com/apk/res/android">\n'
      '%s'
      '</manifest>\n')

  # pylint: disable=redefined-builtin
  def uses_sdk(self, min=None, target=None, extra=''):
    attrs = ''
    if min:
      attrs += ' android:minSdkVersion="%s"' % (min)
    if target:
      attrs += ' android:targetSdkVersion="%s"' % (target)
    if extra:
      attrs += ' ' + extra
    return '    <uses-sdk%s/>\n' % (attrs)

  def test_no_uses_sdk(self):
    """Tests inserting a uses-sdk element into a manifest."""

    manifest_input = self.manifest_tmpl % ''
    expected = self.manifest_tmpl % self.uses_sdk(min='28', target='28')
    output = self.raise_min_sdk_version_test(manifest_input, '28', '28', False)
    self.assertEqual(output, expected)

  def test_no_min(self):
    """Tests inserting a minSdkVersion attribute into a uses-sdk element."""

    manifest_input = self.manifest_tmpl % '    <uses-sdk extra="foo"/>\n'
    expected = self.manifest_tmpl % self.uses_sdk(min='28', target='28',
                                                  extra='extra="foo"')
    output = self.raise_min_sdk_version_test(manifest_input, '28', '28', False)
    self.assertEqual(output, expected)

  def test_raise_min(self):
    """Tests inserting a minSdkVersion attribute into a uses-sdk element."""

    manifest_input = self.manifest_tmpl % self.uses_sdk(min='27')
    expected = self.manifest_tmpl % self.uses_sdk(min='28', target='28')
    output = self.raise_min_sdk_version_test(manifest_input, '28', '28', False)
    self.assertEqual(output, expected)

  def test_raise(self):
    """Tests raising a minSdkVersion attribute."""

    manifest_input = self.manifest_tmpl % self.uses_sdk(min='27')
    expected = self.manifest_tmpl % self.uses_sdk(min='28', target='28')
    output = self.raise_min_sdk_version_test(manifest_input, '28', '28', False)
    self.assertEqual(output, expected)

  def test_no_raise_min(self):
    """Tests a minSdkVersion that doesn't need raising."""

    manifest_input = self.manifest_tmpl % self.uses_sdk(min='28')
    expected = self.manifest_tmpl % self.uses_sdk(min='28', target='27')
    output = self.raise_min_sdk_version_test(manifest_input, '27', '27', False)
    self.assertEqual(output, expected)

  def test_raise_codename(self):
    """Tests raising a minSdkVersion attribute to a codename."""

    manifest_input = self.manifest_tmpl % self.uses_sdk(min='28')
    expected = self.manifest_tmpl % self.uses_sdk(min='P', target='P')
    output = self.raise_min_sdk_version_test(manifest_input, 'P', 'P', False)
    self.assertEqual(output, expected)

  def test_no_raise_codename(self):
    """Tests a minSdkVersion codename that doesn't need raising."""

    manifest_input = self.manifest_tmpl % self.uses_sdk(min='P')
    expected = self.manifest_tmpl % self.uses_sdk(min='P', target='28')
    output = self.raise_min_sdk_version_test(manifest_input, '28', '28', False)
    self.assertEqual(output, expected)

  def test_target(self):
    """Tests an existing targetSdkVersion is preserved."""

    manifest_input = self.manifest_tmpl % self.uses_sdk(min='26', target='27')
    expected = self.manifest_tmpl % self.uses_sdk(min='28', target='27')
    output = self.raise_min_sdk_version_test(manifest_input, '28', '29', False)
    self.assertEqual(output, expected)

  def test_no_target(self):
    """Tests inserting targetSdkVersion when minSdkVersion exists."""

    manifest_input = self.manifest_tmpl % self.uses_sdk(min='27')
    expected = self.manifest_tmpl % self.uses_sdk(min='28', target='29')
    output = self.raise_min_sdk_version_test(manifest_input, '28', '29', False)
    self.assertEqual(output, expected)

  def test_target_no_min(self):
    """"Tests inserting targetSdkVersion when minSdkVersion exists."""

    manifest_input = self.manifest_tmpl % self.uses_sdk(target='27')
    expected = self.manifest_tmpl % self.uses_sdk(min='28', target='27')
    output = self.raise_min_sdk_version_test(manifest_input, '28', '29', False)
    self.assertEqual(output, expected)

  def test_no_target_no_min(self):
    """Tests inserting targetSdkVersion when minSdkVersion does not exist."""

    manifest_input = self.manifest_tmpl % ''
    expected = self.manifest_tmpl % self.uses_sdk(min='28', target='29')
    output = self.raise_min_sdk_version_test(manifest_input, '28', '29', False)
    self.assertEqual(output, expected)

  def test_library_no_target(self):
    """Tests inserting targetSdkVersion when minSdkVersion exists."""

    manifest_input = self.manifest_tmpl % self.uses_sdk(min='27')
    expected = self.manifest_tmpl % self.uses_sdk(min='28', target='15')
    output = self.raise_min_sdk_version_test(manifest_input, '28', '29', True)
    self.assertEqual(output, expected)

  def test_library_target_no_min(self):
    """Tests inserting targetSdkVersion when minSdkVersion exists."""

    manifest_input = self.manifest_tmpl % self.uses_sdk(target='27')
    expected = self.manifest_tmpl % self.uses_sdk(min='28', target='27')
    output = self.raise_min_sdk_version_test(manifest_input, '28', '29', True)
    self.assertEqual(output, expected)

  def test_library_no_target_no_min(self):
    """Tests inserting targetSdkVersion when minSdkVersion does not exist."""

    manifest_input = self.manifest_tmpl % ''
    expected = self.manifest_tmpl % self.uses_sdk(min='28', target='15')
    output = self.raise_min_sdk_version_test(manifest_input, '28', '29', True)
    self.assertEqual(output, expected)

  def test_extra(self):
    """Tests that extra attributes and elements are maintained."""

    manifest_input = self.manifest_tmpl % (
        '    <!-- comment -->\n'
        '    <uses-sdk android:minSdkVersion="27" extra="foo"/>\n'
        '    <application/>\n')

    # pylint: disable=line-too-long
    expected = self.manifest_tmpl % (
        '    <!-- comment -->\n'
        '    <uses-sdk android:minSdkVersion="28" android:targetSdkVersion="29" extra="foo"/>\n'
        '    <application/>\n')

    output = self.raise_min_sdk_version_test(manifest_input, '28', '29', False)

    self.assertEqual(output, expected)

  def test_indent(self):
    """Tests that an inserted element copies the existing indentation."""

    manifest_input = self.manifest_tmpl % '  <!-- comment -->\n'

    # pylint: disable=line-too-long
    expected = self.manifest_tmpl % (
        '  <uses-sdk android:minSdkVersion="28" android:targetSdkVersion="29"/>\n'
        '  <!-- comment -->\n')

    output = self.raise_min_sdk_version_test(manifest_input, '28', '29', False)

    self.assertEqual(output, expected)


class AddLoggingParentTest(unittest.TestCase):
  """Unit tests for add_logging_parent function."""

  def add_logging_parent_test(self, input_manifest, logging_parent=None):
    doc = minidom.parseString(input_manifest)
    if logging_parent:
      manifest_fixer.add_logging_parent(doc, logging_parent)
    output = StringIO.StringIO()
    manifest_fixer.write_xml(output, doc)
    return output.getvalue()

  manifest_tmpl = (
      '<?xml version="1.0" encoding="utf-8"?>\n'
      '<manifest xmlns:android="http://schemas.android.com/apk/res/android">\n'
      '%s'
      '</manifest>\n')

  def uses_logging_parent(self, logging_parent=None):
    attrs = ''
    if logging_parent:
      meta_text = ('<meta-data android:name="android.content.pm.LOGGING_PARENT" '
                   'android:value="%s"/>\n') % (logging_parent)
      attrs += '    <application>\n        %s    </application>\n' % (meta_text)

    return attrs

  def test_no_logging_parent(self):
    """Tests manifest_fixer with no logging_parent."""
    manifest_input = self.manifest_tmpl % ''
    expected = self.manifest_tmpl % self.uses_logging_parent()
    output = self.add_logging_parent_test(manifest_input)
    self.assertEqual(output, expected)

  def test_logging_parent(self):
    """Tests manifest_fixer with no logging_parent."""
    manifest_input = self.manifest_tmpl % ''
    expected = self.manifest_tmpl % self.uses_logging_parent('FOO')
    output = self.add_logging_parent_test(manifest_input, 'FOO')
    self.assertEqual(output, expected)


class AddUsesLibrariesTest(unittest.TestCase):
  """Unit tests for add_uses_libraries function."""

  def run_test(self, input_manifest, new_uses_libraries):
    doc = minidom.parseString(input_manifest)
    manifest_fixer.add_uses_libraries(doc, new_uses_libraries, True)
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


class AddUsesNonSdkApiTest(unittest.TestCase):
  """Unit tests for add_uses_libraries function."""

  def run_test(self, input_manifest):
    doc = minidom.parseString(input_manifest)
    manifest_fixer.add_uses_non_sdk_api(doc)
    output = StringIO.StringIO()
    manifest_fixer.write_xml(output, doc)
    return output.getvalue()

  manifest_tmpl = (
      '<?xml version="1.0" encoding="utf-8"?>\n'
      '<manifest xmlns:android="http://schemas.android.com/apk/res/android">\n'
      '    <application%s/>\n'
      '</manifest>\n')

  def uses_non_sdk_api(self, value):
    return ' android:usesNonSdkApi="true"' if value else ''

  def test_set_true(self):
    """Empty new_uses_libraries must not touch the manifest."""
    manifest_input = self.manifest_tmpl % self.uses_non_sdk_api(False)
    expected = self.manifest_tmpl % self.uses_non_sdk_api(True)
    output = self.run_test(manifest_input)
    self.assertEqual(output, expected)

  def test_already_set(self):
    """new_uses_libraries must not overwrite existing tags."""
    manifest_input = self.manifest_tmpl % self.uses_non_sdk_api(True)
    expected = manifest_input
    output = self.run_test(manifest_input)
    self.assertEqual(output, expected)


class UseEmbeddedDexTest(unittest.TestCase):
  """Unit tests for add_use_embedded_dex function."""

  def run_test(self, input_manifest):
    doc = minidom.parseString(input_manifest)
    manifest_fixer.add_use_embedded_dex(doc)
    output = StringIO.StringIO()
    manifest_fixer.write_xml(output, doc)
    return output.getvalue()

  manifest_tmpl = (
      '<?xml version="1.0" encoding="utf-8"?>\n'
      '<manifest xmlns:android="http://schemas.android.com/apk/res/android">\n'
      '    <application%s/>\n'
      '</manifest>\n')

  def use_embedded_dex(self, value):
    return ' android:useEmbeddedDex="%s"' % value

  def test_manifest_with_undeclared_preference(self):
    manifest_input = self.manifest_tmpl % ''
    expected = self.manifest_tmpl % self.use_embedded_dex('true')
    output = self.run_test(manifest_input)
    self.assertEqual(output, expected)

  def test_manifest_with_use_embedded_dex(self):
    manifest_input = self.manifest_tmpl % self.use_embedded_dex('true')
    expected = manifest_input
    output = self.run_test(manifest_input)
    self.assertEqual(output, expected)

  def test_manifest_with_not_use_embedded_dex(self):
    manifest_input = self.manifest_tmpl % self.use_embedded_dex('false')
    self.assertRaises(RuntimeError, self.run_test, manifest_input)


class AddExtractNativeLibsTest(unittest.TestCase):
  """Unit tests for add_extract_native_libs function."""

  def run_test(self, input_manifest, value):
    doc = minidom.parseString(input_manifest)
    manifest_fixer.add_extract_native_libs(doc, value)
    output = StringIO.StringIO()
    manifest_fixer.write_xml(output, doc)
    return output.getvalue()

  manifest_tmpl = (
      '<?xml version="1.0" encoding="utf-8"?>\n'
      '<manifest xmlns:android="http://schemas.android.com/apk/res/android">\n'
      '    <application%s/>\n'
      '</manifest>\n')

  def extract_native_libs(self, value):
    return ' android:extractNativeLibs="%s"' % value

  def test_set_true(self):
    manifest_input = self.manifest_tmpl % ''
    expected = self.manifest_tmpl % self.extract_native_libs('true')
    output = self.run_test(manifest_input, True)
    self.assertEqual(output, expected)

  def test_set_false(self):
    manifest_input = self.manifest_tmpl % ''
    expected = self.manifest_tmpl % self.extract_native_libs('false')
    output = self.run_test(manifest_input, False)
    self.assertEqual(output, expected)

  def test_match(self):
    manifest_input = self.manifest_tmpl % self.extract_native_libs('true')
    expected = manifest_input
    output = self.run_test(manifest_input, True)
    self.assertEqual(output, expected)

  def test_conflict(self):
    manifest_input = self.manifest_tmpl % self.extract_native_libs('true')
    self.assertRaises(RuntimeError, self.run_test, manifest_input, False)


class AddNoCodeApplicationTest(unittest.TestCase):
  """Unit tests for set_has_code_to_false function."""

  def run_test(self, input_manifest):
    doc = minidom.parseString(input_manifest)
    manifest_fixer.set_has_code_to_false(doc)
    output = StringIO.StringIO()
    manifest_fixer.write_xml(output, doc)
    return output.getvalue()

  manifest_tmpl = (
      '<?xml version="1.0" encoding="utf-8"?>\n'
      '<manifest xmlns:android="http://schemas.android.com/apk/res/android">\n'
      '%s'
      '</manifest>\n')

  def test_no_application(self):
    manifest_input = self.manifest_tmpl % ''
    expected = self.manifest_tmpl % '    <application android:hasCode="false"/>\n'
    output = self.run_test(manifest_input)
    self.assertEqual(output, expected)

  def test_has_application_no_has_code(self):
    manifest_input = self.manifest_tmpl % '    <application/>\n'
    expected = self.manifest_tmpl % '    <application android:hasCode="false"/>\n'
    output = self.run_test(manifest_input)
    self.assertEqual(output, expected)

  def test_has_application_has_code_false(self):
    """ Do nothing if there's already an application elemeent. """
    manifest_input = self.manifest_tmpl % '    <application android:hasCode="false"/>\n'
    output = self.run_test(manifest_input)
    self.assertEqual(output, manifest_input)

  def test_has_application_has_code_true(self):
    """ Do nothing if there's already an application elemeent even if its
     hasCode attribute is true. """
    manifest_input = self.manifest_tmpl % '    <application android:hasCode="true"/>\n'
    output = self.run_test(manifest_input)
    self.assertEqual(output, manifest_input)


if __name__ == '__main__':
  unittest.main(verbosity=2)
