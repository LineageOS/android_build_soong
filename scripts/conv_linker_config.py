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
""" A tool to convert json file into pb with linker config format."""

import argparse
import collections
import json
import os

import linker_config_pb2
from google.protobuf.descriptor import FieldDescriptor
from google.protobuf.json_format import ParseDict
from google.protobuf.text_format import MessageToString


def Proto(args):
  json_content = ''
  with open(args.source) as f:
    for line in f:
      if not line.lstrip().startswith('//'):
        json_content += line
  obj = json.loads(json_content, object_pairs_hook=collections.OrderedDict)
  pb = ParseDict(obj, linker_config_pb2.LinkerConfig())
  with open(args.output, 'wb') as f:
    f.write(pb.SerializeToString())


def Print(args):
  with open(args.source, 'rb') as f:
    pb = linker_config_pb2.LinkerConfig()
    pb.ParseFromString(f.read())
  print(MessageToString(pb))


def SystemProvide(args):
  pb = linker_config_pb2.LinkerConfig()
  with open(args.source, 'rb') as f:
    pb.ParseFromString(f.read())
  libraries = args.value.split()

  def IsInLibPath(lib_name):
    lib_path = os.path.join(args.system, 'lib', lib_name)
    lib64_path = os.path.join(args.system, 'lib64', lib_name)
    return os.path.exists(lib_path) or os.path.islink(lib_path) or os.path.exists(lib64_path) or os.path.islink(lib64_path)

  installed_libraries = list(filter(IsInLibPath, libraries))
  for item in installed_libraries:
    if item not in getattr(pb, 'provideLibs'):
      getattr(pb, 'provideLibs').append(item)
  with open(args.output, 'wb') as f:
    f.write(pb.SerializeToString())


def Append(args):
  pb = linker_config_pb2.LinkerConfig()
  with open(args.source, 'rb') as f:
    pb.ParseFromString(f.read())

  if getattr(type(pb), args.key).DESCRIPTOR.label == FieldDescriptor.LABEL_REPEATED:
    for value in args.value.split():
      getattr(pb, args.key).append(value)
  else:
    setattr(pb, args.key, args.value)

  with open(args.output, 'wb') as f:
    f.write(pb.SerializeToString())


def GetArgParser():
  parser = argparse.ArgumentParser()
  subparsers = parser.add_subparsers()

  parser_proto = subparsers.add_parser(
      'proto', help='Convert the input JSON configuration file into protobuf.')
  parser_proto.add_argument(
      '-s',
      '--source',
      required=True,
      type=str,
      help='Source linker configuration file in JSON.')
  parser_proto.add_argument(
      '-o',
      '--output',
      required=True,
      type=str,
      help='Target path to create protobuf file.')
  parser_proto.set_defaults(func=Proto)

  print_proto = subparsers.add_parser(
      'print', help='Print configuration in human-readable text format.')
  print_proto.add_argument(
      '-s',
      '--source',
      required=True,
      type=str,
      help='Source linker configuration file in protobuf.')
  print_proto.set_defaults(func=Print)

  system_provide_libs = subparsers.add_parser(
      'systemprovide', help='Append system provide libraries into the configuration.')
  system_provide_libs.add_argument(
      '-s',
      '--source',
      required=True,
      type=str,
      help='Source linker configuration file in protobuf.')
  system_provide_libs.add_argument(
      '-o',
      '--output',
      required=True,
      type=str,
      help='Target linker configuration file to write in protobuf.')
  system_provide_libs.add_argument(
      '--value',
      required=True,
      type=str,
      help='Values of the libraries to append. If there are more than one it should be separated by empty space')
  system_provide_libs.add_argument(
      '--system',
      required=True,
      type=str,
      help='Path of the system image.')
  system_provide_libs.set_defaults(func=SystemProvide)

  append = subparsers.add_parser(
      'append', help='Append value(s) to given key.')
  append.add_argument(
      '-s',
      '--source',
      required=True,
      type=str,
      help='Source linker configuration file in protobuf.')
  append.add_argument(
      '-o',
      '--output',
      required=True,
      type=str,
      help='Target linker configuration file to write in protobuf.')
  append.add_argument(
      '--key',
      required=True,
      type=str,
      help='.')
  append.add_argument(
      '--value',
      required=True,
      type=str,
      help='Values of the libraries to append. If there are more than one it should be separated by empty space')
  append.set_defaults(func=Append)

  return parser


def main():
  args = GetArgParser().parse_args()
  args.func(args)


if __name__ == '__main__':
  main()
