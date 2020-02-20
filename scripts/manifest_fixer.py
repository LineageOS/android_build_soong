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


from manifest import android_ns
from manifest import compare_version_gt
from manifest import ensure_manifest_android_ns
from manifest import find_child_with_attribute
from manifest import get_children_with_tag
from manifest import get_indent
from manifest import parse_manifest
from manifest import write_xml


def parse_args():
  """Parse commandline arguments."""

  parser = argparse.ArgumentParser()
  parser.add_argument('--minSdkVersion', default='', dest='min_sdk_version',
                      help='specify minSdkVersion used by the build system')
  parser.add_argument('--targetSdkVersion', default='', dest='target_sdk_version',
                      help='specify targetSdkVersion used by the build system')
  parser.add_argument('--raise-min-sdk-version', dest='raise_min_sdk_version', action='store_true',
                      help='raise the minimum sdk version in the manifest if necessary')
  parser.add_argument('--library', dest='library', action='store_true',
                      help='manifest is for a static library')
  parser.add_argument('--uses-library', dest='uses_libraries', action='append',
                      help='specify additional <uses-library> tag to add. android:requred is set to true')
  parser.add_argument('--optional-uses-library', dest='optional_uses_libraries', action='append',
                      help='specify additional <uses-library> tag to add. android:requred is set to false')
  parser.add_argument('--uses-non-sdk-api', dest='uses_non_sdk_api', action='store_true',
                      help='manifest is for a package built against the platform')
  parser.add_argument('--logging-parent', dest='logging_parent', default='',
                      help=('specify logging parent as an additional <meta-data> tag. '
                            'This value is ignored if the logging_parent meta-data tag is present.'))
  parser.add_argument('--use-embedded-dex', dest='use_embedded_dex', action='store_true',
                      help=('specify if the app wants to use embedded dex and avoid extracted,'
                            'locally compiled code. Must not conflict if already declared '
                            'in the manifest.'))
  parser.add_argument('--extract-native-libs', dest='extract_native_libs',
                      default=None, type=lambda x: (str(x).lower() == 'true'),
                      help=('specify if the app wants to use embedded native libraries. Must not conflict '
                            'if already declared in the manifest.'))
  parser.add_argument('--has-no-code', dest='has_no_code', action='store_true',
                      help=('adds hasCode="false" attribute to application. Ignored if application elem '
                            'already has a hasCode attribute.'))
  parser.add_argument('input', help='input AndroidManifest.xml file')
  parser.add_argument('output', help='output AndroidManifest.xml file')
  return parser.parse_args()


def raise_min_sdk_version(doc, min_sdk_version, target_sdk_version, library):
  """Ensure the manifest contains a <uses-sdk> tag with a minSdkVersion.

  Args:
    doc: The XML document.  May be modified by this function.
    min_sdk_version: The requested minSdkVersion attribute.
    target_sdk_version: The requested targetSdkVersion attribute.
    library: True if the manifest is for a library.
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
    min_attr.value = min_sdk_version
    element.setAttributeNode(min_attr)
  else:
    if compare_version_gt(min_sdk_version, min_attr.value):
      min_attr.value = min_sdk_version

  # Insert the targetSdkVersion attribute if it is missing.  If it is already
  # present leave it as is.
  target_attr = element.getAttributeNodeNS(android_ns, 'targetSdkVersion')
  if target_attr is None:
    target_attr = doc.createAttributeNS(android_ns, 'android:targetSdkVersion')
    if library:
      # TODO(b/117122200): libraries shouldn't set targetSdkVersion at all, but
      # ManifestMerger treats minSdkVersion="Q" as targetSdkVersion="Q" if it
      # is empty.  Set it to something low so that it will be overriden by the
      # main manifest, but high enough that it doesn't cause implicit
      # permissions grants.
      target_attr.value = '15'
    else:
      target_attr.value = target_sdk_version
    element.setAttributeNode(target_attr)


def add_logging_parent(doc, logging_parent_value):
  """Add logging parent as an additional <meta-data> tag.

  Args:
    doc: The XML document. May be modified by this function.
    logging_parent_value: A string representing the logging
      parent value.
  Raises:
    RuntimeError: Invalid manifest
  """
  manifest = parse_manifest(doc)

  logging_parent_key = 'android.content.pm.LOGGING_PARENT'
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

  if not find_child_with_attribute(application, 'meta-data', android_ns,
                                   'name', logging_parent_key):
    ul = doc.createElement('meta-data')
    ul.setAttributeNS(android_ns, 'android:name', logging_parent_key)
    ul.setAttributeNS(android_ns, 'android:value', logging_parent_value)
    application.insertBefore(doc.createTextNode(indent), last)
    application.insertBefore(ul, last)
    last = application.lastChild

  # align the closing tag with the opening tag if it's not
  # indented
  if last and last.nodeType != minidom.Node.TEXT_NODE:
    indent = get_indent(application.previousSibling, 1)
    application.appendChild(doc.createTextNode(indent))


def add_uses_libraries(doc, new_uses_libraries, required):
  """Add additional <uses-library> tags

  Args:
    doc: The XML document. May be modified by this function.
    new_uses_libraries: The names of libraries to be added by this function.
    required: The value of android:required attribute. Can be true or false.
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
    ul.setAttributeNS(android_ns, 'android:required', str(required).lower())

    application.insertBefore(doc.createTextNode(indent), last)
    application.insertBefore(ul, last)

  # align the closing tag with the opening tag if it's not
  # indented
  if application.lastChild.nodeType != minidom.Node.TEXT_NODE:
    indent = get_indent(application.previousSibling, 1)
    application.appendChild(doc.createTextNode(indent))


