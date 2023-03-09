#!/usr/bin/env python
#
# Copyright (C) 2023 The Android Open Source Project
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
"""Unit tests for conv_linker_config.py."""

import os
import shutil
import tempfile
import unittest

import conv_linker_config
from linker_config_pb2 import LinkerConfig

class FileArgs:
  def __init__(self, files, sep = ':'):
    self.files = files
    self.sep = sep


class FileArg:
  def __init__(self, file):
    self.file = file


class TempDirTest(unittest.TestCase):

  def setUp(self):
    self.tempdir = tempfile.mkdtemp()


  def tearDown(self):
    shutil.rmtree(self.tempdir)


  def write(self, name, contents):
    with open(os.path.join(self.tempdir, name), 'wb') as f:
      f.write(contents)


  def read(self, name):
    with open(os.path.join(self.tempdir, name), 'rb') as f:
      return f.read()


  def resolve_paths(self, args):
    for i in range(len(args)):
      if isinstance(args[i], FileArgs):
        args[i] = args[i].sep.join(os.path.join(self.tempdir, f.file) for f in args[i].files)
      elif isinstance(args[i], FileArg):
        args[i] = os.path.join(self.tempdir, args[i].file)
    return args


class ConvLinkerConfigTest(TempDirTest):
  """Unit tests for conv_linker_config."""

  def test_Proto_with_multiple_input(self):
    self.write('foo.json', b'{ "provideLibs": ["libfoo.so"]}')
    self.write('bar.json', b'{ "provideLibs": ["libbar.so"]}')
    self.command(['proto', '-s', FileArgs([FileArg('foo.json'), FileArg('bar.json')]), '-o', FileArg('out.pb')])
    pb = LinkerConfig()
    pb.ParseFromString(self.read('out.pb'))
    self.assertSetEqual(set(pb.provideLibs), set(['libfoo.so', 'libbar.so']))


  def command(self, args):
    parser = conv_linker_config.GetArgParser()
    parsed_args = parser.parse_args(self.resolve_paths(args))
    parsed_args.func(parsed_args)


if __name__ == '__main__':
  unittest.main(verbosity=2)
