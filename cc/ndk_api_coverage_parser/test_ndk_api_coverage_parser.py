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
"""Tests for ndk_api_coverage_parser.py."""
import io
import textwrap
import unittest

from xml.etree.ElementTree import fromstring
from symbolfile import FUTURE_API_LEVEL, SymbolFileParser
import ndk_api_coverage_parser as nparser


# pylint: disable=missing-docstring


# https://stackoverflow.com/a/24349916/632035
def etree_equal(elem1, elem2):
    """Returns true if the two XML elements are equal.

    xml.etree.ElementTree's comparison operator cares about the ordering of
    elements and attributes, but they are stored in an unordered dict so the
    ordering is not deterministic.

    lxml is apparently API compatible with xml and does use an OrderedDict, but
    we don't have it in the tree.
    """
    if elem1.tag != elem2.tag:
        return False
    if elem1.text != elem2.text:
        return False
    if elem1.tail != elem2.tail:
        return False
    if elem1.attrib != elem2.attrib:
        return False
    if len(elem1) != len(elem2):
        return False
    return all(etree_equal(c1, c2) for c1, c2 in zip(elem1, elem2))


class ApiCoverageSymbolFileParserTest(unittest.TestCase):
    def test_parse(self):
        input_file = io.StringIO(textwrap.dedent(u"""\
            LIBLOG { # introduced-arm64=24 introduced-x86=24 introduced-x86_64=24
              global:
                android_name_to_log_id; # apex llndk introduced=23
                android_log_id_to_name; # llndk arm
                __android_log_assert; # introduced-x86=23
                __android_log_buf_print; # var
                __android_log_buf_write;
              local:
                *;
            };
            
            LIBLOG_PLATFORM {
                android_fdtrack; # llndk
                android_net; # introduced=23
            };
            
            LIBLOG_FOO { # var
                android_var;
            };
        """))
        parser = SymbolFileParser(input_file, {}, "", FUTURE_API_LEVEL, True, True)
        generator = nparser.XmlGenerator(io.StringIO())
        result = generator.convertToXml(parser.parse())
        expected = fromstring('<ndk-library><symbol apex="True" arch="" introduced="23" introduced-arm64="24" introduced-x86="24" introduced-x86_64="24" is_deprecated="False" is_platform="False" llndk="True" name="android_name_to_log_id" /><symbol arch="arm" introduced-arm64="24" introduced-x86="24" introduced-x86_64="24" is_deprecated="False" is_platform="False" llndk="True" name="android_log_id_to_name" /><symbol arch="" introduced-arm64="24" introduced-x86="23" introduced-x86_64="24" is_deprecated="False" is_platform="False" name="__android_log_assert" /><symbol arch="" introduced-arm64="24" introduced-x86="24" introduced-x86_64="24" is_deprecated="False" is_platform="False" name="__android_log_buf_write" /><symbol arch="" is_deprecated="False" is_platform="True" llndk="True" name="android_fdtrack" /><symbol arch="" introduced="23" is_deprecated="False" is_platform="True" name="android_net" /></ndk-library>')
        self.assertTrue(etree_equal(expected, result))


def main():
    suite = unittest.TestLoader().loadTestsFromName(__name__)
    unittest.TextTestRunner(verbosity=3).run(suite)


if __name__ == '__main__':
    main()
