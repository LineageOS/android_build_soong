#!/usr/bin/env python3
#
# Copyright 2018 Google Inc. All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Generates kotlinc module xml file to drive kotlinc

import argparse
import os

from ninja_rsp import NinjaRspFileReader

def parse_args():
  """Parse commandline arguments."""

  def convert_arg_line_to_args(arg_line):
    for arg in arg_line.split():
      if arg.startswith('#'):
        return
      if not arg.strip():
        continue
      yield arg

  parser = argparse.ArgumentParser(fromfile_prefix_chars='@')
  parser.convert_arg_line_to_args = convert_arg_line_to_args
  parser.add_argument('--out', dest='out',
                      help='file to which the module.xml contents will be written.')
  parser.add_argument('--classpath', dest='classpath', action='append', default=[],
                      help='classpath to pass to kotlinc.')
  parser.add_argument('--name', dest='name',
                      help='name of the module.')
  parser.add_argument('--out_dir', dest='out_dir',
                      help='directory to which kotlinc will write output files.')
  parser.add_argument('--srcs', dest='srcs', action='append', default=[],
                      help='file containing whitespace separated list of source files.')
  parser.add_argument('--common_srcs', dest='common_srcs', action='append', default=[],
                      help='file containing whitespace separated list of common multiplatform source files.')

  return parser.parse_args()

def main():
  """Program entry point."""
  args = parse_args()

  if not args.out:
    raise RuntimeError('--out argument is required')

  if not args.name:
    raise RuntimeError('--name argument is required')

  with open(args.out, 'w') as f:
    # Print preamble
    f.write('<modules>\n')
    f.write('  <module name="%s" type="java-production" outputDir="%s">\n' % (args.name, args.out_dir or ''))

    # Print classpath entries
    for c in args.classpath:
      for entry in c.split(':'):
        path = os.path.abspath(entry)
        f.write('    <classpath path="%s"/>\n' % path)

    # For each rsp file, print source entries
    for rsp_file in args.srcs:
      for src in NinjaRspFileReader(rsp_file):
        path = os.path.abspath(src)
        if src.endswith('.java'):
          f.write('    <javaSourceRoots path="%s"/>\n' % path)
        elif src.endswith('.kt'):
          f.write('    <sources path="%s"/>\n' % path)
        else:
          raise RuntimeError('unknown source file type %s' % file)

    for rsp_file in args.common_srcs:
      for src in NinjaRspFileReader(rsp_file):
        path = os.path.abspath(src)
        f.write('    <sources path="%s"/>\n' % path)
        f.write('    <commonSources path="%s"/>\n' % path)

    f.write('  </module>\n')
    f.write('</modules>\n')

if __name__ == '__main__':
  main()
