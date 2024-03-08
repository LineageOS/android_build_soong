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
import sys

import linker_config_pb2 #pylint: disable=import-error
from google.protobuf.descriptor import FieldDescriptor
from google.protobuf.json_format import ParseDict
from google.protobuf.text_format import MessageToString


def LoadJsonMessage(path):
    """
    Loads a message from a .json file with `//` comments strippedfor convenience.
    """
    json_content = ''
    with open(path) as f:
        for line in f:
            if not line.lstrip().startswith('//'):
                json_content += line
    obj = json.loads(json_content, object_pairs_hook=collections.OrderedDict)
    return ParseDict(obj, linker_config_pb2.LinkerConfig())


def Proto(args):
    """
    Merges input json files (--source) into a protobuf message (--output).
    Fails if the output file exists. Set --force or --append to deal with the existing
    output file.
    --force to overwrite the output file with the input (.json files).
    --append to append the input to the output file.
    """
    pb = linker_config_pb2.LinkerConfig()
    if os.path.isfile(args.output):
        if args.force:
            pass
        elif args.append:
            with open(args.output, 'rb') as f:
                pb.ParseFromString(f.read())
        else:
            sys.stderr.write(f'Error: {args.output} exists. Use --force or --append.\n')
            sys.exit(1)

    if args.source:
        for input in args.source.split(':'):
            pb.MergeFrom(LoadJsonMessage(input))

    ValidateAndWriteAsPbFile(pb, args.output)


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
        return os.path.exists(lib_path) or os.path.islink(
            lib_path) or os.path.exists(lib64_path) or os.path.islink(
                lib64_path)

    installed_libraries = [lib for lib in libraries if IsInLibPath(lib)]
    for item in installed_libraries:
        if item not in getattr(pb, 'provideLibs'):
            getattr(pb, 'provideLibs').append(item)

    ValidateAndWriteAsPbFile(pb, args.output)


def Append(args):
    pb = linker_config_pb2.LinkerConfig()
    with open(args.source, 'rb') as f:
        pb.ParseFromString(f.read())

    if getattr(type(pb),
               args.key).DESCRIPTOR.label == FieldDescriptor.LABEL_REPEATED:
        for value in args.value.split():
            getattr(pb, args.key).append(value)
    else:
        setattr(pb, args.key, args.value)

    ValidateAndWriteAsPbFile(pb, args.output)



def Merge(args):
    pb = linker_config_pb2.LinkerConfig()
    for other in args.input:
        with open(other, 'rb') as f:
            pb.MergeFromString(f.read())

    ValidateAndWriteAsPbFile(pb, args.output)


def Validate(args):
    if os.path.isdir(args.input):
        config_file = os.path.join(args.input, 'etc/linker.config.pb')
        if os.path.exists(config_file):
            args.input = config_file
            Validate(args)
        # OK if there's no linker config file.
        return

    if not os.path.isfile(args.input):
        sys.exit(f"{args.input} is not a file")

    pb = linker_config_pb2.LinkerConfig()
    with open(args.input, 'rb') as f:
        pb.ParseFromString(f.read())

    if args.type == 'apex':
        # Shouldn't use provideLibs/requireLibs in APEX linker.config.pb
        if getattr(pb, 'provideLibs'):
            sys.exit(f'{args.input}: provideLibs is set. Use provideSharedLibs in apex_manifest')
        if getattr(pb, 'requireLibs'):
            sys.exit(f'{args.input}: requireLibs is set. Use requireSharedLibs in apex_manifest')
    elif args.type == 'system':
        if getattr(pb, 'visible'):
            sys.exit(f'{args.input}: do not use visible, which is for APEX')
        if getattr(pb, 'permittedPaths'):
            sys.exit(f'{args.input}: do not use permittedPaths, which is for APEX')
    else:
        sys.exit(f'Unknown type: {args.type}')

    # Reject contributions field at build time while keeping the runtime behavior for GRF.
    if getattr(pb, 'contributions'):
        sys.exit(f"{args.input}: 'contributions' is set. "
                 "It's deprecated. Instead, make the APEX 'visible' and use android_dlopen_ext().")


def ValidateAndWriteAsPbFile(pb, output_path):
    ValidateConfiguration(pb)
    with open(output_path, 'wb') as f:
        f.write(pb.SerializeToString())


def ValidateConfiguration(pb):
    """
    Validate if the configuration is valid to be used as linker configuration
    """

    # Validate if provideLibs and requireLibs have common module
    provideLibs = set(getattr(pb, 'provideLibs'))
    requireLibs = set(getattr(pb, 'requireLibs'))

    intersectLibs = provideLibs.intersection(requireLibs)

    if intersectLibs:
        for lib in intersectLibs:
            print(f'{lib} exists both in requireLibs and provideLibs', file=sys.stderr)
        sys.exit(1)


def GetArgParser():
    parser = argparse.ArgumentParser()
    subparsers = parser.add_subparsers()

    parser_proto = subparsers.add_parser(
        'proto',
        help='Convert the input JSON configuration file into protobuf.')
    parser_proto.add_argument(
        '-s',
        '--source',
        nargs='?',
        type=str,
        help='Colon-separated list of linker configuration files in JSON.')
    parser_proto.add_argument(
        '-o',
        '--output',
        required=True,
        type=str,
        help='Target path to create protobuf file.')
    option_for_existing_output = parser_proto.add_mutually_exclusive_group()
    option_for_existing_output.add_argument(
        '-f',
        '--force',
        action='store_true',
        help='Overwrite if the output file exists.')
    option_for_existing_output.add_argument(
        '-a',
        '--append',
        action='store_true',
        help='Append the input to the output file if the output file exists.')
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
        'systemprovide',
        help='Append system provide libraries into the configuration.')
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
        help='Values of the libraries to append. If there are more than one '
        'it should be separated by empty space'
    )
    system_provide_libs.add_argument(
        '--system', required=True, type=str, help='Path of the system image.')
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
    append.add_argument('--key', required=True, type=str, help='.')
    append.add_argument(
        '--value',
        required=True,
        type=str,
        help='Values of the libraries to append. If there are more than one'
        'it should be separated by empty space'
    )
    append.set_defaults(func=Append)

    append = subparsers.add_parser('merge', help='Merge configurations')
    append.add_argument(
        '-o',
        '--out',
        required=True,
        type=str,
        help='Output linker configuration file to write in protobuf.')
    append.add_argument(
        '-i',
        '--input',
        nargs='+',
        type=str,
        help='Linker configuration files to merge.')
    append.set_defaults(func=Merge)

    validate = subparsers.add_parser('validate', help='Validate configuration')
    validate.add_argument(
        '--type',
        required=True,
        choices=['apex', 'system'],
        help='Type of linker configuration')
    validate.add_argument(
        'input',
        help='Input can be a directory which has etc/linker.config.pb or a path'
        ' to the linker config file')
    validate.set_defaults(func=Validate)

    return parser


def main():
    parser = GetArgParser()
    args = parser.parse_args()
    if 'func' in args:
        args.func(args)
    else:
        parser.print_help()


if __name__ == '__main__':
    main()
