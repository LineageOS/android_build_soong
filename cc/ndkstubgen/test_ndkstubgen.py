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
"""Tests for ndkstubgen.py."""
import io
import textwrap
import unittest
from copy import copy

import symbolfile
from symbolfile import Arch, Tags

import ndkstubgen


# pylint: disable=missing-docstring


class GeneratorTest(unittest.TestCase):
    def setUp(self) -> None:
        self.filter = symbolfile.Filter(Arch('arm'), 9, False, False)

    def test_omit_version(self) -> None:
        # Thorough testing of the cases involved here is handled by
        # OmitVersionTest, PrivateVersionTest, and SymbolPresenceTest.
        src_file = io.StringIO()
        version_file = io.StringIO()
        symbol_list_file = io.StringIO()
        generator = ndkstubgen.Generator(src_file,
                                         version_file, symbol_list_file,
                                         self.filter)

        version = symbolfile.Version('VERSION_PRIVATE', None, Tags(), [
            symbolfile.Symbol('foo', Tags()),
        ])
        generator.write_version(version)
        self.assertEqual('', src_file.getvalue())
        self.assertEqual('', version_file.getvalue())

        version = symbolfile.Version('VERSION', None, Tags.from_strs(['x86']),
                                     [
                                         symbolfile.Symbol('foo', Tags()),
                                     ])
        generator.write_version(version)
        self.assertEqual('', src_file.getvalue())
        self.assertEqual('', version_file.getvalue())

        version = symbolfile.Version('VERSION', None,
                                     Tags.from_strs(['introduced=14']), [
                                         symbolfile.Symbol('foo', Tags()),
                                     ])
        generator.write_version(version)
        self.assertEqual('', src_file.getvalue())
        self.assertEqual('', version_file.getvalue())

    def test_omit_symbol(self) -> None:
        # Thorough testing of the cases involved here is handled by
        # SymbolPresenceTest.
        src_file = io.StringIO()
        version_file = io.StringIO()
        symbol_list_file = io.StringIO()
        generator = ndkstubgen.Generator(src_file,
                                         version_file, symbol_list_file,
                                         self.filter)

        version = symbolfile.Version('VERSION_1', None, Tags(), [
            symbolfile.Symbol('foo', Tags.from_strs(['x86'])),
        ])
        generator.write_version(version)
        self.assertEqual('', src_file.getvalue())
        self.assertEqual('', version_file.getvalue())

        version = symbolfile.Version('VERSION_1', None, Tags(), [
            symbolfile.Symbol('foo', Tags.from_strs(['introduced=14'])),
        ])
        generator.write_version(version)
        self.assertEqual('', src_file.getvalue())
        self.assertEqual('', version_file.getvalue())

        version = symbolfile.Version('VERSION_1', None, Tags(), [
            symbolfile.Symbol('foo', Tags.from_strs(['llndk'])),
        ])
        generator.write_version(version)
        self.assertEqual('', src_file.getvalue())
        self.assertEqual('', version_file.getvalue())

        version = symbolfile.Version('VERSION_1', None, Tags(), [
            symbolfile.Symbol('foo', Tags.from_strs(['apex'])),
        ])
        generator.write_version(version)
        self.assertEqual('', src_file.getvalue())
        self.assertEqual('', version_file.getvalue())

    def test_write(self) -> None:
        src_file = io.StringIO()
        version_file = io.StringIO()
        symbol_list_file = io.StringIO()
        generator = ndkstubgen.Generator(src_file,
                                         version_file, symbol_list_file,
                                         self.filter)

        versions = [
            symbolfile.Version('VERSION_1', None, Tags(), [
                symbolfile.Symbol('foo', Tags()),
                symbolfile.Symbol('bar', Tags.from_strs(['var'])),
                symbolfile.Symbol('woodly', Tags.from_strs(['weak'])),
                symbolfile.Symbol('doodly', Tags.from_strs(['weak', 'var'])),
            ]),
            symbolfile.Version('VERSION_2', 'VERSION_1', Tags(), [
                symbolfile.Symbol('baz', Tags()),
            ]),
            symbolfile.Version('VERSION_3', 'VERSION_1', Tags(), [
                symbolfile.Symbol('qux', Tags.from_strs(['versioned=14'])),
            ]),
        ]

        generator.write(versions)
        expected_src = textwrap.dedent("""\
            void foo() {}
            int bar = 0;
            __attribute__((weak)) void woodly() {}
            __attribute__((weak)) int doodly = 0;
            void baz() {}
            void qux() {}
        """)
        self.assertEqual(expected_src, src_file.getvalue())

        expected_version = textwrap.dedent("""\
            VERSION_1 {
                global:
                    foo;
                    bar;
                    woodly;
                    doodly;
            };
            VERSION_2 {
                global:
                    baz;
            } VERSION_1;
        """)
        self.assertEqual(expected_version, version_file.getvalue())

        expected_allowlist = textwrap.dedent("""\
            [abi_symbol_list]
            foo
            bar
            woodly
            doodly
            baz
            qux
        """)
        self.assertEqual(expected_allowlist, symbol_list_file.getvalue())


