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

from verify_overlaps import *

class TestDetectOverlaps(unittest.TestCase):

    def read_signature_csv_from_string_as_dict(self, csv):
        with io.StringIO(csv) as f:
            return read_signature_csv_from_stream_as_dict(f)

    def extract_subset_from_monolithic_flags_as_dict_from_string(self, monolithic, patterns):
        with io.StringIO(patterns) as f:
            return extract_subset_from_monolithic_flags_as_dict_from_stream(monolithic, f)

    extractInput = '''
Ljava/lang/Object;->hashCode()I,public-api,system-api,test-api
Ljava/lang/Object;->toString()Ljava/lang/String;,blocked
'''

    def test_extract_subset(self):
        monolithic = self.read_signature_csv_from_string_as_dict(TestDetectOverlaps.extractInput)
        modular = self.read_signature_csv_from_string_as_dict('''
Ljava/lang/Object;->hashCode()I,public-api,system-api,test-api
''')

        patterns = 'Ljava/lang/Object;->hashCode()I'

        subset = self.extract_subset_from_monolithic_flags_as_dict_from_string(monolithic, patterns)
        expected = {
            'Ljava/lang/Object;->hashCode()I': {
                None: ['public-api', 'system-api', 'test-api'],
                'signature': 'Ljava/lang/Object;->hashCode()I',
            },
        }
        self.assertEqual(expected, subset)

    def test_match(self):
        monolithic = self.read_signature_csv_from_string_as_dict('''
Ljava/lang/Object;->hashCode()I,public-api,system-api,test-api
''')
        modular = self.read_signature_csv_from_string_as_dict('''
Ljava/lang/Object;->hashCode()I,public-api,system-api,test-api
''')
        mismatches = compare_signature_flags(monolithic, modular)
        expected = []
        self.assertEqual(expected, mismatches)

    def test_mismatch_overlapping_flags(self):
        monolithic = self.read_signature_csv_from_string_as_dict('''
Ljava/lang/Object;->toString()Ljava/lang/String;,public-api
''')
        modular = self.read_signature_csv_from_string_as_dict('''
Ljava/lang/Object;->toString()Ljava/lang/String;,public-api,system-api,test-api
''')
        mismatches = compare_signature_flags(monolithic, modular)
        expected = [
            (
                'Ljava/lang/Object;->toString()Ljava/lang/String;',
                ['public-api', 'system-api', 'test-api'],
                ['public-api'],
            ),
        ]
        self.assertEqual(expected, mismatches)


    def test_mismatch_monolithic_blocked(self):
        monolithic = self.read_signature_csv_from_string_as_dict('''
Ljava/lang/Object;->toString()Ljava/lang/String;,blocked
''')
        modular = self.read_signature_csv_from_string_as_dict('''
Ljava/lang/Object;->toString()Ljava/lang/String;,public-api,system-api,test-api
''')
        mismatches = compare_signature_flags(monolithic, modular)
        expected = [
            (
                'Ljava/lang/Object;->toString()Ljava/lang/String;',
                ['public-api', 'system-api', 'test-api'],
                ['blocked'],
            ),
        ]
        self.assertEqual(expected, mismatches)

    def test_mismatch_modular_blocked(self):
        monolithic = self.read_signature_csv_from_string_as_dict('''
Ljava/lang/Object;->toString()Ljava/lang/String;,public-api,system-api,test-api
''')
        modular = self.read_signature_csv_from_string_as_dict('''
Ljava/lang/Object;->toString()Ljava/lang/String;,blocked
''')
        mismatches = compare_signature_flags(monolithic, modular)
        expected = [
            (
                'Ljava/lang/Object;->toString()Ljava/lang/String;',
                ['blocked'],
                ['public-api', 'system-api', 'test-api'],
            ),
        ]
        self.assertEqual(expected, mismatches)

    def test_missing_from_monolithic(self):
        monolithic = self.read_signature_csv_from_string_as_dict('')
        modular = self.read_signature_csv_from_string_as_dict('''
Ljava/lang/Object;->toString()Ljava/lang/String;,public-api,system-api,test-api
''')
        mismatches = compare_signature_flags(monolithic, modular)
        expected = [
            (
                'Ljava/lang/Object;->toString()Ljava/lang/String;',
                ['public-api', 'system-api', 'test-api'],
                [],
            ),
        ]
        self.assertEqual(expected, mismatches)

    def test_missing_from_modular(self):
        monolithic = self.read_signature_csv_from_string_as_dict('''
Ljava/lang/Object;->hashCode()I,public-api,system-api,test-api
''')
        modular = {}
        mismatches = compare_signature_flags(monolithic, modular)
        expected = [
            (
                'Ljava/lang/Object;->hashCode()I',
                [],
                ['public-api', 'system-api', 'test-api'],
            ),
        ]
        self.assertEqual(expected, mismatches)

    def test_blocked_missing_from_modular(self):
        monolithic = self.read_signature_csv_from_string_as_dict('''
Ljava/lang/Object;->hashCode()I,blocked
''')
        modular = {}
        mismatches = compare_signature_flags(monolithic, modular)
        expected = [
            (
                'Ljava/lang/Object;->hashCode()I',
                [],
                ['blocked'],
            ),
        ]
        self.assertEqual(expected, mismatches)

if __name__ == '__main__':
    unittest.main(verbosity=2)
