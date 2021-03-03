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


def uses_library_xml(name, attr=''):
  return '<uses-library android:name="%s"%s />' % (name, attr)


def required_xml(value):
  return ' android:required="%s"' % ('true' if value else 'false')


def uses_library_apk(name, sfx=''):
  return "uses-library%s:'%s'" % (sfx, name)


def required_apk(value):
  return '' if value else '-not-required'


class EnforceUsesLibrariesTest(unittest.TestCase):
  """Unit tests for add_extract_native_libs function."""

  def run_test(self, xml, apk, uses_libraries=[], optional_uses_libraries=[]):
    doc = minidom.parseString(xml)
    try:
      relax = False
      manifest_check.enforce_uses_libraries(doc, uses_libraries,
        optional_uses_libraries, relax, is_apk=False)
      manifest_check.enforce_uses_libraries(apk, uses_libraries,
        optional_uses_libraries, relax, is_apk=True)
      return True
    except manifest_check.ManifestMismatchError:
      return False

  xml_tmpl = (
      '<?xml version="1.0" encoding="utf-8"?>\n'
      '<manifest xmlns:android="http://schemas.android.com/apk/res/android">\n'
      '    <application>\n'
      '    %s\n'
      '    </application>\n'
      '</manifest>\n')

  apk_tmpl = (
      "package: name='com.google.android.something' versionCode='100'\n"
      "sdkVersion:'29'\n"
      "targetSdkVersion:'29'\n"
      "uses-permission: name='android.permission.ACCESS_NETWORK_STATE'\n"
      "%s\n"
      "densities: '160' '240' '320' '480' '640' '65534")

  def test_uses_library(self):
    xml = self.xml_tmpl % (uses_library_xml('foo'))
    apk = self.apk_tmpl % (uses_library_apk('foo'))
    matches = self.run_test(xml, apk, uses_libraries=['foo'])
    self.assertTrue(matches)

  def test_uses_library_required(self):
    xml = self.xml_tmpl % (uses_library_xml('foo', required_xml(True)))
    apk = self.apk_tmpl % (uses_library_apk('foo', required_apk(True)))
    matches = self.run_test(xml, apk, uses_libraries=['foo'])
    self.assertTrue(matches)

  def test_optional_uses_library(self):
    xml = self.xml_tmpl % (uses_library_xml('foo', required_xml(False)))
    apk = self.apk_tmpl % (uses_library_apk('foo', required_apk(False)))
    matches = self.run_test(xml, apk, optional_uses_libraries=['foo'])
    self.assertTrue(matches)

  def test_expected_uses_library(self):
    xml = self.xml_tmpl % (uses_library_xml('foo', required_xml(False)))
    apk = self.apk_tmpl % (uses_library_apk('foo', required_apk(False)))
    matches = self.run_test(xml, apk, uses_libraries=['foo'])
    self.assertFalse(matches)

  def test_expected_optional_uses_library(self):
    xml = self.xml_tmpl % (uses_library_xml('foo'))
    apk = self.apk_tmpl % (uses_library_apk('foo'))
    matches = self.run_test(xml, apk, optional_uses_libraries=['foo'])
    self.assertFalse(matches)

  def test_missing_uses_library(self):
    xml = self.xml_tmpl % ('')
    apk = self.apk_tmpl % ('')
    matches = self.run_test(xml, apk, uses_libraries=['foo'])
    self.assertFalse(matches)

  def test_missing_optional_uses_library(self):
    xml = self.xml_tmpl % ('')
    apk = self.apk_tmpl % ('')
    matches = self.run_test(xml, apk, optional_uses_libraries=['foo'])
    self.assertFalse(matches)

  def test_extra_uses_library(self):
    xml = self.xml_tmpl % (uses_library_xml('foo'))
    apk = self.apk_tmpl % (uses_library_xml('foo'))
    matches = self.run_test(xml, apk)
    self.assertFalse(matches)

  def test_extra_optional_uses_library(self):
    xml = self.xml_tmpl % (uses_library_xml('foo', required_xml(False)))
    apk = self.apk_tmpl % (uses_library_apk('foo', required_apk(False)))
    matches = self.run_test(xml, apk)
    self.assertFalse(matches)

  def test_multiple_uses_library(self):
    xml = self.xml_tmpl % ('\n'.join([uses_library_xml('foo'),
                                      uses_library_xml('bar')]))
    apk = self.apk_tmpl % ('\n'.join([uses_library_apk('foo'),
                                      uses_library_apk('bar')]))
    matches = self.run_test(xml, apk, uses_libraries=['foo', 'bar'])
    self.assertTrue(matches)

  def test_multiple_optional_uses_library(self):
    xml = self.xml_tmpl % ('\n'.join([uses_library_xml('foo', required_xml(False)),
                                      uses_library_xml('bar', required_xml(False))]))
    apk = self.apk_tmpl % ('\n'.join([uses_library_apk('foo', required_apk(False)),
                                      uses_library_apk('bar', required_apk(False))]))
    matches = self.run_test(xml, apk, optional_uses_libraries=['foo', 'bar'])
    self.assertTrue(matches)

  def test_order_uses_library(self):
    xml = self.xml_tmpl % ('\n'.join([uses_library_xml('foo'),
                                      uses_library_xml('bar')]))
    apk = self.apk_tmpl % ('\n'.join([uses_library_apk('foo'),
                                      uses_library_apk('bar')]))
    matches = self.run_test(xml, apk, uses_libraries=['bar', 'foo'])
    self.assertFalse(matches)

  def test_order_optional_uses_library(self):
    xml = self.xml_tmpl % ('\n'.join([uses_library_xml('foo', required_xml(False)),
                                      uses_library_xml('bar', required_xml(False))]))
    apk = self.apk_tmpl % ('\n'.join([uses_library_apk('foo', required_apk(False)),
                                      uses_library_apk('bar', required_apk(False))]))
    matches = self.run_test(xml, apk, optional_uses_libraries=['bar', 'foo'])
    self.assertFalse(matches)

  def test_duplicate_uses_library(self):
    xml = self.xml_tmpl % ('\n'.join([uses_library_xml('foo'),
                                      uses_library_xml('foo')]))
    apk = self.apk_tmpl % ('\n'.join([uses_library_apk('foo'),
                                      uses_library_apk('foo')]))
    matches = self.run_test(xml, apk, uses_libraries=['foo'])
    self.assertTrue(matches)

  def test_duplicate_optional_uses_library(self):
    xml = self.xml_tmpl % ('\n'.join([uses_library_xml('foo', required_xml(False)),
                                      uses_library_xml('foo', required_xml(False))]))
    apk = self.apk_tmpl % ('\n'.join([uses_library_apk('foo', required_apk(False)),
                                      uses_library_apk('foo', required_apk(False))]))
    matches = self.run_test(xml, apk, optional_uses_libraries=['foo'])
    self.assertTrue(matches)

  def test_mixed(self):
    xml = self.xml_tmpl % ('\n'.join([uses_library_xml('foo'),
                                      uses_library_xml('bar', required_xml(False))]))
    apk = self.apk_tmpl % ('\n'.join([uses_library_apk('foo'),
                                      uses_library_apk('bar', required_apk(False))]))
    matches = self.run_test(xml, apk, uses_libraries=['foo'],
                            optional_uses_libraries=['bar'])
    self.assertTrue(matches)


