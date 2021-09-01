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
"""Verify that one set of hidden API flags is a subset of another.
"""

import argparse
import csv
import sys
from itertools import chain

#pylint: disable=line-too-long
class InteriorNode:
    """An interior node in a trie.

    Each interior node has a dict that maps from an element of a signature to
    either another interior node or a leaf. Each interior node represents either
    a package, class or nested class. Class members are represented by a Leaf.

    Associating the set of flags [public-api] with the signature
    "Ljava/lang/Object;->String()Ljava/lang/String;" will cause the following
    nodes to be created:
    Node()
    ^- package:java -> Node()
       ^- package:lang -> Node()
           ^- class:Object -> Node()
              ^- member:String()Ljava/lang/String; -> Leaf([public-api])

    Associating the set of flags [blocked,core-platform-api] with the signature
    "Ljava/lang/Character$UnicodeScript;->of(I)Ljava/lang/Character$UnicodeScript;"
    will cause the following nodes to be created:
    Node()
    ^- package:java -> Node()
       ^- package:lang -> Node()
           ^- class:Character -> Node()
              ^- class:UnicodeScript -> Node()
                 ^- member:of(I)Ljava/lang/Character$UnicodeScript;
                    -> Leaf([blocked,core-platform-api])

    Attributes:
        nodes: a dict from an element of the signature to the Node/Leaf
          containing the next element/value.
    """
    #pylint: enable=line-too-long

    def __init__(self):
        self.nodes = {}

    #pylint: disable=line-too-long
    def signatureToElements(self, signature):
        """Split a signature or a prefix into a number of elements:
        1. The packages (excluding the leading L preceding the first package).
        2. The class names, from outermost to innermost.
        3. The member signature.
        e.g.
        Ljava/lang/Character$UnicodeScript;->of(I)Ljava/lang/Character$UnicodeScript;
        will be broken down into these elements:
        1. package:java
        2. package:lang
        3. class:Character
        4. class:UnicodeScript
        5. member:of(I)Ljava/lang/Character$UnicodeScript;
        """
        # Remove the leading L.
        #  - java/lang/Character$UnicodeScript;->of(I)Ljava/lang/Character$UnicodeScript;
        text = signature.removeprefix("L")
        # Split the signature between qualified class name and the class member
        # signature.
        #  0 - java/lang/Character$UnicodeScript
        #  1 - of(I)Ljava/lang/Character$UnicodeScript;
        parts = text.split(";->")
        member = parts[1:]
        # Split the qualified class name into packages, and class name.
        #  0 - java
        #  1 - lang
        #  2 - Character$UnicodeScript
        elements = parts[0].split("/")
        packages = elements[0:-1]
        className = elements[-1]
        if className in ("*" , "**"): #pylint: disable=no-else-return
            # Cannot specify a wildcard and target a specific member
            if len(member) != 0:
                raise Exception(
                    "Invalid signature %s: contains wildcard %s and member " \
                    "signature %s"
                    % (signature, className, member[0]))
            wildcard = [className]
            # Assemble the parts into a single list, adding prefixes to identify
            # the different parts.
            #  0 - package:java
            #  1 - package:lang
            #  2 - *
            return list(
                chain(["package:" + x for x in packages], wildcard))
        else:
            # Split the class name into outer / inner classes
            #  0 - Character
            #  1 - UnicodeScript
            classes = className.split("$")
            # Assemble the parts into a single list, adding prefixes to identify
            # the different parts.
            #  0 - package:java
            #  1 - package:lang
            #  2 - class:Character
            #  3 - class:UnicodeScript
            #  4 - member:of(I)Ljava/lang/Character$UnicodeScript;
            return list(
                chain(
                    ["package:" + x for x in packages],
                    ["class:" + x for x in classes],
                    ["member:" + x for x in member]))
    #pylint: enable=line-too-long

    def add(self, signature, value):
        """Associate the value with the specific signature.

        :param signature: the member signature
        :param value: the value to associated with the signature
        :return: n/a
        """
        # Split the signature into elements.
        elements = self.signatureToElements(signature)
        # Find the Node associated with the deepest class.
        node = self
        for element in elements[:-1]:
            if element in node.nodes:
                node = node.nodes[element]
            else:
                next_node = InteriorNode()
                node.nodes[element] = next_node
                node = next_node
        # Add a Leaf containing the value and associate it with the member
        # signature within the class.
        lastElement = elements[-1]
        if not lastElement.startswith("member:"):
            raise Exception(
                "Invalid signature: %s, does not identify a specific member" %
                signature)
        if lastElement in node.nodes:
            raise Exception("Duplicate signature: %s" % signature)
        node.nodes[lastElement] = Leaf(value)

    def getMatchingRows(self, pattern):
        """Get the values (plural) associated with the pattern.

        e.g. If the pattern is a full signature then this will return a list
        containing the value associated with that signature.

        If the pattern is a class then this will return a list containing the
        values associated with all members of that class.

        If the pattern is a package then this will return a list containing the
        values associated with all the members of all the classes in that
        package and sub-packages.

        If the pattern ends with "*" then the preceding part is treated as a
        package and this will return a list containing the values associated
        with all the members of all the classes in that package.

        If the pattern ends with "**" then the preceding part is treated
        as a package and this will return a list containing the values
        associated with all the members of all the classes in that package and
        all sub-packages.

        :param pattern: the pattern which could be a complete signature or a
        class, or package wildcard.
        :return: an iterable containing all the values associated with the
        pattern.
        """
        elements = self.signatureToElements(pattern)
        node = self
        # Include all values from this node and all its children.
        selector = lambda x: True
        lastElement = elements[-1]
        if lastElement in ("*", "**"):
            elements = elements[:-1]
            if lastElement == "*":
                # Do not include values from sub-packages.
                selector = lambda x: not x.startswith("package:")
        for element in elements:
            if element in node.nodes:
                node = node.nodes[element]
            else:
                return []
        return chain.from_iterable(node.values(selector))

    def values(self, selector):
        """:param selector: a function that can be applied to a key in the nodes
        attribute to determine whether to return its values.

        :return: A list of iterables of all the values associated with
        this node and its children.
        """
        values = []
        self.appendValues(values, selector)
        return values

    def appendValues(self, values, selector):
        """Append the values associated with this node and its children to the
        list.

        For each item (key, child) in nodes the child node's values are returned
        if and only if the selector returns True when called on its key. A child
        node's values are all the values associated with it and all its
        descendant nodes.

        :param selector: a function that can be applied to a key in the nodes
        attribute to determine whether to return its values.
        :param values: a list of a iterables of values.
        """
        for key, node in self.nodes.items():
            if selector(key):
                node.appendValues(values, lambda x: True)


