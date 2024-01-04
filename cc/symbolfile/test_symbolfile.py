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
from symbolfile import Arch, Tag, Tags, Version, Symbol, Filter
from copy import copy

# pylint: disable=missing-docstring


class DecodeApiLevelTest(unittest.TestCase):
    def test_decode_api_level(self) -> None:
        self.assertEqual(9, symbolfile.decode_api_level('9', {}))
        self.assertEqual(9000, symbolfile.decode_api_level('O', {'O': 9000}))

        with self.assertRaises(KeyError):
            symbolfile.decode_api_level('O', {})


class TagsTest(unittest.TestCase):
    def test_get_tags_no_tags(self) -> None:
        self.assertEqual(Tags(), symbolfile.get_tags('', {}))
        self.assertEqual(Tags(), symbolfile.get_tags('foo bar baz', {}))

    def test_get_tags(self) -> None:
        self.assertEqual(Tags.from_strs(['llndk', 'apex']),
                         symbolfile.get_tags('# llndk apex', {}))
        self.assertEqual(Tags.from_strs(['llndk', 'apex']),
                         symbolfile.get_tags('foo # llndk apex', {}))

    def test_get_unrecognized_tags(self) -> None:
        with self.assertRaises(symbolfile.ParseError):
            symbolfile.get_tags('# bar', {})
        with self.assertRaises(symbolfile.ParseError):
            symbolfile.get_tags('foo # bar', {})
        with self.assertRaises(symbolfile.ParseError):
            symbolfile.get_tags('# #', {})
        with self.assertRaises(symbolfile.ParseError):
            symbolfile.get_tags('# apex # llndk', {})

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
            symbolfile.decode_api_level_tag(t, api_map) for t in (
                Tag('introduced=9'),
                Tag('introduced-arm=14'),
                Tag('versioned=16'),
                Tag('arm'),
                Tag('introduced=O'),
                Tag('introduced=P'),
            )
        ]
        expected_tags = [
            Tag('introduced=9'),
            Tag('introduced-arm=14'),
            Tag('versioned=16'),
            Tag('arm'),
            Tag('introduced=9000'),
            Tag('introduced=9001'),
        ]
        self.assertListEqual(expected_tags, tags)

        with self.assertRaises(symbolfile.ParseError):
            symbolfile.decode_api_level_tag(Tag('introduced=O'), {})


class PrivateVersionTest(unittest.TestCase):
    def test_version_is_private(self) -> None:
        def mock_version(name: str) -> Version:
            return Version(name, base=None, tags=Tags(), symbols=[])

        self.assertFalse(mock_version('foo').is_private)
        self.assertFalse(mock_version('PRIVATE').is_private)
        self.assertFalse(mock_version('PLATFORM').is_private)
        self.assertFalse(mock_version('foo_private').is_private)
        self.assertFalse(mock_version('foo_platform').is_private)
        self.assertFalse(mock_version('foo_PRIVATE_').is_private)
        self.assertFalse(mock_version('foo_PLATFORM_').is_private)

        self.assertTrue(mock_version('foo_PRIVATE').is_private)
        self.assertTrue(mock_version('foo_PLATFORM').is_private)


