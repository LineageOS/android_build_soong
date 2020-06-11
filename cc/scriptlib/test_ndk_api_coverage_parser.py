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

from xml.etree.ElementTree import tostring
from gen_stub_libs import FUTURE_API_LEVEL, SymbolFileParser
import ndk_api_coverage_parser as nparser


# pylint: disable=missing-docstring


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
        result = tostring(generator.convertToXml(parser.parse())).decode()
        expected = '<ndk-library><symbol apex="True" arch="" introduced="23" introduced-arm64="24" introduced-x86="24" introduced-x86_64="24" is_deprecated="False" is_platform="False" llndk="True" name="android_name_to_log_id" /><symbol arch="arm" introduced-arm64="24" introduced-x86="24" introduced-x86_64="24" is_deprecated="False" is_platform="False" llndk="True" name="android_log_id_to_name" /><symbol arch="" introduced-arm64="24" introduced-x86="23" introduced-x86_64="24" is_deprecated="False" is_platform="False" name="__android_log_assert" /><symbol arch="" introduced-arm64="24" introduced-x86="24" introduced-x86_64="24" is_deprecated="False" is_platform="False" name="__android_log_buf_write" /><symbol arch="" is_deprecated="False" is_platform="True" llndk="True" name="android_fdtrack" /><symbol arch="" introduced="23" is_deprecated="False" is_platform="True" name="android_net" /></ndk-library>'
        self.assertEqual(expected, result)


def main():
    suite = unittest.TestLoader().loadTestsFromName(__name__)
    unittest.TextTestRunner(verbosity=3).run(suite)


if __name__ == '__main__':
    main()
