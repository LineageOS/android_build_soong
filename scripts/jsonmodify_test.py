#!/usr/bin/env python
#
# Copyright (C) 2022 The Android Open Source Project
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
"""Tests for jsonmodify."""

import json
import jsonmodify
import unittest


class JsonmodifyTest(unittest.TestCase):

  def test_set_value(self):
    obj = json.loads('{"field1": 111}')
    field1 = jsonmodify.SetValue("field1")
    field1.apply(obj, 222)
    field2 = jsonmodify.SetValue("field2")
    field2.apply(obj, 333)
    expected = json.loads('{"field1": 222, "field2": 333}')
    self.assertEqual(obj, expected)

  def test_replace(self):
    obj = json.loads('{"field1": 111}')
    field1 = jsonmodify.Replace("field1")
    field1.apply(obj, 222)
    field2 = jsonmodify.Replace("field2")
    field2.apply(obj, 333)
    expected = json.loads('{"field1": 222}')
    self.assertEqual(obj, expected)

  def test_replace_if_equal(self):
    obj = json.loads('{"field1": 111, "field2": 222}')
    field1 = jsonmodify.ReplaceIfEqual("field1")
    field1.apply(obj, 111, 333)
    field2 = jsonmodify.ReplaceIfEqual("field2")
    field2.apply(obj, 444, 555)
    field3 = jsonmodify.ReplaceIfEqual("field3")
    field3.apply(obj, 666, 777)
    expected = json.loads('{"field1": 333, "field2": 222}')
    self.assertEqual(obj, expected)

  def test_remove(self):
    obj = json.loads('{"field1": 111, "field2": 222}')
    field2 = jsonmodify.Remove("field2")
    field2.apply(obj)
    field3 = jsonmodify.Remove("field3")
    field3.apply(obj)
    expected = json.loads('{"field1": 111}')
    self.assertEqual(obj, expected)

  def test_append_list(self):
    obj = json.loads('{"field1": [111]}')
    field1 = jsonmodify.AppendList("field1")
    field1.apply(obj, 222, 333)
    field2 = jsonmodify.AppendList("field2")
    field2.apply(obj, 444, 555, 666)
    expected = json.loads('{"field1": [111, 222, 333], "field2": [444, 555, 666]}')
    self.assertEqual(obj, expected)


if __name__ == '__main__':
    unittest.main(verbosity=2)