class SymbolPresenceTest(unittest.TestCase):
    def test_symbol_in_arch(self) -> None:
        self.assertTrue(symbolfile.symbol_in_arch(Tags(), Arch('arm')))
        self.assertTrue(
            symbolfile.symbol_in_arch(Tags.from_strs(['arm']), Arch('arm')))

        self.assertFalse(
            symbolfile.symbol_in_arch(Tags.from_strs(['x86']), Arch('arm')))

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
    def setUp(self) -> None:
        self.filter = Filter(arch = Arch('arm'), api = 9)
        self.version = Version('foo', None, Tags(), [])

    def assertOmit(self, f: Filter, v: Version) -> None:
        self.assertTrue(f.should_omit_version(v))

    def assertInclude(self, f: Filter, v: Version) -> None:
        self.assertFalse(f.should_omit_version(v))

    def test_omit_private(self) -> None:
        f = self.filter
        v = self.version

        self.assertInclude(f, v)

        v.name = 'foo_PRIVATE'
        self.assertOmit(f, v)

        v.name = 'foo_PLATFORM'
        self.assertOmit(f, v)

        v.name = 'foo'
        v.tags = Tags.from_strs(['platform-only'])
        self.assertOmit(f, v)

    def test_omit_llndk(self) -> None:
        f = self.filter
        v = self.version
        v_llndk = copy(v)
        v_llndk.tags = Tags.from_strs(['llndk'])

        self.assertOmit(f, v_llndk)

        f.llndk = True
        self.assertInclude(f, v)
        self.assertInclude(f, v_llndk)

    def test_omit_apex(self) -> None:
        f = self.filter
        v = self.version
        v_apex = copy(v)
        v_apex.tags = Tags.from_strs(['apex'])
        v_systemapi = copy(v)
        v_systemapi.tags = Tags.from_strs(['systemapi'])

        self.assertOmit(f, v_apex)

        f.apex = True
        self.assertInclude(f, v)
        self.assertInclude(f, v_apex)
        self.assertOmit(f, v_systemapi)

    def test_omit_systemapi(self) -> None:
        f = self.filter
        v = self.version
        v_apex = copy(v)
        v_apex.tags = Tags.from_strs(['apex'])
        v_systemapi = copy(v)
        v_systemapi.tags = Tags.from_strs(['systemapi'])

        self.assertOmit(f, v_systemapi)

        f.systemapi = True
        self.assertInclude(f, v)
        self.assertInclude(f, v_systemapi)
        self.assertOmit(f, v_apex)

    def test_omit_arch(self) -> None:
        f_arm = self.filter
        v_none = self.version
        self.assertInclude(f_arm, v_none)

        v_arm = copy(v_none)
        v_arm.tags = Tags.from_strs(['arm'])
        self.assertInclude(f_arm, v_arm)

        v_x86 = copy(v_none)
        v_x86.tags = Tags.from_strs(['x86'])
        self.assertOmit(f_arm, v_x86)

    def test_omit_api(self) -> None:
        f_api9 = self.filter
        v_none = self.version
        self.assertInclude(f_api9, v_none)

        v_api9 = copy(v_none)
        v_api9.tags = Tags.from_strs(['introduced=9'])
        self.assertInclude(f_api9, v_api9)

        v_api14 = copy(v_none)
        v_api14.tags = Tags.from_strs(['introduced=14'])
        self.assertOmit(f_api9, v_api14)


