# Copyright 2021 Google Inc. All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
import unittest

from pyfakefs import fake_filesystem_unittest

import bloaty_merger
import file_sections_pb2


class BloatyMergerTestCase(fake_filesystem_unittest.TestCase):
  def setUp(self):
    self.setUpPyfakefs()

  def test_parse_csv(self):
    csv_content = "sections,vmsize,filesize\nsection1,2,3\n"
    self.fs.create_file("file1.bloaty.csv", contents=csv_content)
    pb = bloaty_merger.parse_csv("file1.bloaty.csv")
    self.assertEqual(pb.path, "file1")
    self.assertEqual(len(pb.sections), 1)
    s = pb.sections[0]
    self.assertEqual(s.name, "section1")
    self.assertEqual(s.vm_size, 2)
    self.assertEqual(s.file_size, 3)

  def test_missing_file(self):
    with self.assertRaises(FileNotFoundError):
      bloaty_merger.parse_csv("missing.bloaty.csv")

  def test_malformed_csv(self):
    csv_content = "header1,heaVder2,header3\n4,5,6\n"
    self.fs.create_file("file1.bloaty.csv", contents=csv_content)
    with self.assertRaises(KeyError):
      bloaty_merger.parse_csv("file1.bloaty.csv")

  def test_create_file_metrics(self):
    file_list = "file1.bloaty.csv file2.bloaty.csv"
    file1_content = "sections,vmsize,filesize\nsection1,2,3\nsection2,7,8"
    file2_content = "sections,vmsize,filesize\nsection1,4,5\n"

    self.fs.create_file("files.lst", contents=file_list)
    self.fs.create_file("file1.bloaty.csv", contents=file1_content)
    self.fs.create_file("file2.bloaty.csv", contents=file2_content)

    bloaty_merger.create_file_size_metrics("files.lst", "output.pb")

    metrics = file_sections_pb2.FileSizeMetrics()
    with open("output.pb", "rb") as output:
      metrics.ParseFromString(output.read())


if __name__ == '__main__':
  suite = unittest.TestLoader().loadTestsFromTestCase(BloatyMergerTestCase)
  unittest.TextTestRunner(verbosity=2).run(suite)
