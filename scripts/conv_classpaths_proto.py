#  Copyright (C) 2021 The Android Open Source Project
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#       http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.

import argparse

import classpaths_pb2

import google.protobuf.json_format as json_format
import google.protobuf.text_format as text_format


def encode(args):
    pb = classpaths_pb2.ExportedClasspathsJars()
    if args.format == 'json':
        json_format.Parse(args.input.read(), pb)
    else:
        text_format.Parse(args.input.read(), pb)
    args.output.write(pb.SerializeToString())
    args.input.close()
    args.output.close()


def decode(args):
    pb = classpaths_pb2.ExportedClasspathsJars()
    pb.ParseFromString(args.input.read())
    if args.format == 'json':
        args.output.write(json_format.MessageToJson(pb))
    else:
        args.output.write(text_format.MessageToString(pb).encode('utf_8'))
    args.input.close()
    args.output.close()


def main():
    parser = argparse.ArgumentParser('Convert classpaths.proto messages between binary and '
                                     'human-readable formats.')
    parser.add_argument('-f', '--format', default='textproto',
                        help='human-readable format, either json or text(proto), '
                             'defaults to textproto')
    parser.add_argument('-i', '--input',
                        nargs='?', type=argparse.FileType('rb'), default=sys.stdin.buffer)
    parser.add_argument('-o', '--output',
                        nargs='?', type=argparse.FileType('wb'),
                        default=sys.stdout.buffer)

    subparsers = parser.add_subparsers()

    parser_encode = subparsers.add_parser('encode',
                                          help='convert classpaths protobuf message from '
                                               'JSON to binary format',
                                          parents=[parser], add_help=False)

    parser_encode.set_defaults(func=encode)

    parser_decode = subparsers.add_parser('decode',
                                          help='print classpaths config in JSON format',
                                          parents=[parser], add_help=False)
    parser_decode.set_defaults(func=decode)

    args = parser.parse_args()
    args.func(args)


if __name__ == '__main__':
    main()
