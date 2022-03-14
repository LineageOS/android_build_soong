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
"""Verify that one set of hidden API flags is a subset of another."""

import argparse
import csv
import sys
from itertools import chain

from signature_trie import signature_trie


def dict_reader(csvfile):
    return csv.DictReader(
        csvfile, delimiter=",", quotechar="|", fieldnames=["signature"])


def read_flag_trie_from_file(file):
    with open(file, "r") as stream:
        return read_flag_trie_from_stream(stream)


def read_flag_trie_from_stream(stream):
    trie = signature_trie()
    reader = dict_reader(stream)
    for row in reader:
        signature = row["signature"]
        trie.add(signature, row)
    return trie


def extract_subset_from_monolithic_flags_as_dict_from_file(
        monolithicTrie, patternsFile):
    """Extract a subset of flags from the dict of monolithic flags.

    :param monolithicFlagsDict: the dict containing all the monolithic flags.
    :param patternsFile: a file containing a list of signature patterns that
    define the subset.
    :return: the dict from signature to row.
    """
    with open(patternsFile, "r") as stream:
        return extract_subset_from_monolithic_flags_as_dict_from_stream(
            monolithicTrie, stream)


def extract_subset_from_monolithic_flags_as_dict_from_stream(
        monolithicTrie, stream):
    """Extract a subset of flags from the trie of monolithic flags.

    :param monolithicTrie: the trie containing all the monolithic flags.
    :param stream: a stream containing a list of signature patterns that define
    the subset.
    :return: the dict from signature to row.
    """
    dict_signature_to_row = {}
    for pattern in stream:
        pattern = pattern.rstrip()
        rows = monolithicTrie.get_matching_rows(pattern)
        for row in rows:
            signature = row["signature"]
            dict_signature_to_row[signature] = row
    return dict_signature_to_row


def read_signature_csv_from_stream_as_dict(stream):
    """Read the csv contents from the stream into a dict.

    The first column is assumed to be the signature and used as the
    key.

    The whole row is stored as the value.
    :param stream: the csv contents to read
    :return: the dict from signature to row.
    """
    dict_signature_to_row = {}
    reader = dict_reader(stream)
    for row in reader:
        signature = row["signature"]
        dict_signature_to_row[signature] = row
    return dict_signature_to_row


def read_signature_csv_from_file_as_dict(csvFile):
    """Read the csvFile into a dict.

    The first column is assumed to be the signature and used as the
    key.

    The whole row is stored as the value.
    :param csvFile: the csv file to read
    :return: the dict from signature to row.
    """
    with open(csvFile, "r") as f:
        return read_signature_csv_from_stream_as_dict(f)


def compare_signature_flags(monolithicFlagsDict, modularFlagsDict):
    """Compare the signature flags between the two dicts.

    :param monolithicFlagsDict: the dict containing the subset of the monolithic
    flags that should be equal to the modular flags.
    :param modularFlagsDict:the dict containing the flags produced by a single
    bootclasspath_fragment module.
    :return: list of mismatches., each mismatch is a tuple where the first item
    is the signature, and the second and third items are lists of the flags from
    modular dict, and monolithic dict respectively.
    """
    mismatchingSignatures = []
    # Create a sorted set of all the signatures from both the monolithic and
    # modular dicts.
    allSignatures = sorted(
        set(chain(monolithicFlagsDict.keys(), modularFlagsDict.keys())))
    for signature in allSignatures:
        monolithicRow = monolithicFlagsDict.get(signature, {})
        monolithicFlags = monolithicRow.get(None, [])
        if signature in modularFlagsDict:
            modularRow = modularFlagsDict.get(signature, {})
            modularFlags = modularRow.get(None, [])
        else:
            modularFlags = ["blocked"]
        if monolithicFlags != modularFlags:
            mismatchingSignatures.append(
                (signature, modularFlags, monolithicFlags))
    return mismatchingSignatures


def main(argv):
    args_parser = argparse.ArgumentParser(
        description="Verify that sets of hidden API flags are each a subset of "
        "the monolithic flag file.")
    args_parser.add_argument("monolithicFlags", help="The monolithic flag file")
    args_parser.add_argument(
        "modularFlags",
        nargs=argparse.REMAINDER,
        help="Flags produced by individual bootclasspath_fragment modules")
    args = args_parser.parse_args(argv[1:])

    # Read in all the flags into the trie
    monolithicFlagsPath = args.monolithicFlags
    monolithicTrie = read_flag_trie_from_file(monolithicFlagsPath)

    # For each subset specified on the command line, create dicts for the flags
    # provided by the subset and the corresponding flags from the complete set
    # of flags and compare them.
    failed = False
    for modularPair in args.modularFlags:
        parts = modularPair.split(":")
        modularFlagsPath = parts[0]
        modularPatternsPath = parts[1]
        modularFlagsDict = read_signature_csv_from_file_as_dict(
            modularFlagsPath)
        monolithicFlagsSubsetDict = \
            extract_subset_from_monolithic_flags_as_dict_from_file(
            monolithicTrie, modularPatternsPath)
        mismatchingSignatures = compare_signature_flags(
            monolithicFlagsSubsetDict, modularFlagsDict)
        if mismatchingSignatures:
            failed = True
            print("ERROR: Hidden API flags are inconsistent:")
            print("< " + modularFlagsPath)
            print("> " + monolithicFlagsPath)
            for mismatch in mismatchingSignatures:
                signature = mismatch[0]
                print()
                print("< " + ",".join([signature] + mismatch[1]))
                print("> " + ",".join([signature] + mismatch[2]))

    if failed:
        sys.exit(1)


if __name__ == "__main__":
    main(sys.argv)