class ExtractTargetSdkVersionTest(unittest.TestCase):
  def run_test(self, xml, apk, version):
    doc = minidom.parseString(xml)
    v = manifest_check.extract_target_sdk_version(doc, is_apk=False)
    self.assertEqual(v, version)
    v = manifest_check.extract_target_sdk_version(apk, is_apk=True)
    self.assertEqual(v, version)

  xml_tmpl = (
      '<?xml version="1.0" encoding="utf-8"?>\n'
      '<manifest xmlns:android="http://schemas.android.com/apk/res/android">\n'
      '    <uses-sdk android:minSdkVersion="28" android:targetSdkVersion="%s" />\n'
      '</manifest>\n')

  apk_tmpl = (
      "package: name='com.google.android.something' versionCode='100'\n"
      "sdkVersion:'28'\n"
      "targetSdkVersion:'%s'\n"
      "uses-permission: name='android.permission.ACCESS_NETWORK_STATE'\n")

  def test_targert_sdk_version_28(self):
    xml = self.xml_tmpl % "28"
    apk = self.apk_tmpl % "28"
    self.run_test(xml, apk, "28")

  def test_targert_sdk_version_29(self):
    xml = self.xml_tmpl % "29"
    apk = self.apk_tmpl % "29"
    self.run_test(xml, apk, "29")

if __name__ == '__main__':
  unittest.main(verbosity=2)