def add_uses_non_sdk_api(doc):
  """Add android:usesNonSdkApi=true attribute to <application>.

  Args:
    doc: The XML document. May be modified by this function.
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

  attr = application.getAttributeNodeNS(android_ns, 'usesNonSdkApi')
  if attr is None:
    attr = doc.createAttributeNS(android_ns, 'android:usesNonSdkApi')
    attr.value = 'true'
    application.setAttributeNode(attr)


def add_use_embedded_dex(doc):
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

  attr = application.getAttributeNodeNS(android_ns, 'useEmbeddedDex')
  if attr is None:
    attr = doc.createAttributeNS(android_ns, 'android:useEmbeddedDex')
    attr.value = 'true'
    application.setAttributeNode(attr)
  elif attr.value != 'true':
    raise RuntimeError('existing attribute mismatches the option of --use-embedded-dex')


def add_extract_native_libs(doc, extract_native_libs):
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

  value = str(extract_native_libs).lower()
  attr = application.getAttributeNodeNS(android_ns, 'extractNativeLibs')
  if attr is None:
    attr = doc.createAttributeNS(android_ns, 'android:extractNativeLibs')
    attr.value = value
    application.setAttributeNode(attr)
  elif attr.value != value:
    raise RuntimeError('existing attribute extractNativeLibs="%s" conflicts with --extract-native-libs="%s"' %
                       (attr.value, value))


def set_has_code_to_false(doc):
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

  attr = application.getAttributeNodeNS(android_ns, 'hasCode')
  if attr is not None:
    # Do nothing if the application already has a hasCode attribute.
    return
  attr = doc.createAttributeNS(android_ns, 'android:hasCode')
  attr.value = 'false'
  application.setAttributeNode(attr)


def main():
  """Program entry point."""
  try:
    args = parse_args()

    doc = minidom.parse(args.input)

    ensure_manifest_android_ns(doc)

    if args.raise_min_sdk_version:
      raise_min_sdk_version(doc, args.min_sdk_version, args.target_sdk_version, args.library)

    if args.uses_libraries:
      add_uses_libraries(doc, args.uses_libraries, True)

    if args.optional_uses_libraries:
      add_uses_libraries(doc, args.optional_uses_libraries, False)

    if args.uses_non_sdk_api:
      add_uses_non_sdk_api(doc)

    if args.logging_parent:
      add_logging_parent(doc, args.logging_parent)

    if args.use_embedded_dex:
      add_use_embedded_dex(doc)

    if args.has_no_code:
      set_has_code_to_false(doc)

    if args.extract_native_libs is not None:
      add_extract_native_libs(doc, args.extract_native_libs)

    with open(args.output, 'wb') as f:
      write_xml(f, doc)

  # pylint: disable=broad-except
  except Exception as err:
    print('error: ' + str(err), file=sys.stderr)
    sys.exit(-1)

if __name__ == '__main__':
  main()