class OmitSymbolTest(unittest.TestCase):
    def setUp(self) -> None:
        self.filter = Filter(arch = Arch('arm'), api = 9)

    def assertOmit(self, f: Filter, s: Symbol) -> None:
        self.assertTrue(f.should_omit_symbol(s))

    def assertInclude(self, f: Filter, s: Symbol) -> None:
        self.assertFalse(f.should_omit_symbol(s))

    def test_omit_ndk(self) -> None:
        f_ndk = self.filter
        f_nondk = copy(f_ndk)
        f_nondk.ndk = False
        f_nondk.apex = True

        s_ndk = Symbol('foo', Tags())
        s_nonndk = Symbol('foo', Tags.from_strs(['apex']))

        self.assertInclude(f_ndk, s_ndk)
        self.assertOmit(f_ndk, s_nonndk)
        self.assertOmit(f_nondk, s_ndk)
        self.assertInclude(f_nondk, s_nonndk)

    def test_omit_llndk(self) -> None:
        f_none = self.filter
        f_llndk = copy(f_none)
        f_llndk.llndk = True

        s_none = Symbol('foo', Tags())
        s_llndk = Symbol('foo', Tags.from_strs(['llndk']))

        self.assertOmit(f_none, s_llndk)
        self.assertInclude(f_llndk, s_none)
        self.assertInclude(f_llndk, s_llndk)

    def test_omit_apex(self) -> None:
        f_none = self.filter
        f_apex = copy(f_none)
        f_apex.apex = True

        s_none = Symbol('foo', Tags())
        s_apex = Symbol('foo', Tags.from_strs(['apex']))
        s_systemapi = Symbol('foo', Tags.from_strs(['systemapi']))

        self.assertOmit(f_none, s_apex)
        self.assertInclude(f_apex, s_none)
        self.assertInclude(f_apex, s_apex)
        self.assertOmit(f_apex, s_systemapi)

    def test_omit_systemapi(self) -> None:
        f_none = self.filter
        f_systemapi = copy(f_none)
        f_systemapi.systemapi = True

        s_none = Symbol('foo', Tags())
        s_apex = Symbol('foo', Tags.from_strs(['apex']))
        s_systemapi = Symbol('foo', Tags.from_strs(['systemapi']))

        self.assertOmit(f_none, s_systemapi)
        self.assertInclude(f_systemapi, s_none)
        self.assertInclude(f_systemapi, s_systemapi)
        self.assertOmit(f_systemapi, s_apex)

    def test_omit_apex_and_systemapi(self) -> None:
        f = self.filter
        f.systemapi = True
        f.apex = True

        s_none = Symbol('foo', Tags())
        s_apex = Symbol('foo', Tags.from_strs(['apex']))
        s_systemapi = Symbol('foo', Tags.from_strs(['systemapi']))
        self.assertInclude(f, s_none)
        self.assertInclude(f, s_apex)
        self.assertInclude(f, s_systemapi)

    def test_omit_arch(self) -> None:
        f_arm = self.filter
        s_none = Symbol('foo', Tags())
        s_arm = Symbol('foo', Tags.from_strs(['arm']))
        s_x86 = Symbol('foo', Tags.from_strs(['x86']))

        self.assertInclude(f_arm, s_none)
        self.assertInclude(f_arm, s_arm)
        self.assertOmit(f_arm, s_x86)

    def test_omit_api(self) -> None:
        f_api9 = self.filter
        s_none = Symbol('foo', Tags())
        s_api9 = Symbol('foo', Tags.from_strs(['introduced=9']))
        s_api14 = Symbol('foo', Tags.from_strs(['introduced=14']))

        self.assertInclude(f_api9, s_none)
        self.assertInclude(f_api9, s_api9)
        self.assertOmit(f_api9, s_api14)


