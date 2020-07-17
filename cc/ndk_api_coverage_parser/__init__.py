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
"""Generates xml of NDK libraries used for API coverage analysis."""
import argparse
import json
import os
import sys

from xml.etree.ElementTree import Element, SubElement, tostring
from symbolfile import ALL_ARCHITECTURES, FUTURE_API_LEVEL, MultiplyDefinedSymbolError, SymbolFileParser


ROOT_ELEMENT_TAG = 'ndk-library'
SYMBOL_ELEMENT_TAG = 'symbol'
ARCHITECTURE_ATTRIBUTE_KEY = 'arch'
DEPRECATED_ATTRIBUTE_KEY = 'is_deprecated'
PLATFORM_ATTRIBUTE_KEY = 'is_platform'
NAME_ATTRIBUTE_KEY = 'name'
VARIABLE_TAG = 'var'
EXPOSED_TARGET_TAGS = (
    'vndk',
    'apex',
    'llndk',
)
API_LEVEL_TAG_PREFIXES = (
    'introduced=',
    'introduced-',
)


def parse_tags(tags):
    """Parses tags and save needed tags in the created attributes.

    Return attributes dictionary.
    """
    attributes = {}
    arch = []
    for tag in tags:
        if tag.startswith(tuple(API_LEVEL_TAG_PREFIXES)):
            key, _, value = tag.partition('=')
            attributes.update({key: value})
        elif tag in ALL_ARCHITECTURES:
            arch.append(tag)
        elif tag in EXPOSED_TARGET_TAGS:
            attributes.update({tag: 'True'})
    attributes.update({ARCHITECTURE_ATTRIBUTE_KEY: ','.join(arch)})
    return attributes


class XmlGenerator(object):
    """Output generator that writes parsed symbol file to a xml file."""
    def __init__(self, output_file):
        self.output_file = output_file

    def convertToXml(self, versions):
        """Writes all symbol data to the output file."""
        root = Element(ROOT_ELEMENT_TAG)
        for version in versions:
            if VARIABLE_TAG in version.tags:
                continue
            version_attributes = parse_tags(version.tags)
            _, _, postfix = version.name.partition('_')
            is_platform = postfix == 'PRIVATE' or postfix == 'PLATFORM'
            is_deprecated = postfix == 'DEPRECATED'
            version_attributes.update({PLATFORM_ATTRIBUTE_KEY: str(is_platform)})
            version_attributes.update({DEPRECATED_ATTRIBUTE_KEY: str(is_deprecated)})
            for symbol in version.symbols:
                if VARIABLE_TAG in symbol.tags:
                    continue
                attributes = {NAME_ATTRIBUTE_KEY: symbol.name}
                attributes.update(version_attributes)
                # If same version tags already exist, it will be overwrite here.
                attributes.update(parse_tags(symbol.tags))
                SubElement(root, SYMBOL_ELEMENT_TAG, attributes)
        return root

    def write_xml_to_file(self, root):
        """Write xml element root to output_file."""
        parsed_data = tostring(root)
        output_file = open(self.output_file, "wb")
        output_file.write(parsed_data)

    def write(self, versions):
        root = self.convertToXml(versions)
        self.write_xml_to_file(root)


def parse_args():
    """Parses and returns command line arguments."""
    parser = argparse.ArgumentParser()

    parser.add_argument('symbol_file', type=os.path.realpath, help='Path to symbol file.')
    parser.add_argument(
        'output_file', type=os.path.realpath,
        help='The output parsed api coverage file.')
    parser.add_argument(
        '--api-map', type=os.path.realpath, required=True,
        help='Path to the API level map JSON file.')
    return parser.parse_args()


def main():
    """Program entry point."""
    args = parse_args()

    with open(args.api_map) as map_file:
        api_map = json.load(map_file)

    with open(args.symbol_file) as symbol_file:
        try:
            versions = SymbolFileParser(symbol_file, api_map, "", FUTURE_API_LEVEL,
                                        True, True).parse()
        except MultiplyDefinedSymbolError as ex:
            sys.exit('{}: error: {}'.format(args.symbol_file, ex))

    generator = XmlGenerator(args.output_file)
    generator.write(versions)

if __name__ == '__main__':
    main()
