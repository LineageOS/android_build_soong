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
"""list_image is a tool that prints out content of an .img file.

To print content of an image to stdout:
  list_image foo.img
"""
from __future__ import print_function

import argparse
import os
import sys
import subprocess

class ImageEntry(object):

  def __init__(self, name, base_dir, is_directory=False):
    self._name = name
    self._base_dir = base_dir
    self._is_directory = is_directory

  @property
  def name(self):
    return self._name

  @property
  def full_path(self):
    return os.path.join(self._base_dir, self._name)

  @property
  def is_directory(self):
    return self._is_directory


class Image(object):

  def __init__(self, debugfs_path, img_path):
    self._debugfs = debugfs_path
    self._img_path = img_path

  def list(self, path):
    print(path)
    process = subprocess.Popen([self._debugfs, '-R', 'ls -l -p %s' % path, self._img_path],
                               stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                               universal_newlines=True)
    stdout, _ = process.communicate()
    res = str(stdout)
    entries = []
    for line in res.split('\n'):
      if not line:
        continue
      parts = line.split('/')
      if len(parts) != 8:
        continue
      name = parts[5]
      if not name:
        continue
      bits = parts[2]
      is_directory = bits[1] == '4'
      entries.append(ImageEntry(name, path, is_directory))

    for e in sorted(entries, key=lambda e: e.name):
      yield e
      if e.is_directory and e.name != '.' and e.name != '..':
        yield from self.list(path + e.name + '/')


def main(argv):
  parser = argparse.ArgumentParser()

  debugfs_default = None
  if 'ANDROID_HOST_OUT' in os.environ:
    debugfs_default = '%s/bin/debugfs_static' % os.environ['ANDROID_HOST_OUT']
  parser.add_argument('--debugfs_path', help='The path to debugfs binary', default=debugfs_default)
  parser.add_argument('img_path', type=str, help='.img file')
  args = parser.parse_args(argv)

  if not args.debugfs_path:
    print('ANDROID_HOST_OUT environment variable is not defined, --debugfs_path must be set',
          file=sys.stderr)
    sys.exit(1)

  for e in Image(args.debugfs_path, args.img_path).list('./'):
    if e.is_directory:
      continue
    print(e.full_path)


if __name__ == '__main__':
  main(sys.argv[1:])
