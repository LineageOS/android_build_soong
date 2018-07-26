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


def find_child_with_attribute(element, tag_name, namespace_uri,
                              attr_name, value):
  for child in get_children_with_tag(element, tag_name):
    attr = child.getAttributeNodeNS(namespace_uri, attr_name)
    if attr is not None and attr.value == value:
      return child
  return None


def parse_args():
  """Parse commandline arguments."""

  parser = argparse.ArgumentParser()
  parser.add_argument('--minSdkVersion', default='', dest='min_sdk_version',
                      help='specify minSdkVersion used by the build system')
  parser.add_argument('--library', dest='library', action='store_true',
                      help='manifest is for a static library')
  parser.add_argument('--uses-library', dest='uses_libraries', action='append',
                      help='specify additional <uses-library> tag to add')
  parser.add_argument('input', help='input AndroidManifest.xml file')
  parser.add_argument('output', help='output AndroidManifest.xml file')
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


def get_indent(element, default_level):
  indent = ''
  if element is not None and element.nodeType == minidom.Node.TEXT_NODE:
    text = element.nodeValue
    indent = text[:len(text)-len(text.lstrip())]
  if not indent or indent == '\n':
    # 1 indent = 4 space
    indent = '\n' + (' ' * default_level * 4)
  return indent


def raise_min_sdk_version(doc, requested, library):
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
    indent = get_indent(manifest.firstChild, 1)
    manifest.insertBefore(element, manifest.firstChild)

    # Insert an indent before uses-sdk to line it up with the indentation of the
    # other children of the <manifest> tag.
    manifest.insertBefore(doc.createTextNode(indent), manifest.firstChild)

  # Get or insert the minSdkVersion attribute.  If it is already present, make
  # sure it as least the requested value.
  min_attr = element.getAttributeNodeNS(android_ns, 'minSdkVersion')
  if min_attr is None:
    min_attr = doc.createAttributeNS(android_ns, 'android:minSdkVersion')
    min_attr.value = requested
    element.setAttributeNode(min_attr)
  else:
    if compare_version_gt(requested, min_attr.value):
      min_attr.value = requested

  # Insert the targetSdkVersion attribute if it is missing.  If it is already
  # present leave it as is.
  target_attr = element.getAttributeNodeNS(android_ns, 'targetSdkVersion')
  if target_attr is None:
    target_attr = doc.createAttributeNS(android_ns, 'android:targetSdkVersion')
    if library:
      target_attr.value = '1'
    else:
      target_attr.value = requested
    element.setAttributeNode(target_attr)


def add_uses_libraries(doc, new_uses_libraries):
  """Add additional <uses-library> tags with android:required=true.

  Args:
    doc: The XML document. May be modified by this function.
    new_uses_libraries: The names of libraries to be added by this function.
  Raises:
    RuntimeError: Invalid manifest
  """

  manifest = parse_manifest(doc)
  elems = get_children_with_tag(manifest, 'application')
  application = elems[0] if len(elems) == 1 else None
  if len(elems) > 1:
    raise RuntimeError('found multiple <application> tags')
  elif not elems:
    application = doc.createElement('application')
    indent = get_indent(manifest.firstChild, 1)
    first = manifest.firstChild
    manifest.insertBefore(doc.createTextNode(indent), first)
    manifest.insertBefore(application, first)

  indent = get_indent(application.firstChild, 2)

  last = application.lastChild
  if last is not None and last.nodeType != minidom.Node.TEXT_NODE:
    last = None

  for name in new_uses_libraries:
    if find_child_with_attribute(application, 'uses-library', android_ns,
                                 'name', name) is not None:
      # If the uses-library tag of the same 'name' attribute value exists,
      # respect it.
      continue

    ul = doc.createElement('uses-library')
    ul.setAttributeNS(android_ns, 'android:name', name)
    ul.setAttributeNS(android_ns, 'android:required', 'true')

    application.insertBefore(doc.createTextNode(indent), last)
    application.insertBefore(ul, last)

  # align the closing tag with the opening tag if it's not
  # indented
  if application.lastChild.nodeType != minidom.Node.TEXT_NODE:
    indent = get_indent(application.previousSibling, 1)
    application.appendChild(doc.createTextNode(indent))


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
      raise_min_sdk_version(doc, args.min_sdk_version, args.library)

    if args.uses_libraries:
      add_uses_libraries(doc, args.uses_libraries)

    with open(args.output, 'wb') as f:
      write_xml(f, doc)

  # pylint: disable=broad-except
  except Exception as err:
    print('error: ' + str(err), file=sys.stderr)
    sys.exit(-1)

if __name__ == '__main__':
  main()