class SymbolFileParseTest(unittest.TestCase):
    def setUp(self) -> None:
        self.filter = Filter(arch = Arch('arm'), api = 16)

    def test_next_line(self) -> None:
        input_file = io.StringIO(textwrap.dedent("""\
            foo

            bar
            # baz
            qux
        """))
        parser = symbolfile.SymbolFileParser(input_file, {}, self.filter)
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
            VERSION_1 { # weak introduced=35
                baz;
                qux; # apex llndk
            };

            VERSION_2 {
            } VERSION_1; # not-a-tag
        """))
        parser = symbolfile.SymbolFileParser(input_file, {}, self.filter)

        parser.next_line()
        version = parser.parse_version()
        self.assertEqual('VERSION_1', version.name)
        self.assertIsNone(version.base)
        self.assertEqual(Tags.from_strs(['weak', 'introduced=35']), version.tags)

        expected_symbols = [
            Symbol('baz', Tags()),
            Symbol('qux', Tags.from_strs(['apex', 'llndk'])),
        ]
        self.assertEqual(expected_symbols, version.symbols)

        parser.next_line()
        version = parser.parse_version()
        self.assertEqual('VERSION_2', version.name)
        self.assertEqual('VERSION_1', version.base)
        self.assertEqual(Tags(), version.tags)

    def test_parse_version_eof(self) -> None:
        input_file = io.StringIO(textwrap.dedent("""\
            VERSION_1 {
        """))
        parser = symbolfile.SymbolFileParser(input_file, {}, self.filter)
        parser.next_line()
        with self.assertRaises(symbolfile.ParseError):
            parser.parse_version()

    def test_unknown_scope_label(self) -> None:
        input_file = io.StringIO(textwrap.dedent("""\
            VERSION_1 {
                foo:
            }
        """))
        parser = symbolfile.SymbolFileParser(input_file, {}, self.filter)
        parser.next_line()
        with self.assertRaises(symbolfile.ParseError):
            parser.parse_version()

    def test_parse_symbol(self) -> None:
        input_file = io.StringIO(textwrap.dedent("""\
            foo;
            bar; # llndk apex
        """))
        parser = symbolfile.SymbolFileParser(input_file, {}, self.filter)

        parser.next_line()
        symbol = parser.parse_symbol()
        self.assertEqual('foo', symbol.name)
        self.assertEqual(Tags(), symbol.tags)

        parser.next_line()
        symbol = parser.parse_symbol()
        self.assertEqual('bar', symbol.name)
        self.assertEqual(Tags.from_strs(['llndk', 'apex']), symbol.tags)

    def test_wildcard_symbol_global(self) -> None:
        input_file = io.StringIO(textwrap.dedent("""\
            VERSION_1 {
                *;
            };
        """))
        parser = symbolfile.SymbolFileParser(input_file, {}, self.filter)
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
        parser = symbolfile.SymbolFileParser(input_file, {}, self.filter)
        parser.next_line()
        version = parser.parse_version()
        self.assertEqual([], version.symbols)

    def test_missing_semicolon(self) -> None:
        input_file = io.StringIO(textwrap.dedent("""\
            VERSION_1 {
                foo
            };
        """))
        parser = symbolfile.SymbolFileParser(input_file, {}, self.filter)
        parser.next_line()
        with self.assertRaises(symbolfile.ParseError):
            parser.parse_version()

    def test_parse_fails_invalid_input(self) -> None:
        with self.assertRaises(symbolfile.ParseError):
            input_file = io.StringIO('foo')
            parser = symbolfile.SymbolFileParser(input_file, {}, self.filter)
            parser.parse()

    def test_parse(self) -> None:
        input_file = io.StringIO(textwrap.dedent("""\
            VERSION_1 {
                local:
                    hidden1;
                global:
                    foo;
                    bar; # llndk
            };

            VERSION_2 { # weak
                # Implicit global scope.
                    woodly;
                    doodly; # llndk
                local:
                    qwerty;
            } VERSION_1;
        """))
        parser = symbolfile.SymbolFileParser(input_file, {}, self.filter)
        versions = parser.parse()

        expected = [
            symbolfile.Version('VERSION_1', None, Tags(), [
                Symbol('foo', Tags()),
                Symbol('bar', Tags.from_strs(['llndk'])),
            ]),
            symbolfile.Version(
                'VERSION_2', 'VERSION_1', Tags.from_strs(['weak']), [
                    Symbol('woodly', Tags()),
                    Symbol('doodly', Tags.from_strs(['llndk'])),
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
        f = copy(self.filter)
        f.llndk = True
        parser = symbolfile.SymbolFileParser(input_file, {}, f)

        parser.next_line()
        version = parser.parse_version()
        self.assertEqual('VERSION_1', version.name)
        self.assertIsNone(version.base)

        expected_symbols = [
            Symbol('foo', Tags()),
            Symbol('bar', Tags.from_strs(['llndk'])),
            Symbol('baz', Tags.from_strs(['llndk', 'apex'])),
            Symbol('qux', Tags.from_strs(['apex'])),
        ]
        self.assertEqual(expected_symbols, version.symbols)


def main() -> None:
    suite = unittest.TestLoader().loadTestsFromName(__name__)
    unittest.TextTestRunner(verbosity=3).run(suite)


if __name__ == '__main__':
    main()