class IntegrationTest(unittest.TestCase):
    def setUp(self) -> None:
        self.filter = symbolfile.Filter(Arch('arm'), 9, False, False)

    def test_integration(self) -> None:
        api_map = {
            'O': 9000,
            'P': 9001,
        }

        input_file = io.StringIO(textwrap.dedent("""\
            VERSION_1 {
                global:
                    foo; # var
                    bar; # x86
                    fizz; # introduced=O
                    buzz; # introduced=P
                local:
                    *;
            };

            VERSION_2 { # arm
                baz; # introduced=9
                qux; # versioned=14
            } VERSION_1;

            VERSION_3 { # introduced=14
                woodly;
                doodly; # var
            } VERSION_2;

            VERSION_4 { # versioned=9
                wibble;
                wizzes; # llndk
                waggle; # apex
            } VERSION_2;

            VERSION_5 { # versioned=14
                wobble;
            } VERSION_4;
        """))
        parser = symbolfile.SymbolFileParser(input_file, api_map, self.filter)
        versions = parser.parse()

        src_file = io.StringIO()
        version_file = io.StringIO()
        symbol_list_file = io.StringIO()
        generator = ndkstubgen.Generator(src_file,
                                         version_file, symbol_list_file,
                                         self.filter)
        generator.write(versions)

        expected_src = textwrap.dedent("""\
            int foo = 0;
            void baz() {}
            void qux() {}
            void wibble() {}
            void wobble() {}
        """)
        self.assertEqual(expected_src, src_file.getvalue())

        expected_version = textwrap.dedent("""\
            VERSION_1 {
                global:
                    foo;
            };
            VERSION_2 {
                global:
                    baz;
            } VERSION_1;
            VERSION_4 {
                global:
                    wibble;
            } VERSION_2;
        """)
        self.assertEqual(expected_version, version_file.getvalue())

        expected_allowlist = textwrap.dedent("""\
            [abi_symbol_list]
            foo
            baz
            qux
            wibble
            wobble
        """)
        self.assertEqual(expected_allowlist, symbol_list_file.getvalue())

    def test_integration_future_api(self) -> None:
        api_map = {
            'O': 9000,
            'P': 9001,
            'Q': 9002,
        }

        input_file = io.StringIO(textwrap.dedent("""\
            VERSION_1 {
                global:
                    foo; # introduced=O
                    bar; # introduced=P
                    baz; # introduced=Q
                local:
                    *;
            };
        """))
        f = copy(self.filter)
        f.api = 9001
        parser = symbolfile.SymbolFileParser(input_file, api_map, f)
        versions = parser.parse()

        src_file = io.StringIO()
        version_file = io.StringIO()
        symbol_list_file = io.StringIO()
        f = copy(self.filter)
        f.api = 9001
        generator = ndkstubgen.Generator(src_file,
                                         version_file, symbol_list_file, f)
        generator.write(versions)

        expected_src = textwrap.dedent("""\
            void foo() {}
            void bar() {}
        """)
        self.assertEqual(expected_src, src_file.getvalue())

        expected_version = textwrap.dedent("""\
            VERSION_1 {
                global:
                    foo;
                    bar;
            };
        """)
        self.assertEqual(expected_version, version_file.getvalue())

        expected_allowlist = textwrap.dedent("""\
            [abi_symbol_list]
            foo
            bar
        """)
        self.assertEqual(expected_allowlist, symbol_list_file.getvalue())

    def test_multiple_definition(self) -> None:
        input_file = io.StringIO(textwrap.dedent("""\
            VERSION_1 {
                global:
                    foo;
                    foo;
                    bar;
                    baz;
                    qux; # arm
                local:
                    *;
            };

            VERSION_2 {
                global:
                    bar;
                    qux; # arm64
            } VERSION_1;

            VERSION_PRIVATE {
                global:
                    baz;
            } VERSION_2;

        """))
        f = copy(self.filter)
        f.api = 16
        parser = symbolfile.SymbolFileParser(input_file, {}, f)

        with self.assertRaises(
                symbolfile.MultiplyDefinedSymbolError) as ex_context:
            parser.parse()
        self.assertEqual(['bar', 'foo'],
                         ex_context.exception.multiply_defined_symbols)

    def test_integration_with_apex(self) -> None:
        api_map = {
            'O': 9000,
            'P': 9001,
        }

        input_file = io.StringIO(textwrap.dedent("""\
            VERSION_1 {
                global:
                    foo; # var
                    bar; # x86
                    fizz; # introduced=O
                    buzz; # introduced=P
                local:
                    *;
            };

            VERSION_2 { # arm
                baz; # introduced=9
                qux; # versioned=14
            } VERSION_1;

            VERSION_3 { # introduced=14
                woodly;
                doodly; # var
            } VERSION_2;

            VERSION_4 { # versioned=9
                wibble;
                wizzes; # llndk
                waggle; # apex
                bubble; # apex llndk
                duddle; # llndk apex
            } VERSION_2;

            VERSION_5 { # versioned=14
                wobble;
            } VERSION_4;
        """))
        f = copy(self.filter)
        f.apex = True
        parser = symbolfile.SymbolFileParser(input_file, api_map, f)
        versions = parser.parse()

        src_file = io.StringIO()
        version_file = io.StringIO()
        symbol_list_file = io.StringIO()
        f = copy(self.filter)
        f.apex = True
        generator = ndkstubgen.Generator(src_file,
                                         version_file, symbol_list_file, f)
        generator.write(versions)

        expected_src = textwrap.dedent("""\
            int foo = 0;
            void baz() {}
            void qux() {}
            void wibble() {}
            void waggle() {}
            void bubble() {}
            void duddle() {}
            void wobble() {}
        """)
        self.assertEqual(expected_src, src_file.getvalue())

        expected_version = textwrap.dedent("""\
            VERSION_1 {
                global:
                    foo;
            };
            VERSION_2 {
                global:
                    baz;
            } VERSION_1;
            VERSION_4 {
                global:
                    wibble;
                    waggle;
                    bubble;
                    duddle;
            } VERSION_2;
        """)
        self.assertEqual(expected_version, version_file.getvalue())

    def test_empty_stub(self) -> None:
        """Tests that empty stubs can be generated.

        This is not a common case, but libraries whose only behavior is to
        interpose symbols to alter existing behavior do not need to expose
        their interposing symbols as API, so it's possible for the stub to be
        empty while still needing a stub to link against. libsigchain is an
        example of this.
        """
        input_file = io.StringIO(textwrap.dedent("""\
            VERSION_1 {
                local:
                    *;
            };
        """))
        f = copy(self.filter)
        f.apex = True
        parser = symbolfile.SymbolFileParser(input_file, {}, f)
        versions = parser.parse()

        src_file = io.StringIO()
        version_file = io.StringIO()
        symbol_list_file = io.StringIO()
        f = copy(self.filter)
        f.apex = True
        generator = ndkstubgen.Generator(src_file,
                                         version_file,
                                         symbol_list_file, f)
        generator.write(versions)

        self.assertEqual('', src_file.getvalue())
        self.assertEqual('', version_file.getvalue())


def main() -> None:
    suite = unittest.TestLoader().loadTestsFromName(__name__)
    unittest.TextTestRunner(verbosity=3).run(suite)


if __name__ == '__main__':
    main()
