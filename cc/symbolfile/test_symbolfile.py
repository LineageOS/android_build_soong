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
"""Tests for symbolfile."""
import io
import textwrap
import unittest

import symbolfile
from symbolfile import Arch, Tag

# pylint: disable=missing-docstring


class DecodeApiLevelTest(unittest.TestCase):
    def test_decode_api_level(self) -> None:
        self.assertEqual(9, symbolfile.decode_api_level('9', {}))
        self.assertEqual(9000, symbolfile.decode_api_level('O', {'O': 9000}))

        with self.assertRaises(KeyError):
            symbolfile.decode_api_level('O', {})


class TagsTest(unittest.TestCase):
    def test_get_tags_no_tags(self) -> None:
        self.assertEqual([], symbolfile.get_tags(''))
        self.assertEqual([], symbolfile.get_tags('foo bar baz'))

    def test_get_tags(self) -> None:
        self.assertEqual(['foo', 'bar'], symbolfile.get_tags('# foo bar'))
        self.assertEqual(['bar', 'baz'], symbolfile.get_tags('foo # bar baz'))

    def test_split_tag(self) -> None:
        self.assertTupleEqual(('foo', 'bar'),
                              symbolfile.split_tag(Tag('foo=bar')))
        self.assertTupleEqual(('foo', 'bar=baz'),
                              symbolfile.split_tag(Tag('foo=bar=baz')))
        with self.assertRaises(ValueError):
            symbolfile.split_tag(Tag('foo'))

    def test_get_tag_value(self) -> None:
        self.assertEqual('bar', symbolfile.get_tag_value(Tag('foo=bar')))
        self.assertEqual('bar=baz',
                         symbolfile.get_tag_value(Tag('foo=bar=baz')))
        with self.assertRaises(ValueError):
            symbolfile.get_tag_value(Tag('foo'))

    def test_is_api_level_tag(self) -> None:
        self.assertTrue(symbolfile.is_api_level_tag(Tag('introduced=24')))
        self.assertTrue(symbolfile.is_api_level_tag(Tag('introduced-arm=24')))
        self.assertTrue(symbolfile.is_api_level_tag(Tag('versioned=24')))

        # Shouldn't try to process things that aren't a key/value tag.
        self.assertFalse(symbolfile.is_api_level_tag(Tag('arm')))
        self.assertFalse(symbolfile.is_api_level_tag(Tag('introduced')))
        self.assertFalse(symbolfile.is_api_level_tag(Tag('versioned')))

        # We don't support arch specific `versioned` tags.
        self.assertFalse(symbolfile.is_api_level_tag(Tag('versioned-arm=24')))

    def test_decode_api_level_tags(self) -> None:
        api_map = {
            'O': 9000,
            'P': 9001,
        }

        tags = [
            Tag('introduced=9'),
            Tag('introduced-arm=14'),
            Tag('versioned=16'),
            Tag('arm'),
            Tag('introduced=O'),
            Tag('introduced=P'),
        ]
        expected_tags = [
            Tag('introduced=9'),
            Tag('introduced-arm=14'),
            Tag('versioned=16'),
            Tag('arm'),
            Tag('introduced=9000'),
            Tag('introduced=9001'),
        ]
        self.assertListEqual(
            expected_tags, symbolfile.decode_api_level_tags(tags, api_map))

        with self.assertRaises(symbolfile.ParseError):
            symbolfile.decode_api_level_tags([Tag('introduced=O')], {})


class PrivateVersionTest(unittest.TestCase):
    def test_version_is_private(self) -> None:
        self.assertFalse(symbolfile.version_is_private('foo'))
        self.assertFalse(symbolfile.version_is_private('PRIVATE'))
        self.assertFalse(symbolfile.version_is_private('PLATFORM'))
        self.assertFalse(symbolfile.version_is_private('foo_private'))
        self.assertFalse(symbolfile.version_is_private('foo_platform'))
        self.assertFalse(symbolfile.version_is_private('foo_PRIVATE_'))
        self.assertFalse(symbolfile.version_is_private('foo_PLATFORM_'))

        self.assertTrue(symbolfile.version_is_private('foo_PRIVATE'))
        self.assertTrue(symbolfile.version_is_private('foo_PLATFORM'))


