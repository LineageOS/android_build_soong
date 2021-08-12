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

from signature_patterns import *

class TestGeneratedPatterns(unittest.TestCase):

    def produce_patterns_from_string(self, csv):
        with io.StringIO(csv) as f:
            return produce_patterns_from_stream(f)

    def test_generate(self):
        patterns = self.produce_patterns_from_string('''
Ljava/lang/ProcessBuilder$Redirect$1;-><init>()V,blocked
Ljava/lang/Character$UnicodeScript;->of(I)Ljava/lang/Character$UnicodeScript;,public-api
Ljava/lang/Object;->hashCode()I,public-api,system-api,test-api
Ljava/lang/Object;->toString()Ljava/lang/String;,blocked
''')
        expected = [
            "java/lang/Character",
            "java/lang/Object",
            "java/lang/ProcessBuilder",
        ]
        self.assertEqual(expected, patterns)

if __name__ == '__main__':
    unittest.main(verbosity=2)
