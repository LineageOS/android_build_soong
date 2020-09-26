#!/usr/bin/env python
#
# Copyright (C) 2016 The Android Open Source Project
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
"""Generates source for stub shared libraries for the NDK."""
import argparse
import json
import logging
import os
import sys
from typing import Iterable, TextIO

import symbolfile
from symbolfile import Arch, Version


class Generator:
    """Output generator that writes stub source files and version scripts."""
    def __init__(self, src_file: TextIO, version_script: TextIO, arch: Arch,
                 api: int, llndk: bool, apex: bool) -> None:
        self.src_file = src_file
        self.version_script = version_script
        self.arch = arch
        self.api = api
        self.llndk = llndk
        self.apex = apex

    def write(self, versions: Iterable[Version]) -> None:
        """Writes all symbol data to the output files."""
        for version in versions:
            self.write_version(version)

    def write_version(self, version: Version) -> None:
        """Writes a single version block's data to the output files."""
        if symbolfile.should_omit_version(version, self.arch, self.api,
                                          self.llndk, self.apex):
            return

        section_versioned = symbolfile.symbol_versioned_in_api(
            version.tags, self.api)
        version_empty = True
        pruned_symbols = []
        for symbol in version.symbols:
            if symbolfile.should_omit_symbol(symbol, self.arch, self.api,
                                             self.llndk, self.apex):
                continue

            if symbolfile.symbol_versioned_in_api(symbol.tags, self.api):
                version_empty = False
            pruned_symbols.append(symbol)

        if len(pruned_symbols) > 0:
            if not version_empty and section_versioned:
                self.version_script.write(version.name + ' {\n')
                self.version_script.write('    global:\n')
            for symbol in pruned_symbols:
                emit_version = symbolfile.symbol_versioned_in_api(
                    symbol.tags, self.api)
                if section_versioned and emit_version:
                    self.version_script.write('        ' + symbol.name + ';\n')

                weak = ''
                if 'weak' in symbol.tags:
                    weak = '__attribute__((weak)) '

                if 'var' in symbol.tags:
                    self.src_file.write('{}int {} = 0;\n'.format(
                        weak, symbol.name))
                else:
                    self.src_file.write('{}void {}() {{}}\n'.format(
                        weak, symbol.name))

            if not version_empty and section_versioned:
                base = '' if version.base is None else ' ' + version.base
                self.version_script.write('}' + base + ';\n')


def parse_args() -> argparse.Namespace:
    """Parses and returns command line arguments."""
    parser = argparse.ArgumentParser()

    parser.add_argument('-v', '--verbose', action='count', default=0)

    parser.add_argument(
        '--api', required=True, help='API level being targeted.')
    parser.add_argument(
        '--arch', choices=symbolfile.ALL_ARCHITECTURES, required=True,
        help='Architecture being targeted.')
    parser.add_argument(
        '--llndk', action='store_true', help='Use the LLNDK variant.')
    parser.add_argument(
        '--apex', action='store_true', help='Use the APEX variant.')

    # https://github.com/python/mypy/issues/1317
    # mypy has issues with using os.path.realpath as an argument here.
    parser.add_argument(
        '--api-map',
        type=os.path.realpath,  # type: ignore
        required=True,
        help='Path to the API level map JSON file.')

    parser.add_argument(
        'symbol_file',
        type=os.path.realpath,  # type: ignore
        help='Path to symbol file.')
    parser.add_argument(
        'stub_src',
        type=os.path.realpath,  # type: ignore
        help='Path to output stub source file.')
    parser.add_argument(
        'version_script',
        type=os.path.realpath,  # type: ignore
        help='Path to output version script.')

    return parser.parse_args()


def main() -> None:
    """Program entry point."""
    args = parse_args()

    with open(args.api_map) as map_file:
        api_map = json.load(map_file)
    api = symbolfile.decode_api_level(args.api, api_map)

    verbose_map = (logging.WARNING, logging.INFO, logging.DEBUG)
    verbosity = args.verbose
    if verbosity > 2:
        verbosity = 2
    logging.basicConfig(level=verbose_map[verbosity])

    with open(args.symbol_file) as symbol_file:
        try:
            versions = symbolfile.SymbolFileParser(symbol_file, api_map,
                                                   args.arch, api, args.llndk,
                                                   args.apex).parse()
        except symbolfile.MultiplyDefinedSymbolError as ex:
            sys.exit('{}: error: {}'.format(args.symbol_file, ex))

    with open(args.stub_src, 'w') as src_file:
        with open(args.version_script, 'w') as version_file:
            generator = Generator(src_file, version_file, args.arch, api,
                                  args.llndk, args.apex)
            generator.write(versions)


if __name__ == '__main__':
    main()
