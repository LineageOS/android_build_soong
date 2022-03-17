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

import signature_patterns


class TestGeneratedPatterns(unittest.TestCase):

    csv_flags = """
Ljava/lang/ProcessBuilder$Redirect$1;-><init>()V,blocked
Ljava/lang/Character$UnicodeScript;->of(I)Ljava/lang/Character$UnicodeScript;,public-api
Ljava/lang/Object;->hashCode()I,public-api,system-api,test-api
Ljava/lang/Object;->toString()Ljava/lang/String;,blocked
"""

    @staticmethod
    def produce_patterns_from_string(csv_text,
                                     split_packages=None,
                                     package_prefixes=None):
        with io.StringIO(csv_text) as f:
            return signature_patterns.produce_patterns_from_stream(
                f, split_packages, package_prefixes)

    def test_generate_default(self):
        patterns = self.produce_patterns_from_string(
            TestGeneratedPatterns.csv_flags)
        expected = [
            'java/lang/*',
        ]
        self.assertEqual(expected, patterns)

    def test_generate_split_package(self):
        patterns = self.produce_patterns_from_string(
            TestGeneratedPatterns.csv_flags, split_packages={'java/lang'})
        expected = [
            'java/lang/Character',
            'java/lang/Object',
            'java/lang/ProcessBuilder',
        ]
        self.assertEqual(expected, patterns)

    def test_generate_split_package_wildcard(self):
        patterns = self.produce_patterns_from_string(
            TestGeneratedPatterns.csv_flags, split_packages={'*'})
        expected = [
            'java/lang/Character',
            'java/lang/Object',
            'java/lang/ProcessBuilder',
        ]
        self.assertEqual(expected, patterns)

    def test_generate_package_prefix(self):
        patterns = self.produce_patterns_from_string(
            TestGeneratedPatterns.csv_flags, package_prefixes={'java/lang'})
        expected = [
            'java/lang/**',
        ]
        self.assertEqual(expected, patterns)

    def test_generate_package_prefix_top_package(self):
        patterns = self.produce_patterns_from_string(
            TestGeneratedPatterns.csv_flags, package_prefixes={'java'})
        expected = [
            'java/**',
        ]
        self.assertEqual(expected, patterns)

    def test_split_package_wildcard_conflicts_with_other_split_packages(self):
        errors = signature_patterns.validate_split_packages({'*', 'java'})
        expected = [
            'split packages are invalid as they contain both the wildcard (*)'
            ' and specific packages, use the wildcard or specific packages,'
            ' not a mixture'
        ]
        self.assertEqual(expected, errors)

    def test_split_package_wildcard_conflicts_with_package_prefixes(self):
        errors = signature_patterns.validate_package_prefixes(
            {'*'}, package_prefixes={'java'})
        expected = [
            "split package '*' conflicts with all package prefixes java\n"
            '    add split_packages:[] to fix',
        ]
        self.assertEqual(expected, errors)

    def test_split_package_conflict(self):
        errors = signature_patterns.validate_package_prefixes(
            {'java/split'}, package_prefixes={'java'})
        expected = [
            'split package java.split is matched by package prefix java',
        ]
        self.assertEqual(expected, errors)


if __name__ == '__main__':
    unittest.main(verbosity=2)
