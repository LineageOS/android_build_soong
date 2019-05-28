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

import sys
import unittest
from xml.dom import minidom

import manifest_check

sys.dont_write_bytecode = True


def uses_library(name, attr=''):
  return '<uses-library android:name="%s"%s />' % (name, attr)


def required(value):
  return ' android:required="%s"' % ('true' if value else 'false')


class EnforceUsesLibrariesTest(unittest.TestCase):
  """Unit tests for add_extract_native_libs function."""

  def run_test(self, input_manifest, uses_libraries=None, optional_uses_libraries=None):
    doc = minidom.parseString(input_manifest)
    try:
      manifest_check.enforce_uses_libraries(doc, uses_libraries, optional_uses_libraries)
      return True
    except manifest_check.ManifestMismatchError:
      return False

  manifest_tmpl = (
      '<?xml version="1.0" encoding="utf-8"?>\n'
      '<manifest xmlns:android="http://schemas.android.com/apk/res/android">\n'
      '    <application>\n'
      '    %s\n'
      '    </application>\n'
      '</manifest>\n')

  def test_uses_library(self):
    manifest_input = self.manifest_tmpl % (uses_library('foo'))
    matches = self.run_test(manifest_input, uses_libraries=['foo'])
    self.assertTrue(matches)

  def test_uses_library_required(self):
    manifest_input = self.manifest_tmpl % (uses_library('foo', required(True)))
    matches = self.run_test(manifest_input, uses_libraries=['foo'])
    self.assertTrue(matches)

  def test_optional_uses_library(self):
    manifest_input = self.manifest_tmpl % (uses_library('foo', required(False)))
    matches = self.run_test(manifest_input, optional_uses_libraries=['foo'])
    self.assertTrue(matches)

  def test_expected_uses_library(self):
    manifest_input = self.manifest_tmpl % (uses_library('foo', required(False)))
    matches = self.run_test(manifest_input, uses_libraries=['foo'])
    self.assertFalse(matches)

  def test_expected_optional_uses_library(self):
    manifest_input = self.manifest_tmpl % (uses_library('foo'))
    matches = self.run_test(manifest_input, optional_uses_libraries=['foo'])
    self.assertFalse(matches)

  def test_missing_uses_library(self):
    manifest_input = self.manifest_tmpl % ('')
    matches = self.run_test(manifest_input, uses_libraries=['foo'])
    self.assertFalse(matches)

  def test_missing_optional_uses_library(self):
    manifest_input = self.manifest_tmpl % ('')
    matches = self.run_test(manifest_input, optional_uses_libraries=['foo'])
    self.assertFalse(matches)

  def test_extra_uses_library(self):
    manifest_input = self.manifest_tmpl % (uses_library('foo'))
    matches = self.run_test(manifest_input)
    self.assertFalse(matches)

  def test_extra_optional_uses_library(self):
    manifest_input = self.manifest_tmpl % (uses_library('foo', required(False)))
    matches = self.run_test(manifest_input)
    self.assertFalse(matches)

  def test_multiple_uses_library(self):
    manifest_input = self.manifest_tmpl % ('\n'.join([uses_library('foo'),
                                                      uses_library('bar')]))
    matches = self.run_test(manifest_input, uses_libraries=['foo', 'bar'])
    self.assertTrue(matches)

  def test_multiple_optional_uses_library(self):
    manifest_input = self.manifest_tmpl % ('\n'.join([uses_library('foo', required(False)),
                                                      uses_library('bar', required(False))]))
    matches = self.run_test(manifest_input, optional_uses_libraries=['foo', 'bar'])
    self.assertTrue(matches)

  def test_order_uses_library(self):
    manifest_input = self.manifest_tmpl % ('\n'.join([uses_library('foo'),
                                                      uses_library('bar')]))
    matches = self.run_test(manifest_input, uses_libraries=['bar', 'foo'])
    self.assertFalse(matches)

  def test_order_optional_uses_library(self):
    manifest_input = self.manifest_tmpl % ('\n'.join([uses_library('foo', required(False)),
                                                      uses_library('bar', required(False))]))
    matches = self.run_test(manifest_input, optional_uses_libraries=['bar', 'foo'])
    self.assertFalse(matches)

  def test_duplicate_uses_library(self):
    manifest_input = self.manifest_tmpl % ('\n'.join([uses_library('foo'),
                                                      uses_library('foo')]))
    matches = self.run_test(manifest_input, uses_libraries=['foo'])
    self.assertTrue(matches)

  def test_duplicate_optional_uses_library(self):
    manifest_input = self.manifest_tmpl % ('\n'.join([uses_library('foo', required(False)),
                                                      uses_library('foo', required(False))]))
    matches = self.run_test(manifest_input, optional_uses_libraries=['foo'])
    self.assertTrue(matches)

  def test_mixed(self):
    manifest_input = self.manifest_tmpl % ('\n'.join([uses_library('foo'),
                                                      uses_library('bar', required(False))]))
    matches = self.run_test(manifest_input, uses_libraries=['foo'],
                            optional_uses_libraries=['bar'])
    self.assertTrue(matches)


class ExtractTargetSdkVersionTest(unittest.TestCase):
  def test_target_sdk_version(self):
    manifest = (
      '<?xml version="1.0" encoding="utf-8"?>\n'
      '<manifest xmlns:android="http://schemas.android.com/apk/res/android">\n'
      '    <uses-sdk android:minSdkVersion="28" android:targetSdkVersion="29" />\n'
      '</manifest>\n')
    doc = minidom.parseString(manifest)
    target_sdk_version = manifest_check.extract_target_sdk_version(doc)
    self.assertEqual(target_sdk_version, '29')

  def test_min_sdk_version(self):
    manifest = (
      '<?xml version="1.0" encoding="utf-8"?>\n'
      '<manifest xmlns:android="http://schemas.android.com/apk/res/android">\n'
      '    <uses-sdk android:minSdkVersion="28" />\n'
      '</manifest>\n')
    doc = minidom.parseString(manifest)
    target_sdk_version = manifest_check.extract_target_sdk_version(doc)
    self.assertEqual(target_sdk_version, '28')

if __name__ == '__main__':
  unittest.main(verbosity=2)
