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


def dict_reader(csv_file):
    return csv.DictReader(
        csv_file, delimiter=",", quotechar="|", fieldnames=["signature"])


def read_flag_trie_from_file(file):
    with open(file, "r", encoding="utf8") as stream:
        return read_flag_trie_from_stream(stream)


def read_flag_trie_from_stream(stream):
    trie = signature_trie()
    reader = dict_reader(stream)
    for row in reader:
        signature = row["signature"]
        trie.add(signature, row)
    return trie


def extract_subset_from_monolithic_flags_as_dict_from_file(
        monolithic_trie, patterns_file):
    """Extract a subset of flags from the dict of monolithic flags.

    :param monolithic_trie: the trie containing all the monolithic flags.
    :param patterns_file: a file containing a list of signature patterns that
    define the subset.
    :return: the dict from signature to row.
    """
    with open(patterns_file, "r", encoding="utf8") as stream:
        return extract_subset_from_monolithic_flags_as_dict_from_stream(
            monolithic_trie, stream)


def extract_subset_from_monolithic_flags_as_dict_from_stream(
        monolithic_trie, stream):
    """Extract a subset of flags from the trie of monolithic flags.

    :param monolithic_trie: the trie containing all the monolithic flags.
    :param stream: a stream containing a list of signature patterns that define
    the subset.
    :return: the dict from signature to row.
    """
    dict_signature_to_row = {}
    for pattern in stream:
        pattern = pattern.rstrip()
        rows = monolithic_trie.get_matching_rows(pattern)
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


def read_signature_csv_from_file_as_dict(csv_file):
    """Read the csvFile into a dict.

    The first column is assumed to be the signature and used as the
    key.

    The whole row is stored as the value.
    :param csv_file: the csv file to read
    :return: the dict from signature to row.
    """
    with open(csv_file, "r", encoding="utf8") as f:
        return read_signature_csv_from_stream_as_dict(f)


def compare_signature_flags(monolithic_flags_dict, modular_flags_dict):
    """Compare the signature flags between the two dicts.

    :param monolithic_flags_dict: the dict containing the subset of the
    monolithic flags that should be equal to the modular flags.
    :param modular_flags_dict:the dict containing the flags produced by a single
    bootclasspath_fragment module.
    :return: list of mismatches., each mismatch is a tuple where the first item
    is the signature, and the second and third items are lists of the flags from
    modular dict, and monolithic dict respectively.
    """
    mismatching_signatures = []
    # Create a sorted set of all the signatures from both the monolithic and
    # modular dicts.
    all_signatures = sorted(
        set(chain(monolithic_flags_dict.keys(), modular_flags_dict.keys())))
    for signature in all_signatures:
        monolithic_row = monolithic_flags_dict.get(signature, {})
        monolithic_flags = monolithic_row.get(None, [])
        if signature in modular_flags_dict:
            modular_row = modular_flags_dict.get(signature, {})
            modular_flags = modular_row.get(None, [])
        else:
            modular_flags = ["blocked"]
        if monolithic_flags != modular_flags:
            mismatching_signatures.append(
                (signature, modular_flags, monolithic_flags))
    return mismatching_signatures


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
    monolithic_flags_path = args.monolithicFlags
    monolithic_trie = read_flag_trie_from_file(monolithic_flags_path)

    # For each subset specified on the command line, create dicts for the flags
    # provided by the subset and the corresponding flags from the complete set
    # of flags and compare them.
    failed = False
    for modular_pair in args.modularFlags:
        parts = modular_pair.split(":")
        modular_flags_path = parts[0]
        modular_patterns_path = parts[1]
        modular_flags_dict = read_signature_csv_from_file_as_dict(
            modular_flags_path)
        monolithic_flags_subset_dict = \
            extract_subset_from_monolithic_flags_as_dict_from_file(
                monolithic_trie, modular_patterns_path)
        mismatching_signatures = compare_signature_flags(
            monolithic_flags_subset_dict, modular_flags_dict)
        if mismatching_signatures:
            failed = True
            print("ERROR: Hidden API flags are inconsistent:")
            print("< " + modular_flags_path)
            print("> " + monolithic_flags_path)
            for mismatch in mismatching_signatures:
                signature = mismatch[0]
                print()
                print("< " + ",".join([signature] + mismatch[1]))
                print("> " + ",".join([signature] + mismatch[2]))

    if failed:
        sys.exit(1)


if __name__ == "__main__":
    main(sys.argv)