class SymbolPresenceTest(unittest.TestCase):
    def test_symbol_in_arch(self) -> None:
        self.assertTrue(symbolfile.symbol_in_arch([], Arch('arm')))
        self.assertTrue(symbolfile.symbol_in_arch([Tag('arm')], Arch('arm')))

        self.assertFalse(symbolfile.symbol_in_arch([Tag('x86')], Arch('arm')))

    def test_symbol_in_api(self) -> None:
        self.assertTrue(symbolfile.symbol_in_api([], Arch('arm'), 9))
        self.assertTrue(
            symbolfile.symbol_in_api([Tag('introduced=9')], Arch('arm'), 9))
        self.assertTrue(
            symbolfile.symbol_in_api([Tag('introduced=9')], Arch('arm'), 14))
        self.assertTrue(
            symbolfile.symbol_in_api([Tag('introduced-arm=9')], Arch('arm'),
                                     14))
        self.assertTrue(
            symbolfile.symbol_in_api([Tag('introduced-arm=9')], Arch('arm'),
                                     14))
        self.assertTrue(
            symbolfile.symbol_in_api([Tag('introduced-x86=14')], Arch('arm'),
                                     9))
        self.assertTrue(
            symbolfile.symbol_in_api(
                [Tag('introduced-arm=9'),
                 Tag('introduced-x86=21')], Arch('arm'), 14))
        self.assertTrue(
            symbolfile.symbol_in_api(
                [Tag('introduced=9'),
                 Tag('introduced-x86=21')], Arch('arm'), 14))
        self.assertTrue(
            symbolfile.symbol_in_api(
                [Tag('introduced=21'),
                 Tag('introduced-arm=9')], Arch('arm'), 14))
        self.assertTrue(
            symbolfile.symbol_in_api([Tag('future')], Arch('arm'),
                                     symbolfile.FUTURE_API_LEVEL))

        self.assertFalse(
            symbolfile.symbol_in_api([Tag('introduced=14')], Arch('arm'), 9))
        self.assertFalse(
            symbolfile.symbol_in_api([Tag('introduced-arm=14')], Arch('arm'),
                                     9))
        self.assertFalse(
            symbolfile.symbol_in_api([Tag('future')], Arch('arm'), 9))
        self.assertFalse(
            symbolfile.symbol_in_api(
                [Tag('introduced=9'), Tag('future')], Arch('arm'), 14))
        self.assertFalse(
            symbolfile.symbol_in_api([Tag('introduced-arm=9'),
                                      Tag('future')], Arch('arm'), 14))
        self.assertFalse(
            symbolfile.symbol_in_api(
                [Tag('introduced-arm=21'),
                 Tag('introduced-x86=9')], Arch('arm'), 14))
        self.assertFalse(
            symbolfile.symbol_in_api(
                [Tag('introduced=9'),
                 Tag('introduced-arm=21')], Arch('arm'), 14))
        self.assertFalse(
            symbolfile.symbol_in_api(
                [Tag('introduced=21'),
                 Tag('introduced-x86=9')], Arch('arm'), 14))

        # Interesting edge case: this symbol should be omitted from the
        # library, but this call should still return true because none of the
        # tags indiciate that it's not present in this API level.
        self.assertTrue(symbolfile.symbol_in_api([Tag('x86')], Arch('arm'), 9))

    def test_verioned_in_api(self) -> None:
        self.assertTrue(symbolfile.symbol_versioned_in_api([], 9))
        self.assertTrue(
            symbolfile.symbol_versioned_in_api([Tag('versioned=9')], 9))
        self.assertTrue(
            symbolfile.symbol_versioned_in_api([Tag('versioned=9')], 14))

        self.assertFalse(
            symbolfile.symbol_versioned_in_api([Tag('versioned=14')], 9))


