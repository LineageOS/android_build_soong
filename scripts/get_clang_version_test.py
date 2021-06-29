#!/usr/bin/env python
#
# Copyright (C) 2021 The Android Open Source Project
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
"""Unit tests for get_clang_version.py."""

import unittest

import get_clang_version

class GetClangVersionTest(unittest.TestCase):
  """Unit tests for get_clang_version."""

  def test_get_clang_version(self):
    """Test parsing of clang prebuilts version."""
    self.assertIsNotNone(get_clang_version.get_clang_prebuilts_version())


if __name__ == '__main__':
  unittest.main(verbosity=2)
