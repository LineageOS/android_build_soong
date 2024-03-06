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
"""A tool for modifying values in a test config."""

from __future__ import print_function

import argparse
import json
import sys
from xml.dom import minidom


from manifest import get_children_with_tag
from manifest import parse_manifest
from manifest import parse_test_config
from manifest import write_xml

KNOWN_PREPARERS = ['com.android.tradefed.targetprep.TestAppInstallSetup',
                   'com.android.tradefed.targetprep.suite.SuiteApkInstaller']

KNOWN_TEST_RUNNERS = ['com.android.tradefed.testtype.AndroidJUnitTest']

MAINLINE_CONTROLLER = 'com.android.tradefed.testtype.suite.module.MainlineTestModuleController'

def parse_args():
  """Parse commandline arguments."""

  parser = argparse.ArgumentParser()
  parser.add_argument('--manifest', default='', dest='manifest',
                      help=('AndroidManifest.xml that contains the original package name'))
  parser.add_argument('--package-name', default='', dest='package_name',
                      help=('overwrite package fields in the test config'))
  parser.add_argument('--test-file-name', default='', dest='test_file_name',
                      help=('overwrite test file name in the test config'))
  parser.add_argument('--orig-test-file-name', default='', dest='orig_test_file_name',
                      help=('Use with test-file-name to only override a single apk'))
  parser.add_argument('--mainline-package-name', default='', dest='mainline_package_name',
                      help=('overwrite mainline module package name in the test config'))
  parser.add_argument('--test-runner-options', default='', dest='test_runner_options',
                      help=('Add test runner options in the test config'))
  parser.add_argument('input', help='input test config file')
  parser.add_argument('output', help='output test config file')
  return parser.parse_args()


def overwrite_package_name(test_config_doc, manifest_doc, package_name):

  manifest = parse_manifest(manifest_doc)
  original_package = manifest.getAttribute('package')

  test_config = parse_test_config(test_config_doc)
  tests = get_children_with_tag(test_config, 'test')

  for test in tests:
    options = get_children_with_tag(test, 'option')
    for option in options:
      if option.getAttribute('name') == "package" and option.getAttribute('value') == original_package:
        option.setAttribute('value', package_name)

def overwrite_test_file_name(test_config_doc, test_file_name):

  test_config = parse_test_config(test_config_doc)
  tests = get_children_with_tag(test_config, 'target_preparer')

  for test in tests:
    if test.getAttribute('class') in KNOWN_PREPARERS:
      options = get_children_with_tag(test, 'option')
      for option in options:
        if option.getAttribute('name') == "test-file-name":
          option.setAttribute('value', test_file_name)

def overwrite_single_test_file_name(test_config_doc, orig_test_file_name, new_test_file_name):

  test_config = parse_test_config(test_config_doc)
  tests = get_children_with_tag(test_config, 'target_preparer')

  for test in tests:
    if test.getAttribute('class') in KNOWN_PREPARERS:
      options = get_children_with_tag(test, 'option')
      for option in options:
        if option.getAttribute('name') == "test-file-name" and option.getAttribute('value') == orig_test_file_name:
          option.setAttribute('value', new_test_file_name)

def overwrite_mainline_module_package_name(test_config_doc, mainline_package_name):

  test_config = parse_test_config(test_config_doc)

  for obj in get_children_with_tag(test_config, 'object'):
    if obj.getAttribute('class') == MAINLINE_CONTROLLER:
      for option in get_children_with_tag(obj, 'option'):
        if option.getAttribute('name') == "mainline-module-package-name":
          option.setAttribute('value', mainline_package_name)

def add_test_runner_options_toplevel(test_config_doc, test_runner_options):

  test_config = parse_test_config(test_config_doc)

  test_config.appendChild(test_config_doc.createComment("Options from Android.bp"))
  test_config.appendChild(test_config_doc.createTextNode("\n"))
  for new_option in json.loads(test_runner_options):
    option = test_config_doc.createElement("option")
    # name and value are mandatory,
    name = new_option.get('Name')
    if not name:
      raise RuntimeError('"name" must set in test_runner_option"')
    value = new_option.get('Value')
    if not value:
      raise RuntimeError('"value" must set in test_runner_option"')
    option.setAttribute('name', name) # 'include-filter')
    option.setAttribute('value', value) # 'android.test.example.devcodelab.DevCodelabTest#testHelloFail')
    key = new_option.get('Key')
    if key:
      option.setAttribute('key', key) # 'include-filter')
    # add tab and newline for readability
    test_config.appendChild(test_config_doc.createTextNode("    "))
    test_config.appendChild(option)
    test_config.appendChild(test_config_doc.createTextNode("\n"))

def main():
  """Program entry point."""
  try:
    args = parse_args()

    doc = minidom.parse(args.input)

    if args.package_name:
      if not args.manifest:
        raise RuntimeError('--manifest flag required for --package-name')
      manifest_doc = minidom.parse(args.manifest)
      overwrite_package_name(doc, manifest_doc, args.package_name)

    if args.test_file_name:
      if args.orig_test_file_name:
        overwrite_single_test_file_name(doc, args.orig_test_file_name, args.test_file_name)
      else:
        # You probably never want to override the test_file_name if there
        # are several in the xml, but this is currently only used on generated
        # AndroidTest.xml where there is only a single test-file-name (no data)
        overwrite_test_file_name(doc, args.test_file_name)

    if args.mainline_package_name:
      overwrite_mainline_module_package_name(doc, args.mainline_package_name)

    if args.test_runner_options:
      add_test_runner_options_toplevel(doc, args.test_runner_options)

    with open(args.output, 'w') as f:
      write_xml(f, doc)

  # pylint: disable=broad-except
  except Exception as err:
    print('error: ' + str(err), file=sys.stderr)
    sys.exit(-1)

if __name__ == '__main__':
  main()