class OmitVersionTest(unittest.TestCase):
    def test_omit_private(self) -> None:
        self.assertFalse(
            symbolfile.should_omit_version(
                symbolfile.Version('foo', None, [], []), Arch('arm'), 9, False,
                False))

        self.assertTrue(
            symbolfile.should_omit_version(
                symbolfile.Version('foo_PRIVATE', None, [], []), Arch('arm'),
                9, False, False))
        self.assertTrue(
            symbolfile.should_omit_version(
                symbolfile.Version('foo_PLATFORM', None, [], []), Arch('arm'),
                9, False, False))

        self.assertTrue(
            symbolfile.should_omit_version(
                symbolfile.Version('foo', None, [Tag('platform-only')], []),
                Arch('arm'), 9, False, False))

    def test_omit_llndk(self) -> None:
        self.assertTrue(
            symbolfile.should_omit_version(
                symbolfile.Version('foo', None, [Tag('llndk')], []),
                Arch('arm'), 9, False, False))

        self.assertFalse(
            symbolfile.should_omit_version(
                symbolfile.Version('foo', None, [], []), Arch('arm'), 9, True,
                False))
        self.assertFalse(
            symbolfile.should_omit_version(
                symbolfile.Version('foo', None, [Tag('llndk')], []),
                Arch('arm'), 9, True, False))

    def test_omit_apex(self) -> None:
        self.assertTrue(
            symbolfile.should_omit_version(
                symbolfile.Version('foo', None, [Tag('apex')], []),
                Arch('arm'), 9, False, False))

        self.assertFalse(
            symbolfile.should_omit_version(
                symbolfile.Version('foo', None, [], []), Arch('arm'), 9, False,
                True))
        self.assertFalse(
            symbolfile.should_omit_version(
                symbolfile.Version('foo', None, [Tag('apex')], []),
                Arch('arm'), 9, False, True))

    def test_omit_arch(self) -> None:
        self.assertFalse(
            symbolfile.should_omit_version(
                symbolfile.Version('foo', None, [], []), Arch('arm'), 9, False,
                False))
        self.assertFalse(
            symbolfile.should_omit_version(
                symbolfile.Version('foo', None, [Tag('arm')], []), Arch('arm'),
                9, False, False))

        self.assertTrue(
            symbolfile.should_omit_version(
                symbolfile.Version('foo', None, [Tag('x86')], []), Arch('arm'),
                9, False, False))

    def test_omit_api(self) -> None:
        self.assertFalse(
            symbolfile.should_omit_version(
                symbolfile.Version('foo', None, [], []), Arch('arm'), 9, False,
                False))
        self.assertFalse(
            symbolfile.should_omit_version(
                symbolfile.Version('foo', None, [Tag('introduced=9')], []),
                Arch('arm'), 9, False, False))

        self.assertTrue(
            symbolfile.should_omit_version(
                symbolfile.Version('foo', None, [Tag('introduced=14')], []),
                Arch('arm'), 9, False, False))


