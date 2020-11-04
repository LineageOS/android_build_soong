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

import linker_config_pb2
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

  return parser


def main():
  args = GetArgParser().parse_args()
  args.func(args)


if __name__ == '__main__':
  main()
