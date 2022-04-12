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
"""Unit tests for verify_overlaps_test.py."""
import io
import unittest

import verify_overlaps as vo


class TestDetectOverlaps(unittest.TestCase):

    @staticmethod
    def read_flag_trie_from_string(csvdata):
        with io.StringIO(csvdata) as f:
            return vo.read_flag_trie_from_stream(f)

    @staticmethod
    def read_signature_csv_from_string_as_dict(csvdata):
        with io.StringIO(csvdata) as f:
            return vo.read_signature_csv_from_stream_as_dict(f)

    @staticmethod
    def extract_subset_from_monolithic_flags_as_dict_from_string(
            monolithic, patterns):
        with io.StringIO(patterns) as f:
            return vo.extract_subset_from_monolithic_flags_as_dict_from_stream(
                monolithic, f)

    extractInput = """
Ljava/lang/Object;->hashCode()I,public-api,system-api,test-api
Ljava/lang/Object;->toString()Ljava/lang/String;,blocked
Ljava/util/zip/ZipFile;-><clinit>()V,blocked
Ljava/lang/Character$UnicodeScript;->of(I)Ljava/lang/Character$UnicodeScript;,blocked
Ljava/lang/Character;->serialVersionUID:J,sdk
Ljava/lang/ProcessBuilder$Redirect$1;-><init>()V,blocked
"""

    def test_extract_subset_signature(self):
        monolithic = self.read_flag_trie_from_string(
            TestDetectOverlaps.extractInput)

        patterns = "Ljava/lang/Object;->hashCode()I"

        subset = self.extract_subset_from_monolithic_flags_as_dict_from_string(
            monolithic, patterns)
        expected = {
            "Ljava/lang/Object;->hashCode()I": {
                None: ["public-api", "system-api", "test-api"],
                "signature": "Ljava/lang/Object;->hashCode()I",
            },
        }
        self.assertEqual(expected, subset)

    def test_extract_subset_class(self):
        monolithic = self.read_flag_trie_from_string(
            TestDetectOverlaps.extractInput)

        patterns = "java/lang/Object"

        subset = self.extract_subset_from_monolithic_flags_as_dict_from_string(
            monolithic, patterns)
        expected = {
            "Ljava/lang/Object;->hashCode()I": {
                None: ["public-api", "system-api", "test-api"],
                "signature": "Ljava/lang/Object;->hashCode()I",
            },
            "Ljava/lang/Object;->toString()Ljava/lang/String;": {
                None: ["blocked"],
                "signature": "Ljava/lang/Object;->toString()Ljava/lang/String;",
            },
        }
        self.assertEqual(expected, subset)

    def test_extract_subset_outer_class(self):
        monolithic = self.read_flag_trie_from_string(
            TestDetectOverlaps.extractInput)

        patterns = "java/lang/Character"

        subset = self.extract_subset_from_monolithic_flags_as_dict_from_string(
            monolithic, patterns)
        expected = {
            "Ljava/lang/Character$UnicodeScript;"
            "->of(I)Ljava/lang/Character$UnicodeScript;": {
                None: ["blocked"],
                "signature": "Ljava/lang/Character$UnicodeScript;"
                             "->of(I)Ljava/lang/Character$UnicodeScript;",
            },
            "Ljava/lang/Character;->serialVersionUID:J": {
                None: ["sdk"],
                "signature": "Ljava/lang/Character;->serialVersionUID:J",
            },
        }
        self.assertEqual(expected, subset)

    def test_extract_subset_nested_class(self):
        monolithic = self.read_flag_trie_from_string(
            TestDetectOverlaps.extractInput)

        patterns = "java/lang/Character$UnicodeScript"

        subset = self.extract_subset_from_monolithic_flags_as_dict_from_string(
            monolithic, patterns)
        expected = {
            "Ljava/lang/Character$UnicodeScript;"
            "->of(I)Ljava/lang/Character$UnicodeScript;": {
                None: ["blocked"],
                "signature": "Ljava/lang/Character$UnicodeScript;"
                             "->of(I)Ljava/lang/Character$UnicodeScript;",
            },
        }
        self.assertEqual(expected, subset)

    def test_extract_subset_package(self):
        monolithic = self.read_flag_trie_from_string(
            TestDetectOverlaps.extractInput)

        patterns = "java/lang/*"

        subset = self.extract_subset_from_monolithic_flags_as_dict_from_string(
            monolithic, patterns)
        expected = {
            "Ljava/lang/Character$UnicodeScript;"
            "->of(I)Ljava/lang/Character$UnicodeScript;": {
                None: ["blocked"],
                "signature": "Ljava/lang/Character$UnicodeScript;"
                             "->of(I)Ljava/lang/Character$UnicodeScript;",
            },
            "Ljava/lang/Character;->serialVersionUID:J": {
                None: ["sdk"],
                "signature": "Ljava/lang/Character;->serialVersionUID:J",
            },
            "Ljava/lang/Object;->hashCode()I": {
                None: ["public-api", "system-api", "test-api"],
                "signature": "Ljava/lang/Object;->hashCode()I",
            },
            "Ljava/lang/Object;->toString()Ljava/lang/String;": {
                None: ["blocked"],
                "signature": "Ljava/lang/Object;->toString()Ljava/lang/String;",
            },
            "Ljava/lang/ProcessBuilder$Redirect$1;-><init>()V": {
                None: ["blocked"],
                "signature": "Ljava/lang/ProcessBuilder$Redirect$1;-><init>()V",
            },
        }
        self.assertEqual(expected, subset)

    def test_extract_subset_recursive_package(self):
        monolithic = self.read_flag_trie_from_string(
            TestDetectOverlaps.extractInput)

        patterns = "java/**"

        subset = self.extract_subset_from_monolithic_flags_as_dict_from_string(
            monolithic, patterns)
        expected = {
            "Ljava/lang/Character$UnicodeScript;"
            "->of(I)Ljava/lang/Character$UnicodeScript;": {
                None: ["blocked"],
                "signature": "Ljava/lang/Character$UnicodeScript;"
                             "->of(I)Ljava/lang/Character$UnicodeScript;",
            },
            "Ljava/lang/Character;->serialVersionUID:J": {
                None: ["sdk"],
                "signature": "Ljava/lang/Character;->serialVersionUID:J",
            },
            "Ljava/lang/Object;->hashCode()I": {
                None: ["public-api", "system-api", "test-api"],
                "signature": "Ljava/lang/Object;->hashCode()I",
            },
            "Ljava/lang/Object;->toString()Ljava/lang/String;": {
                None: ["blocked"],
                "signature": "Ljava/lang/Object;->toString()Ljava/lang/String;",
            },
            "Ljava/lang/ProcessBuilder$Redirect$1;-><init>()V": {
                None: ["blocked"],
                "signature": "Ljava/lang/ProcessBuilder$Redirect$1;-><init>()V",
            },
            "Ljava/util/zip/ZipFile;-><clinit>()V": {
                None: ["blocked"],
                "signature": "Ljava/util/zip/ZipFile;-><clinit>()V",
            },
        }
        self.assertEqual(expected, subset)

    def test_read_trie_duplicate(self):
        with self.assertRaises(Exception) as context:
            self.read_flag_trie_from_string("""
Ljava/lang/Object;->hashCode()I,public-api,system-api,test-api
Ljava/lang/Object;->hashCode()I,blocked
""")
        self.assertTrue("Duplicate signature: Ljava/lang/Object;->hashCode()I"
                        in str(context.exception))

    def test_read_trie_missing_member(self):
        with self.assertRaises(Exception) as context:
            self.read_flag_trie_from_string("""
Ljava/lang/Object,public-api,system-api,test-api
""")
        self.assertTrue(
            "Invalid signature: Ljava/lang/Object, "
            "does not identify a specific member" in str(context.exception))

    def test_match(self):
        monolithic = self.read_signature_csv_from_string_as_dict("""
Ljava/lang/Object;->hashCode()I,public-api,system-api,test-api
""")
        modular = self.read_signature_csv_from_string_as_dict("""
Ljava/lang/Object;->hashCode()I,public-api,system-api,test-api
""")
        mismatches = vo.compare_signature_flags(monolithic, modular,
                                                ["blocked"])
        expected = []
        self.assertEqual(expected, mismatches)

    def test_mismatch_overlapping_flags(self):
        monolithic = self.read_signature_csv_from_string_as_dict("""
Ljava/lang/Object;->toString()Ljava/lang/String;,public-api
""")
        modular = self.read_signature_csv_from_string_as_dict("""
Ljava/lang/Object;->toString()Ljava/lang/String;,public-api,system-api,test-api
""")
        mismatches = vo.compare_signature_flags(monolithic, modular,
                                                ["blocked"])
        expected = [
            (
                "Ljava/lang/Object;->toString()Ljava/lang/String;",
                ["public-api", "system-api", "test-api"],
                ["public-api"],
            ),
        ]
        self.assertEqual(expected, mismatches)

    def test_mismatch_monolithic_blocked(self):
        monolithic = self.read_signature_csv_from_string_as_dict("""
Ljava/lang/Object;->toString()Ljava/lang/String;,blocked
""")
        modular = self.read_signature_csv_from_string_as_dict("""
Ljava/lang/Object;->toString()Ljava/lang/String;,public-api,system-api,test-api
""")
        mismatches = vo.compare_signature_flags(monolithic, modular,
                                                ["blocked"])
        expected = [
            (
                "Ljava/lang/Object;->toString()Ljava/lang/String;",
                ["public-api", "system-api", "test-api"],
                ["blocked"],
            ),
        ]
        self.assertEqual(expected, mismatches)

    def test_mismatch_modular_blocked(self):
        monolithic = self.read_signature_csv_from_string_as_dict("""
Ljava/lang/Object;->toString()Ljava/lang/String;,public-api,system-api,test-api
""")
        modular = self.read_signature_csv_from_string_as_dict("""
Ljava/lang/Object;->toString()Ljava/lang/String;,blocked
""")
        mismatches = vo.compare_signature_flags(monolithic, modular,
                                                ["blocked"])
        expected = [
            (
                "Ljava/lang/Object;->toString()Ljava/lang/String;",
                ["blocked"],
                ["public-api", "system-api", "test-api"],
            ),
        ]
        self.assertEqual(expected, mismatches)

    def test_match_treat_missing_from_modular_as_blocked(self):
        monolithic = self.read_signature_csv_from_string_as_dict("")
        modular = self.read_signature_csv_from_string_as_dict("""
Ljava/lang/Object;->toString()Ljava/lang/String;,public-api,system-api,test-api
""")
        mismatches = vo.compare_signature_flags(monolithic, modular,
                                                ["blocked"])
        expected = [
            (
                "Ljava/lang/Object;->toString()Ljava/lang/String;",
                ["public-api", "system-api", "test-api"],
                [],
            ),
        ]
        self.assertEqual(expected, mismatches)

    def test_mismatch_treat_missing_from_modular_as_blocked(self):
        monolithic = self.read_signature_csv_from_string_as_dict("""
Ljava/lang/Object;->hashCode()I,public-api,system-api,test-api
""")
        modular = {}
        mismatches = vo.compare_signature_flags(monolithic, modular,
                                                ["blocked"])
        expected = [
            (
                "Ljava/lang/Object;->hashCode()I",
                ["blocked"],
                ["public-api", "system-api", "test-api"],
            ),
        ]
        self.assertEqual(expected, mismatches)

    def test_blocked_missing_from_modular(self):
        monolithic = self.read_signature_csv_from_string_as_dict("""
Ljava/lang/Object;->hashCode()I,blocked
""")
        modular = {}
        mismatches = vo.compare_signature_flags(monolithic, modular,
                                                ["blocked"])
        expected = []
        self.assertEqual(expected, mismatches)

    def test_match_treat_missing_from_modular_as_empty(self):
        monolithic = self.read_signature_csv_from_string_as_dict("")
        modular = self.read_signature_csv_from_string_as_dict("""
Ljava/lang/Object;->toString()Ljava/lang/String;,public-api,system-api,test-api
""")
        mismatches = vo.compare_signature_flags(monolithic, modular, [])
        expected = [
            (
                "Ljava/lang/Object;->toString()Ljava/lang/String;",
                ["public-api", "system-api", "test-api"],
                [],
            ),
        ]
        self.assertEqual(expected, mismatches)

    def test_mismatch_treat_missing_from_modular_as_empty(self):
        monolithic = self.read_signature_csv_from_string_as_dict("""
Ljava/lang/Object;->hashCode()I,public-api,system-api,test-api
""")
        modular = {}
        mismatches = vo.compare_signature_flags(monolithic, modular, [])
        expected = [
            (
                "Ljava/lang/Object;->hashCode()I",
                [],
                ["public-api", "system-api", "test-api"],
            ),
        ]
        self.assertEqual(expected, mismatches)

    def test_empty_missing_from_modular(self):
        monolithic = self.read_signature_csv_from_string_as_dict("""
Ljava/lang/Object;->hashCode()I
""")
        modular = {}
        mismatches = vo.compare_signature_flags(monolithic, modular, [])
        expected = []
        self.assertEqual(expected, mismatches)


if __name__ == "__main__":
    unittest.main(verbosity=2)