class OmitSymbolTest(unittest.TestCase):
    def test_omit_llndk(self) -> None:
        self.assertTrue(
            symbolfile.should_omit_symbol(
                symbolfile.Symbol('foo', [Tag('llndk')]), Arch('arm'), 9,
                False, False))

        self.assertFalse(
            symbolfile.should_omit_symbol(symbolfile.Symbol('foo', []),
                                          Arch('arm'), 9, True, False))
        self.assertFalse(
            symbolfile.should_omit_symbol(
                symbolfile.Symbol('foo', [Tag('llndk')]), Arch('arm'), 9, True,
                False))

    def test_omit_apex(self) -> None:
        self.assertTrue(
            symbolfile.should_omit_symbol(
                symbolfile.Symbol('foo', [Tag('apex')]), Arch('arm'), 9, False,
                False))

        self.assertFalse(
            symbolfile.should_omit_symbol(symbolfile.Symbol('foo', []),
                                          Arch('arm'), 9, False, True))
        self.assertFalse(
            symbolfile.should_omit_symbol(
                symbolfile.Symbol('foo', [Tag('apex')]), Arch('arm'), 9, False,
                True))

    def test_omit_arch(self) -> None:
        self.assertFalse(
            symbolfile.should_omit_symbol(symbolfile.Symbol('foo', []),
                                          Arch('arm'), 9, False, False))
        self.assertFalse(
            symbolfile.should_omit_symbol(
                symbolfile.Symbol('foo', [Tag('arm')]), Arch('arm'), 9, False,
                False))

        self.assertTrue(
            symbolfile.should_omit_symbol(
                symbolfile.Symbol('foo', [Tag('x86')]), Arch('arm'), 9, False,
                False))

    def test_omit_api(self) -> None:
        self.assertFalse(
            symbolfile.should_omit_symbol(symbolfile.Symbol('foo', []),
                                          Arch('arm'), 9, False, False))
        self.assertFalse(
            symbolfile.should_omit_symbol(
                symbolfile.Symbol('foo', [Tag('introduced=9')]), Arch('arm'),
                9, False, False))

        self.assertTrue(
            symbolfile.should_omit_symbol(
                symbolfile.Symbol('foo', [Tag('introduced=14')]), Arch('arm'),
                9, False, False))


