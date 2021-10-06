#!/usr/bin/env python
#
# Copyright (C) 2021 The Android Open Source Project
#
# Licensed under the Apache License, Version 2.0 (the 'License');
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an 'AS IS' BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
"""Unit tests for signature_patterns.py."""
import io
import unittest

from signature_patterns import *  #pylint: disable=unused-wildcard-import,wildcard-import


class TestGeneratedPatterns(unittest.TestCase):

    csvFlags = """
Ljava/lang/ProcessBuilder$Redirect$1;-><init>()V,blocked
Ljava/lang/Character$UnicodeScript;->of(I)Ljava/lang/Character$UnicodeScript;,public-api
Ljava/lang/Object;->hashCode()I,public-api,system-api,test-api
Ljava/lang/Object;->toString()Ljava/lang/String;,blocked
"""

    def produce_patterns_from_string(self,
                                     csv,
                                     splitPackages=None,
                                     packagePrefixes=None):
        with io.StringIO(csv) as f:
            return produce_patterns_from_stream(f, splitPackages,
                                                packagePrefixes)

    def test_generate_default(self):
        patterns = self.produce_patterns_from_string(
            TestGeneratedPatterns.csvFlags)
        expected = [
            'java/lang/*',
        ]
        self.assertEqual(expected, patterns)

    def test_generate_split_package(self):
        patterns = self.produce_patterns_from_string(
            TestGeneratedPatterns.csvFlags, splitPackages={'java/lang'})
        expected = [
            'java/lang/Character',
            'java/lang/Object',
            'java/lang/ProcessBuilder',
        ]
        self.assertEqual(expected, patterns)

    def test_generate_split_package_wildcard(self):
        patterns = self.produce_patterns_from_string(
            TestGeneratedPatterns.csvFlags, splitPackages={'*'})
        expected = [
            'java/lang/Character',
            'java/lang/Object',
            'java/lang/ProcessBuilder',
        ]
        self.assertEqual(expected, patterns)

    def test_generate_package_prefix(self):
        patterns = self.produce_patterns_from_string(
            TestGeneratedPatterns.csvFlags, packagePrefixes={'java/lang'})
        expected = [
            'java/lang/**',
        ]
        self.assertEqual(expected, patterns)

    def test_generate_package_prefix_top_package(self):
        patterns = self.produce_patterns_from_string(
            TestGeneratedPatterns.csvFlags, packagePrefixes={'java'})
        expected = [
            'java/**',
        ]
        self.assertEqual(expected, patterns)

    def test_split_package_wildcard_conflicts_with_other_split_packages(self):
        errors = validate_split_packages({'*', 'java'})
        expected = [
            'split packages are invalid as they contain both the wildcard (*)'
            ' and specific packages, use the wildcard or specific packages,'
            ' not a mixture'
        ]
        self.assertEqual(expected, errors)

    def test_split_package_wildcard_conflicts_with_package_prefixes(self):
        errors = validate_package_prefixes({'*'}, packagePrefixes={'java'})
        expected = [
            'split package "*" conflicts with all package prefixes java\n'
            '    add split_packages:[] to fix',
        ]
        self.assertEqual(expected, errors)

    def test_split_package_conflict(self):
        errors = validate_package_prefixes({'java/split'},
                                           packagePrefixes={'java'})
        expected = [
            'split package java.split is matched by package prefix java',
        ]
        self.assertEqual(expected, errors)


if __name__ == '__main__':
    unittest.main(verbosity=2)
