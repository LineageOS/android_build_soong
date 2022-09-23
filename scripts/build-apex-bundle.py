#!/usr/bin/env python
#
# Copyright (C) 2022 The Android Open Source Project
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
"""A tool to create an APEX bundle out of Soong-built base.zip"""

from __future__ import print_function

import argparse
import sys
import tempfile
import zipfile
import os
import json
import subprocess


def parse_args():
  """Parse commandline arguments."""
  parser = argparse.ArgumentParser()
  parser.add_argument(
      '--overwrite',
      action='store_true',
      help='If set, any previous existing output will be overwritten')
  parser.add_argument('--output', help='specify the output .aab file')
  parser.add_argument(
      'input', help='specify the input <apex name>-base.zip file')
  return parser.parse_args()


def build_bundle(input, output, overwrite):
  base_zip = zipfile.ZipFile(input)

  tmpdir = tempfile.mkdtemp()
  tmp_base_zip = os.path.join(tmpdir, 'base.zip')
  tmp_bundle_config = os.path.join(tmpdir, 'bundle_config.json')

  bundle_config = None
  abi = []

  # This block performs three tasks
  # - extract/load bundle_config.json from input => bundle_config
  # - get ABI from input => abi
  # - discard bundle_config.json from input => tmp/base.zip
  with zipfile.ZipFile(tmp_base_zip, 'a') as out:
    for info in base_zip.infolist():

      # discard bundle_config.json
      if info.filename == 'bundle_config.json':
        bundle_config = json.load(base_zip.open(info.filename))
        continue

      # get ABI from apex/{abi}.img
      dir, basename = os.path.split(info.filename)
      name, ext = os.path.splitext(basename)
      if dir == 'apex' and ext == '.img':
        abi.append(name)

      # copy entries to tmp/base.zip
      out.writestr(info, base_zip.open(info.filename).read())

  base_zip.close()

  if not bundle_config:
    raise ValueError(f'bundle_config.json not found in {input}')
  if len(abi) != 1:
    raise ValueError(f'{input} should have only a single apex/*.img file')

  # add ABI to tmp/bundle_config.json
  apex_config = bundle_config['apex_config']
  if 'supported_abi_set' not in apex_config:
    apex_config['supported_abi_set'] = []
  supported_abi_set = apex_config['supported_abi_set']
  supported_abi_set.append({'abi': abi})

  with open(tmp_bundle_config, 'w') as out:
    json.dump(bundle_config, out)

  # invoke bundletool
  cmd = [
      'bundletool', 'build-bundle', '--config', tmp_bundle_config, '--modules',
      tmp_base_zip, '--output', output
  ]
  if overwrite:
    cmd.append('--overwrite')
  subprocess.check_call(cmd)


def main():
  """Program entry point."""
  try:
    args = parse_args()
    build_bundle(args.input, args.output, args.overwrite)

  # pylint: disable=broad-except
  except Exception as err:
    print('error: ' + str(err), file=sys.stderr)
    sys.exit(-1)


if __name__ == '__main__':
  main()
