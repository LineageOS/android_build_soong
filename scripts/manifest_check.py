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
"""A tool for checking that a manifest agrees with the build system."""

from __future__ import print_function

import argparse
import sys
from xml.dom import minidom


from manifest import android_ns
from manifest import get_children_with_tag
from manifest import parse_manifest
from manifest import write_xml


class ManifestMismatchError(Exception):
  pass


def parse_args():
  """Parse commandline arguments."""

  parser = argparse.ArgumentParser()
  parser.add_argument('--uses-library', dest='uses_libraries',
                      action='append',
                      help='specify uses-library entries known to the build system')
  parser.add_argument('--optional-uses-library',
                      dest='optional_uses_libraries',
                      action='append',
                      help='specify uses-library entries known to the build system with required:false')
  parser.add_argument('--enforce-uses-libraries',
                      dest='enforce_uses_libraries',
                      action='store_true',
                      help='check the uses-library entries known to the build system against the manifest')
  parser.add_argument('--extract-target-sdk-version',
                      dest='extract_target_sdk_version',
                      action='store_true',
                      help='print the targetSdkVersion from the manifest')
  parser.add_argument('--output', '-o', dest='output', help='output AndroidManifest.xml file')
  parser.add_argument('input', help='input AndroidManifest.xml file')
  return parser.parse_args()


def enforce_uses_libraries(doc, uses_libraries, optional_uses_libraries):
  """Verify that the <uses-library> tags in the manifest match those provided by the build system.

  Args:
    doc: The XML document.
    uses_libraries: The names of <uses-library> tags known to the build system
    optional_uses_libraries: The names of <uses-library> tags with required:fals
      known to the build system
  Raises:
    RuntimeError: Invalid manifest
    ManifestMismatchError: Manifest does not match
  """

  manifest = parse_manifest(doc)
  elems = get_children_with_tag(manifest, 'application')
  application = elems[0] if len(elems) == 1 else None
  if len(elems) > 1:
    raise RuntimeError('found multiple <application> tags')
  elif not elems:
    if uses_libraries or optional_uses_libraries:
      raise ManifestMismatchError('no <application> tag found')
    return

  verify_uses_library(application, uses_libraries, optional_uses_libraries)


def verify_uses_library(application, uses_libraries, optional_uses_libraries):
  """Verify that the uses-library values known to the build system match the manifest.

  Args:
    application: the <application> tag in the manifest.
    uses_libraries: the names of expected <uses-library> tags.
    optional_uses_libraries: the names of expected <uses-library> tags with required="false".
  Raises:
    ManifestMismatchError: Manifest does not match
  """

  if uses_libraries is None:
    uses_libraries = []

  if optional_uses_libraries is None:
    optional_uses_libraries = []

  manifest_uses_libraries, manifest_optional_uses_libraries = parse_uses_library(application)

  err = []
  if manifest_uses_libraries != uses_libraries:
    err.append('Expected required <uses-library> tags "%s", got "%s"' %
               (', '.join(uses_libraries), ', '.join(manifest_uses_libraries)))

  if manifest_optional_uses_libraries != optional_uses_libraries:
    err.append('Expected optional <uses-library> tags "%s", got "%s"' %
               (', '.join(optional_uses_libraries), ', '.join(manifest_optional_uses_libraries)))

  if err:
    raise ManifestMismatchError('\n'.join(err))


def parse_uses_library(application):
  """Extract uses-library tags from the manifest.

  Args:
    application: the <application> tag in the manifest.
  """

  libs = get_children_with_tag(application, 'uses-library')

  uses_libraries = [uses_library_name(x) for x in libs if uses_library_required(x)]
  optional_uses_libraries = [uses_library_name(x) for x in libs if not uses_library_required(x)]

  return first_unique_elements(uses_libraries), first_unique_elements(optional_uses_libraries)


def first_unique_elements(l):
  result = []
  [result.append(x) for x in l if x not in result]
  return result


def uses_library_name(lib):
  """Extract the name attribute of a uses-library tag.

  Args:
    lib: a <uses-library> tag.
  """
  name = lib.getAttributeNodeNS(android_ns, 'name')
  return name.value if name is not None else ""


def uses_library_required(lib):
  """Extract the required attribute of a uses-library tag.

  Args:
    lib: a <uses-library> tag.
  """
  required = lib.getAttributeNodeNS(android_ns, 'required')
  return (required.value == 'true') if required is not None else True


def extract_target_sdk_version(doc):
  """Returns the targetSdkVersion from the manifest.

  Args:
    doc: The XML document.
  Raises:
    RuntimeError: invalid manifest
  """

  manifest = parse_manifest(doc)

  # Get or insert the uses-sdk element
  uses_sdk = get_children_with_tag(manifest, 'uses-sdk')
  if len(uses_sdk) > 1:
    raise RuntimeError('found multiple uses-sdk elements')
  elif len(uses_sdk) == 0:
    raise RuntimeError('missing uses-sdk element')

  uses_sdk = uses_sdk[0]

  min_attr = uses_sdk.getAttributeNodeNS(android_ns, 'minSdkVersion')
  if min_attr is None:
    raise RuntimeError('minSdkVersion is not specified')

  target_attr = uses_sdk.getAttributeNodeNS(android_ns, 'targetSdkVersion')
  if target_attr is None:
    target_attr = min_attr

  return target_attr.value


def main():
  """Program entry point."""
  try:
    args = parse_args()

    doc = minidom.parse(args.input)

    if args.enforce_uses_libraries:
      enforce_uses_libraries(doc,
                             args.uses_libraries,
                             args.optional_uses_libraries)

    if args.extract_target_sdk_version:
      print(extract_target_sdk_version(doc))

    if args.output:
      with open(args.output, 'wb') as f:
        write_xml(f, doc)

  # pylint: disable=broad-except
  except Exception as err:
    print('error: ' + str(err), file=sys.stderr)
    sys.exit(-1)

if __name__ == '__main__':
  main()
