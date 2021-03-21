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
import json
import re
import subprocess
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
  parser.add_argument('--enforce-uses-libraries-relax',
                      dest='enforce_uses_libraries_relax',
                      action='store_true',
                      help='do not fail immediately, just save the error message to file')
  parser.add_argument('--enforce-uses-libraries-status',
                      dest='enforce_uses_libraries_status',
                      help='output file to store check status (error message)')
  parser.add_argument('--extract-target-sdk-version',
                      dest='extract_target_sdk_version',
                      action='store_true',
                      help='print the targetSdkVersion from the manifest')
  parser.add_argument('--dexpreopt-config',
                      dest='dexpreopt_configs',
                      action='append',
                      help='a paths to a dexpreopt.config of some library')
  parser.add_argument('--aapt',
                      dest='aapt',
                      help='path to aapt executable')
  parser.add_argument('--output', '-o', dest='output', help='output AndroidManifest.xml file')
  parser.add_argument('input', help='input AndroidManifest.xml file')
  return parser.parse_args()


def enforce_uses_libraries(manifest, required, optional, relax, is_apk = False):
  """Verify that the <uses-library> tags in the manifest match those provided
  by the build system.

  Args:
    manifest: manifest (either parsed XML or aapt dump of APK)
    required: required libs known to the build system
    optional: optional libs known to the build system
    relax:    if true, suppress error on mismatch and just write it to file
    is_apk:   if the manifest comes from an APK or an XML file
  """
  if is_apk:
    manifest_required, manifest_optional = extract_uses_libs_apk(manifest)
  else:
    manifest_required, manifest_optional = extract_uses_libs_xml(manifest)

  err = []
  if manifest_required != required:
    err.append('Expected required <uses-library> tags "%s", got "%s"' %
               (', '.join(required), ', '.join(manifest_required)))

  if manifest_optional != optional:
    err.append('Expected optional <uses-library> tags "%s", got "%s"' %
               (', '.join(optional), ', '.join(manifest_optional)))

  if err:
    errmsg = '\n'.join(err)
    if not relax:
      raise ManifestMismatchError(errmsg)
    return errmsg

  return None


def extract_uses_libs_apk(badging):
  """Extract <uses-library> tags from the manifest of an APK."""

  pattern = re.compile("^uses-library(-not-required)?:'(.*)'$", re.MULTILINE)

  required = []
  optional = []
  for match in re.finditer(pattern, badging):
    libname = match.group(2)
    if match.group(1) == None:
      required.append(libname)
    else:
      optional.append(libname)

  return first_unique_elements(required), first_unique_elements(optional)


def extract_uses_libs_xml(xml):
  """Extract <uses-library> tags from the manifest."""

  manifest = parse_manifest(xml)
  elems = get_children_with_tag(manifest, 'application')
  application = elems[0] if len(elems) == 1 else None
  if len(elems) > 1:
    raise RuntimeError('found multiple <application> tags')
  elif not elems:
    if uses_libraries or optional_uses_libraries:
      raise ManifestMismatchError('no <application> tag found')
    return

  libs = get_children_with_tag(application, 'uses-library')

  required = [uses_library_name(x) for x in libs if uses_library_required(x)]
  optional = [uses_library_name(x) for x in libs if not uses_library_required(x)]

  return first_unique_elements(required), first_unique_elements(optional)


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


def extract_target_sdk_version(manifest, is_apk = False):
  """Returns the targetSdkVersion from the manifest.

  Args:
    manifest: manifest (either parsed XML or aapt dump of APK)
    is_apk:   if the manifest comes from an APK or an XML file
  """
  if is_apk:
    return extract_target_sdk_version_apk(manifest)
  else:
    return extract_target_sdk_version_xml(manifest)


def extract_target_sdk_version_apk(badging):
  """Extract targetSdkVersion tags from the manifest of an APK."""

  pattern = re.compile("^targetSdkVersion?:'(.*)'$", re.MULTILINE)

  for match in re.finditer(pattern, badging):
    return match.group(1)

  raise RuntimeError('cannot find targetSdkVersion in the manifest')


def extract_target_sdk_version_xml(xml):
  """Extract targetSdkVersion tags from the manifest."""

  manifest = parse_manifest(xml)

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


def load_dexpreopt_configs(configs):
  """Load dexpreopt.config files and map module names to library names."""
  module_to_libname = {}

  if configs is None:
    configs = []

  for config in configs:
    with open(config, 'r') as f:
      contents = json.load(f)
    module_to_libname[contents['Name']] = contents['ProvidesUsesLibrary']

  return module_to_libname


def translate_libnames(modules, module_to_libname):
  """Translate module names into library names using the mapping."""
  if modules is None:
    modules = []

  libnames = []
  for name in modules:
    if name in module_to_libname:
      name = module_to_libname[name]
    libnames.append(name)

  return libnames


def main():
  """Program entry point."""
  try:
    args = parse_args()

    # The input can be either an XML manifest or an APK, they are parsed and
    # processed in different ways.
    is_apk = args.input.endswith('.apk')
    if is_apk:
      aapt = args.aapt if args.aapt != None else "aapt"
      manifest = subprocess.check_output([aapt, "dump", "badging", args.input])
    else:
      manifest = minidom.parse(args.input)

    if args.enforce_uses_libraries:
      # Load dexpreopt.config files and build a mapping from module names to
      # library names. This is necessary because build system addresses
      # libraries by their module name (`uses_libs`, `optional_uses_libs`,
      # `LOCAL_USES_LIBRARIES`, `LOCAL_OPTIONAL_LIBRARY_NAMES` all contain
      # module names), while the manifest addresses libraries by their name.
      mod_to_lib = load_dexpreopt_configs(args.dexpreopt_configs)
      required = translate_libnames(args.uses_libraries, mod_to_lib)
      optional = translate_libnames(args.optional_uses_libraries, mod_to_lib)

      # Check if the <uses-library> lists in the build system agree with those
      # in the manifest. Raise an exception on mismatch, unless the script was
      # passed a special parameter to suppress exceptions.
      errmsg = enforce_uses_libraries(manifest, required, optional,
        args.enforce_uses_libraries_relax, is_apk)

      # Create a status file that is empty on success, or contains an error
      # message on failure. When exceptions are suppressed, dexpreopt command
      # command will check file size to determine if the check has failed.
      if args.enforce_uses_libraries_status:
        with open(args.enforce_uses_libraries_status, 'w') as f:
          if not errmsg == None:
            f.write("%s\n" % errmsg)

    if args.extract_target_sdk_version:
      print(extract_target_sdk_version(manifest, is_apk))

    if args.output:
      # XML output is supposed to be written only when this script is invoked
      # with XML input manifest, not with an APK.
      if is_apk:
        raise RuntimeError('cannot save APK manifest as XML')

      with open(args.output, 'wb') as f:
        write_xml(f, manifest)

  # pylint: disable=broad-except
  except Exception as err:
    print('error: ' + str(err), file=sys.stderr)
    sys.exit(-1)

if __name__ == '__main__':
  main()
