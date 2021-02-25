#!/usr/bin/env python
#
# Copyright (C) 2018 The Android Open Source Project
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
"""
Merge multiple CSV files, possibly with different columns.
"""

import argparse
import csv
import io
import heapq
import itertools
import operator

from zipfile import ZipFile

args_parser = argparse.ArgumentParser(description='Merge given CSV files into a single one.')
args_parser.add_argument('--header', help='Comma separated field names; '
                                          'if missing determines the header from input files.')
args_parser.add_argument('--zip_input', help='Treat files as ZIP archives containing CSV files to merge.',
                         action="store_true")
args_parser.add_argument('--key_field', help='The name of the field by which the rows should be sorted. '
                                             'Must be in the field names. '
                                             'Will be the first field in the output. '
                                             'All input files must be sorted by that field.')
args_parser.add_argument('--output', help='Output file for merged CSV.',
                         default='-', type=argparse.FileType('w'))
args_parser.add_argument('files', nargs=argparse.REMAINDER)
args = args_parser.parse_args()


def dict_reader(input):
    return csv.DictReader(input, delimiter=',', quotechar='|')

csv_readers = []
if not(args.zip_input):
    for file in args.files:
        csv_readers.append(dict_reader(open(file, 'r')))
else:
    for file in args.files:
        with ZipFile(file) as zip:
            for entry in zip.namelist():
                if entry.endswith('.uau'):
                    csv_readers.append(dict_reader(io.TextIOWrapper(zip.open(entry, 'r'))))

headers = set()
if args.header:
    fieldnames = args.header.split(',')
else:
    # Build union of all columns from source files:
    for reader in csv_readers:
        headers = headers.union(reader.fieldnames)
    fieldnames = sorted(headers)

# By default chain the csv readers together so that the resulting output is
# the concatenation of the rows from each of them:
all_rows = itertools.chain.from_iterable(csv_readers)

if len(csv_readers) > 0:
    keyField = args.key_field
    if keyField:
        assert keyField in fieldnames, (
            "--key_field {} not found, must be one of {}\n").format(
            keyField, ",".join(fieldnames))
        # Make the key field the first field in the output
        keyFieldIndex = fieldnames.index(args.key_field)
        fieldnames.insert(0, fieldnames.pop(keyFieldIndex))
        # Create an iterable that performs a lazy merge sort on the csv readers
        # sorting the rows by the key field.
        all_rows = heapq.merge(*csv_readers, key=operator.itemgetter(keyField))

# Write all rows from the input files to the output:
writer = csv.DictWriter(args.output, delimiter=',', quotechar='|', quoting=csv.QUOTE_MINIMAL,
                        dialect='unix', fieldnames=fieldnames)
writer.writeheader()

# Read all the rows from the input and write them to the output in the correct
# order:
for row in all_rows:
  writer.writerow(row)
