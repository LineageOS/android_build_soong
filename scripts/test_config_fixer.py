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
import sys
from xml.dom import minidom


from manifest import get_children_with_tag
from manifest import parse_manifest
from manifest import parse_test_config
from manifest import write_xml


def parse_args():
  """Parse commandline arguments."""

  parser = argparse.ArgumentParser()
  parser.add_argument('--manifest', default='', dest='manifest',
                      help=('AndroidManifest.xml that contains the original package name'))
  parser.add_argument('--package-name', default='', dest='package_name',
                      help=('overwrite package fields in the test config'))
  parser.add_argument('--test-file-name', default='', dest='test_file_name',
                      help=('overwrite test file name in the test config'))
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
    if test.getAttribute('class') == "com.android.tradefed.targetprep.TestAppInstallSetup":
      options = get_children_with_tag(test, 'option')
      for option in options:
        if option.getAttribute('name') == "test-file-name":
          option.setAttribute('value', test_file_name)

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
      overwrite_test_file_name(doc, args.test_file_name)

    with open(args.output, 'wb') as f:
      write_xml(f, doc)

  # pylint: disable=broad-except
  except Exception as err:
    print('error: ' + str(err), file=sys.stderr)
    sys.exit(-1)

if __name__ == '__main__':
  main()
