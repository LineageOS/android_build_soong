#!/usr/bin/env python
#
# Copyright (C) 2024 The Android Open Source Project
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
"""Unit tests for uffd_gc_utils.py."""

import unittest

from uffd_gc_utils import should_enable_uffd_gc_impl


class UffdGcUtilsTest(unittest.TestCase):
  def test_should_enable_uffd_gc_impl(self):
    # GKI kernels in new format.
    self.assertTrue(should_enable_uffd_gc_impl(
        "6.1.25-android14-11-g34fde9ec08a3-ab10675345"))
    self.assertTrue(should_enable_uffd_gc_impl(
        "5.4.42-android12-0-something"))
    self.assertFalse(should_enable_uffd_gc_impl(
        "5.4.42-android11-0-something"))
    # GKI kernels in old format.
    self.assertFalse(should_enable_uffd_gc_impl(
        "4.19.282-g4b749a433956-ab10893502"))
    # Non GKI kernels.
    self.assertTrue(should_enable_uffd_gc_impl(
        "6.1.25-foo"))
    self.assertTrue(should_enable_uffd_gc_impl(
        "6.1.25"))
    self.assertTrue(should_enable_uffd_gc_impl(
        "5.10.19-foo"))
    self.assertTrue(should_enable_uffd_gc_impl(
        "5.10.19"))
    with self.assertRaises(SystemExit):
        should_enable_uffd_gc_impl("5.4.42-foo")
    with self.assertRaises(SystemExit):
        should_enable_uffd_gc_impl("5.4.42")
    self.assertFalse(should_enable_uffd_gc_impl(
        "4.19.282-foo"))
    self.assertFalse(should_enable_uffd_gc_impl(
        "4.19.282"))
    with self.assertRaises(SystemExit):
        should_enable_uffd_gc_impl("foo")
    # No kernel.
    self.assertTrue(should_enable_uffd_gc_impl(
        "<unknown-kernel>"))


if __name__ == '__main__':
  unittest.main(verbosity=2)
