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

args_parser = argparse.ArgumentParser(description='Verify that one set of hidden API flags is a subset of another.')
args_parser.add_argument('all', help='All the flags')
args_parser.add_argument('subsets', nargs=argparse.REMAINDER, help='Subsets of the flags')
args = args_parser.parse_args()


def dict_reader(input):
    return csv.DictReader(input, delimiter=',', quotechar='|', fieldnames=['signature'])

# Read in all the flags into a dict indexed by signature
allFlagsBySignature = {}
with open(args.all, 'r') as allFlagsFile:
    allFlagsReader = dict_reader(allFlagsFile)
    for row in allFlagsReader:
        signature = row['signature']
        allFlagsBySignature[signature]=row

failed = False
for subsetPath in args.subsets:
    mismatchingSignatures = []
    with open(subsetPath, 'r') as subsetFlagsFile:
        subsetReader = dict_reader(subsetFlagsFile)
        for row in subsetReader:
            signature = row['signature']
            if signature in allFlagsBySignature:
                allFlags = allFlagsBySignature.get(signature)
                if allFlags != row:
                    mismatchingSignatures.append((signature, row[None], allFlags[None]))
            else:
                mismatchingSignatures.append((signature, row[None], []))


    if mismatchingSignatures:
        failed = True
        print("ERROR: Hidden API flags are inconsistent:")
        print("< " + subsetPath)
        print("> " + args.all)
        for mismatch in mismatchingSignatures:
            print()
            print("< " + mismatch[0] + "," + ",".join(mismatch[1]))
            if mismatch[2] != None:
                print("> " + mismatch[0] + "," + ",".join(mismatch[2]))
            else:
                print("> " + mismatch[0] + " - missing")

if failed:
    sys.exit(1)
