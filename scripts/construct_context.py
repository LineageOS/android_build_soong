#!/usr/bin/env python
#
# Copyright (C) 2020 The Android Open Source Project
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
"""A tool for constructing class loader context."""

from __future__ import print_function

import argparse
import sys

from manifest import compare_version_gt


def parse_args(args):
  """Parse commandline arguments."""
  parser = argparse.ArgumentParser()
  parser.add_argument('--target-sdk-version', default='', dest='sdk',
    help='specify target SDK version (as it appears in the manifest)')
  parser.add_argument('--host-classpath-for-sdk', dest='host_classpaths',
    action='append', nargs=2, metavar=('sdk','classpath'),
    help='specify classpath on host for a given SDK version or "any" version')
  parser.add_argument('--target-classpath-for-sdk', dest='target_classpaths',
    action='append', nargs=2, metavar=('sdk','classpath'),
    help='specify classpath on target for a given SDK version or "any" version')
  return parser.parse_args(args)

# The hidl.manager shared library has a dependency on hidl.base. We manually
# add that information to the class loader context if we see those libraries.
HIDL_MANAGER = 'android.hidl.manager-V1.0-java'
HIDL_BASE    = 'android.hidl.base-V1.0-java'

# Special keyword that means that the classpath should be added to class loader
# context regardless of the target SDK version.
any_sdk = 'any'

# We assume that the order of classpath arguments passed to this script is
# correct (matches the order computed by package manager). It is possible to
# sort them here, but Soong needs to use deterministic order anyway, so it can
# as well use the correct order.
def construct_context(versioned_classpaths, target_sdk):
  context = []
  for [sdk, classpath] in versioned_classpaths:
    if sdk == any_sdk or compare_version_gt(sdk, target_sdk):
      for jar in classpath.split(':'):
        pcl = 'PCL[%s]' % jar
        if HIDL_MANAGER in jar:
          pcl += '{PCL[%s]}' % jar.replace(HIDL_MANAGER, HIDL_BASE, 1)
        context.append(pcl)
  return context

def construct_contexts(args):
  host_context = construct_context(args.host_classpaths, args.sdk)
  target_context = construct_context(args.target_classpaths, args.sdk)
  context_sep = '#'
  return ('class_loader_context_arg=--class-loader-context=PCL[]{%s} ; ' % context_sep.join(host_context) +
    'stored_class_loader_context_arg=--stored-class-loader-context=PCL[]{%s}' % context_sep.join(target_context))

def main():
  """Program entry point."""
  try:
    args = parse_args(sys.argv[1:])
    if not args.sdk:
      raise SystemExit('target sdk version is not set')
    if not args.host_classpaths:
      raise SystemExit('host classpath is not set')
    if not args.target_classpaths:
      raise SystemExit('target classpath is not set')

    print(construct_contexts(args))

  # pylint: disable=broad-except
  except Exception as err:
    print('error: ' + str(err), file=sys.stderr)
    sys.exit(-1)

if __name__ == '__main__':
  main()