class Leaf:
    """A leaf of the trie

    Attributes:
        value: the value associated with this leaf.
    """

    def __init__(self, value):
        self.value = value

    def values(self, selector): #pylint: disable=unused-argument
        """:return: A list of a list of the value associated with this node.
        """
        return [[self.value]]

    def appendValues(self, values, selector): #pylint: disable=unused-argument
        """Appends a list of the value associated with this node to the list.

        :param values: a list of a iterables of values.
        """
        values.append([self.value])


def dict_reader(csvfile):
    return csv.DictReader(
        csvfile, delimiter=",", quotechar="|", fieldnames=["signature"])


def read_flag_trie_from_file(file):
    with open(file, "r") as stream:
        return read_flag_trie_from_stream(stream)


def read_flag_trie_from_stream(stream):
    trie = InteriorNode()
    reader = dict_reader(stream)
    for row in reader:
        signature = row["signature"]
        trie.add(signature, row)
    return trie


def extract_subset_from_monolithic_flags_as_dict_from_file(
        monolithicTrie, patternsFile):
    """Extract a subset of flags from the dict containing all the monolithic
    flags.

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
    """Extract a subset of flags from the trie containing all the monolithic
    flags.

    :param monolithicTrie: the trie containing all the monolithic flags.
    :param stream: a stream containing a list of signature patterns that define
    the subset.
    :return: the dict from signature to row.
    """
    dict_signature_to_row = {}
    for pattern in stream:
        pattern = pattern.rstrip()
        rows = monolithicTrie.getMatchingRows(pattern)
        for row in rows:
            signature = row["signature"]
            dict_signature_to_row[signature] = row
    return dict_signature_to_row


def read_signature_csv_from_stream_as_dict(stream):
    """Read the csv contents from the stream into a dict. The first column is
    assumed to be the signature and used as the key.

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
    """Read the csvFile into a dict. The first column is assumed to be the
    signature and used as the key.

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
        "the monolithic flag file."
    )
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