class SymbolFileParseTest(unittest.TestCase):
    def test_next_line(self) -> None:
        input_file = io.StringIO(textwrap.dedent("""\
            foo

            bar
            # baz
            qux
        """))
        parser = symbolfile.SymbolFileParser(input_file, {}, Arch('arm'), 16,
                                             False, False)
        self.assertIsNone(parser.current_line)

        self.assertEqual('foo', parser.next_line().strip())
        assert parser.current_line is not None
        self.assertEqual('foo', parser.current_line.strip())

        self.assertEqual('bar', parser.next_line().strip())
        self.assertEqual('bar', parser.current_line.strip())

        self.assertEqual('qux', parser.next_line().strip())
        self.assertEqual('qux', parser.current_line.strip())

        self.assertEqual('', parser.next_line())
        self.assertEqual('', parser.current_line)

    def test_parse_version(self) -> None:
        input_file = io.StringIO(textwrap.dedent("""\
            VERSION_1 { # foo bar
                baz;
                qux; # woodly doodly
            };

            VERSION_2 {
            } VERSION_1; # asdf
        """))
        parser = symbolfile.SymbolFileParser(input_file, {}, Arch('arm'), 16,
                                             False, False)

        parser.next_line()
        version = parser.parse_version()
        self.assertEqual('VERSION_1', version.name)
        self.assertIsNone(version.base)
        self.assertEqual(['foo', 'bar'], version.tags)

        expected_symbols = [
            symbolfile.Symbol('baz', []),
            symbolfile.Symbol('qux', [Tag('woodly'), Tag('doodly')]),
        ]
        self.assertEqual(expected_symbols, version.symbols)

        parser.next_line()
        version = parser.parse_version()
        self.assertEqual('VERSION_2', version.name)
        self.assertEqual('VERSION_1', version.base)
        self.assertEqual([], version.tags)

    def test_parse_version_eof(self) -> None:
        input_file = io.StringIO(textwrap.dedent("""\
            VERSION_1 {
        """))
        parser = symbolfile.SymbolFileParser(input_file, {}, Arch('arm'), 16,
                                             False, False)
        parser.next_line()
        with self.assertRaises(symbolfile.ParseError):
            parser.parse_version()

    def test_unknown_scope_label(self) -> None:
        input_file = io.StringIO(textwrap.dedent("""\
            VERSION_1 {
                foo:
            }
        """))
        parser = symbolfile.SymbolFileParser(input_file, {}, Arch('arm'), 16,
                                             False, False)
        parser.next_line()
        with self.assertRaises(symbolfile.ParseError):
            parser.parse_version()

    def test_parse_symbol(self) -> None:
        input_file = io.StringIO(textwrap.dedent("""\
            foo;
            bar; # baz qux
        """))
        parser = symbolfile.SymbolFileParser(input_file, {}, Arch('arm'), 16,
                                             False, False)

        parser.next_line()
        symbol = parser.parse_symbol()
        self.assertEqual('foo', symbol.name)
        self.assertEqual([], symbol.tags)

        parser.next_line()
        symbol = parser.parse_symbol()
        self.assertEqual('bar', symbol.name)
        self.assertEqual(['baz', 'qux'], symbol.tags)

    def test_wildcard_symbol_global(self) -> None:
        input_file = io.StringIO(textwrap.dedent("""\
            VERSION_1 {
                *;
            };
        """))
        parser = symbolfile.SymbolFileParser(input_file, {}, Arch('arm'), 16,
                                             False, False)
        parser.next_line()
        with self.assertRaises(symbolfile.ParseError):
            parser.parse_version()

    def test_wildcard_symbol_local(self) -> None:
        input_file = io.StringIO(textwrap.dedent("""\
            VERSION_1 {
                local:
                    *;
            };
        """))
        parser = symbolfile.SymbolFileParser(input_file, {}, Arch('arm'), 16,
                                             False, False)
        parser.next_line()
        version = parser.parse_version()
        self.assertEqual([], version.symbols)

    def test_missing_semicolon(self) -> None:
        input_file = io.StringIO(textwrap.dedent("""\
            VERSION_1 {
                foo
            };
        """))
        parser = symbolfile.SymbolFileParser(input_file, {}, Arch('arm'), 16,
                                             False, False)
        parser.next_line()
        with self.assertRaises(symbolfile.ParseError):
            parser.parse_version()

    def test_parse_fails_invalid_input(self) -> None:
        with self.assertRaises(symbolfile.ParseError):
            input_file = io.StringIO('foo')
            parser = symbolfile.SymbolFileParser(input_file, {}, Arch('arm'),
                                                 16, False, False)
            parser.parse()

    def test_parse(self) -> None:
        input_file = io.StringIO(textwrap.dedent("""\
            VERSION_1 {
                local:
                    hidden1;
                global:
                    foo;
                    bar; # baz
            };

            VERSION_2 { # wasd
                # Implicit global scope.
                    woodly;
                    doodly; # asdf
                local:
                    qwerty;
            } VERSION_1;
        """))
        parser = symbolfile.SymbolFileParser(input_file, {}, Arch('arm'), 16,
                                             False, False)
        versions = parser.parse()

        expected = [
            symbolfile.Version('VERSION_1', None, [], [
                symbolfile.Symbol('foo', []),
                symbolfile.Symbol('bar', [Tag('baz')]),
            ]),
            symbolfile.Version('VERSION_2', 'VERSION_1', [Tag('wasd')], [
                symbolfile.Symbol('woodly', []),
                symbolfile.Symbol('doodly', [Tag('asdf')]),
            ]),
        ]

        self.assertEqual(expected, versions)

    def test_parse_llndk_apex_symbol(self) -> None:
        input_file = io.StringIO(textwrap.dedent("""\
            VERSION_1 {
                foo;
                bar; # llndk
                baz; # llndk apex
                qux; # apex
            };
        """))
        parser = symbolfile.SymbolFileParser(input_file, {}, Arch('arm'), 16,
                                             False, True)

        parser.next_line()
        version = parser.parse_version()
        self.assertEqual('VERSION_1', version.name)
        self.assertIsNone(version.base)

        expected_symbols = [
            symbolfile.Symbol('foo', []),
            symbolfile.Symbol('bar', [Tag('llndk')]),
            symbolfile.Symbol('baz', [Tag('llndk'), Tag('apex')]),
            symbolfile.Symbol('qux', [Tag('apex')]),
        ]
        self.assertEqual(expected_symbols, version.symbols)


def main() -> None:
    suite = unittest.TestLoader().loadTestsFromName(__name__)
    unittest.TextTestRunner(verbosity=3).run(suite)


if __name__ == '__main__':
    main()
