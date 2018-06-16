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
"""A tool for inserting values from the build system into a manifest."""

from __future__ import print_function
import argparse
import sys
from xml.dom import minidom


android_ns = 'http://schemas.android.com/apk/res/android'


def get_children_with_tag(parent, tag_name):
  children = []
  for child in  parent.childNodes:
    if child.nodeType == minidom.Node.ELEMENT_NODE and \
       child.tagName == tag_name:
      children.append(child)
  return children


def parse_args():
  """Parse commandline arguments."""

  parser = argparse.ArgumentParser()
  parser.add_argument('--minSdkVersion', default='', dest='min_sdk_version',
                      help='specify minSdkVersion used by the build system')
  parser.add_argument('input', help='input AndroidManifest.xml file')
  parser.add_argument('output', help='input AndroidManifest.xml file')
  return parser.parse_args()


def parse_manifest(doc):
  """Get the manifest element."""

  manifest = doc.documentElement
  if manifest.tagName != 'manifest':
    raise RuntimeError('expected manifest tag at root')
  return manifest


def ensure_manifest_android_ns(doc):
  """Make sure the manifest tag defines the android namespace."""

  manifest = parse_manifest(doc)

  ns = manifest.getAttributeNodeNS(minidom.XMLNS_NAMESPACE, 'android')
  if ns is None:
    attr = doc.createAttributeNS(minidom.XMLNS_NAMESPACE, 'xmlns:android')
    attr.value = android_ns
    manifest.setAttributeNode(attr)
  elif ns.value != android_ns:
    raise RuntimeError('manifest tag has incorrect android namespace ' +
                       ns.value)


def as_int(s):
  try:
    i = int(s)
  except ValueError:
    return s, False
  return i, True


def compare_version_gt(a, b):
  """Compare two SDK versions.

  Compares a and b, treating codenames like 'Q' as higher
  than numerical versions like '28'.

  Returns True if a > b

  Args:
    a: value to compare
    b: value to compare
  Returns:
    True if a is a higher version than b
  """

  a, a_is_int = as_int(a.upper())
  b, b_is_int = as_int(b.upper())

  if a_is_int == b_is_int:
    # Both are codenames or both are versions, compare directly
    return a > b
  else:
    # One is a codename, the other is not.  Return true if
    # b is an integer version
    return b_is_int


def raise_min_sdk_version(doc, requested):
  """Ensure the manifest contains a <uses-sdk> tag with a minSdkVersion.

  Args:
    doc: The XML document.  May be modified by this function.
    requested: The requested minSdkVersion attribute.
  Raises:
    RuntimeError: invalid manifest
  """

  manifest = parse_manifest(doc)

  # Get or insert the uses-sdk element
  uses_sdk = get_children_with_tag(manifest, 'uses-sdk')
  if len(uses_sdk) > 1:
    raise RuntimeError('found multiple uses-sdk elements')
  elif len(uses_sdk) == 1:
    element = uses_sdk[0]
  else:
    element = doc.createElement('uses-sdk')
    indent = ''
    first = manifest.firstChild
    if first is not None and first.nodeType == minidom.Node.TEXT_NODE:
      text = first.nodeValue
      indent = text[:len(text)-len(text.lstrip())]
    if not indent or indent == '\n':
      indent = '\n    '

    manifest.insertBefore(element, manifest.firstChild)

    # Insert an indent before uses-sdk to line it up with the indentation of the
    # other children of the <manifest> tag.
    manifest.insertBefore(doc.createTextNode(indent), manifest.firstChild)

  # Get or insert the minSdkVersion attribute
  min_attr = element.getAttributeNodeNS(android_ns, 'minSdkVersion')
  if min_attr is None:
    min_attr = doc.createAttributeNS(android_ns, 'android:minSdkVersion')
    min_attr.value = '1'
    element.setAttributeNode(min_attr)

  # Update the value of the minSdkVersion attribute if necessary
  if compare_version_gt(requested, min_attr.value):
    min_attr.value = requested


def write_xml(f, doc):
  f.write('<?xml version="1.0" encoding="utf-8"?>\n')
  for node in doc.childNodes:
    f.write(node.toxml(encoding='utf-8') + '\n')


def main():
  """Program entry point."""
  try:
    args = parse_args()

    doc = minidom.parse(args.input)

    ensure_manifest_android_ns(doc)

    if args.min_sdk_version:
      raise_min_sdk_version(doc, args.min_sdk_version)

    with open(args.output, 'wb') as f:
      write_xml(f, doc)

  # pylint: disable=broad-except
  except Exception as err:
    print('error: ' + str(err), file=sys.stderr)
    sys.exit(-1)

if __name__ == '__main__':
  main()
