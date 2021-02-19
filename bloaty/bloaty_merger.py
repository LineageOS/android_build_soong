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
"""Bloaty CSV Merger

Merges a list of .csv files from Bloaty into a protobuf.  It takes the list as
a first argument and the output as second. For instance:

    $ bloaty_merger binary_sizes.lst binary_sizes.pb

"""

import argparse
import csv

import ninja_rsp

import file_sections_pb2

BLOATY_EXTENSION = ".bloaty.csv"

def parse_csv(path):
  """Parses a Bloaty-generated CSV file into a protobuf.

  Args:
    path: The filepath to the CSV file, relative to $ANDROID_TOP.

  Returns:
    A file_sections_pb2.File if the file was found; None otherwise.
  """
  file_proto = None
  with open(path, newline='') as csv_file:
    file_proto = file_sections_pb2.File()
    if path.endswith(BLOATY_EXTENSION):
      file_proto.path = path[:-len(BLOATY_EXTENSION)]
    section_reader = csv.DictReader(csv_file)
    for row in section_reader:
      section = file_proto.sections.add()
      section.name = row["sections"]
      section.vm_size = int(row["vmsize"])
      section.file_size = int(row["filesize"])
  return file_proto

def create_file_size_metrics(input_list, output_proto):
  """Creates a FileSizeMetrics proto from a list of CSV files.

  Args:
    input_list: The path to the file which contains the list of CSV files. Each
        filepath is separated by a space.
    output_proto: The path for the output protobuf.
  """
  metrics = file_sections_pb2.FileSizeMetrics()
  reader = ninja_rsp.NinjaRspFileReader(input_list)
  for csv_path in reader:
    file_proto = parse_csv(csv_path)
    if file_proto:
      metrics.files.append(file_proto)
  with open(output_proto, "wb") as output:
    output.write(metrics.SerializeToString())

def main():
  parser = argparse.ArgumentParser()
  parser.add_argument("input_list_file", help="List of bloaty csv files.")
  parser.add_argument("output_proto", help="Output proto.")
  args = parser.parse_args()
  create_file_size_metrics(args.input_list_file, args.output_proto)

if __name__ == '__main__':
  main()
