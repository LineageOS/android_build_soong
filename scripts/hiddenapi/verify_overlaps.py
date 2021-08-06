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
Verify that one set of hidden API flags is a subset of another.
"""

import argparse
import csv

def dict_reader(input):
    return csv.DictReader(input, delimiter=',', quotechar='|', fieldnames=['signature'])

def read_signature_csv_from_stream_as_dict(stream):
    """
    Read the csv contents from the stream into a dict. The first column is assumed to be the
    signature and used as the key. The whole row is stored as the value.

    :param stream: the csv contents to read
    :return: the dict from signature to row.
    """
    dict = {}
    reader = dict_reader(stream)
    for row in reader:
        signature = row['signature']
        dict[signature] = row
    return dict

def read_signature_csv_from_file_as_dict(csvFile):
    """
    Read the csvFile into a dict. The first column is assumed to be the
    signature and used as the key. The whole row is stored as the value.

    :param csvFile: the csv file to read
    :return: the dict from signature to row.
    """
    with open(csvFile, 'r') as f:
        return read_signature_csv_from_stream_as_dict(f)

def compare_signature_flags(monolithicFlagsDict, modularFlagsDict):
    """
    Compare the signature flags between the two dicts.

    :param monolithicFlagsDict: the dict containing the subset of the monolithic
    flags that should be equal to the modular flags.
    :param modularFlagsDict:the dict containing the flags produced by a single
    bootclasspath_fragment module.
    :return: list of mismatches., each mismatch is a tuple where the first item
    is the signature, and the second and third items are lists of the flags from
    modular dict, and monolithic dict respectively.
    """
    mismatchingSignatures = []
    for signature, modularRow in modularFlagsDict.items():
        modularFlags = modularRow.get(None, [])
        monolithicRow = monolithicFlagsDict.get(signature, {})
        monolithicFlags = monolithicRow.get(None, [])
        if monolithicFlags != modularFlags:
            mismatchingSignatures.append((signature, modularFlags, monolithicFlags))
    return mismatchingSignatures

def main(argv):
    args_parser = argparse.ArgumentParser(description='Verify that one set of hidden API flags is a subset of another.')
    args_parser.add_argument('all', help='All the flags')
    args_parser.add_argument('subsets', nargs=argparse.REMAINDER, help='Subsets of the flags')
    args = args_parser.parse_args(argv[1:])

    # Read in all the flags into a dict indexed by signature
    allFlagsBySignature = read_signature_csv_from_file_as_dict(args.all)

    failed = False
    for subsetPath in args.subsets:
        subsetDict = read_signature_csv_from_file_as_dict(subsetPath)
        mismatchingSignatures = compare_signature_flags(allFlagsBySignature, subsetDict)
        if mismatchingSignatures:
            failed = True
            print("ERROR: Hidden API flags are inconsistent:")
            print("< " + subsetPath)
            print("> " + args.all)
            for mismatch in mismatchingSignatures:
                signature = mismatch[0]
                print()
                print("< " + ",".join([signature]+ mismatch[1]))
                print("> " + ",".join([signature]+ mismatch[2]))

    if failed:
        sys.exit(1)

if __name__ == "__main__":
    main(sys.argv)
