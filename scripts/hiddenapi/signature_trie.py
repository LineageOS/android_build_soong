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
"""Verify that one set of hidden API flags is a subset of another."""
import dataclasses
import typing

from itertools import chain


@dataclasses.dataclass()
class Node:

    def values(self, selector):
        """Get the values from a set of selected nodes.

        :param selector: a function that can be applied to a key in the nodes
            attribute to determine whether to return its values.

        :return: A list of iterables of all the values associated with
            this node and its children.
        """
        raise NotImplementedError("Please Implement this method")

    def append_values(self, values, selector):
        """Append the values associated with this node and its children.

        For each item (key, child) in nodes the child node's values are returned
        if and only if the selector returns True when called on its key. A child
        node's values are all the values associated with it and all its
        descendant nodes.

        :param selector: a function that can be applied to a key in the nodes
        attribute to determine whether to return its values.
        :param values: a list of a iterables of values.
        """
        raise NotImplementedError("Please Implement this method")


# pylint: disable=line-too-long
@dataclasses.dataclass()
class InteriorNode(Node):
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
    """

    # pylint: enable=line-too-long

    # A dict from an element of the signature to the Node/Leaf containing the
    # next element/value.
    nodes: typing.Dict[str, Node] = dataclasses.field(default_factory=dict)

    # pylint: disable=line-too-long
    @staticmethod
    def signature_to_elements(signature):
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
        # If there is no member then this will be an empty list.
        member = parts[1:]
        # Split the qualified class name into packages, and class name.
        #  0 - java
        #  1 - lang
        #  2 - Character$UnicodeScript
        elements = parts[0].split("/")
        last_element = elements[-1]
        wildcard = []
        classes = []
        if "*" in last_element:
            if last_element not in ("*", "**"):
                raise Exception(f"Invalid signature '{signature}': invalid "
                                f"wildcard '{last_element}'")
            packages = elements[0:-1]
            # Cannot specify a wildcard and target a specific member
            if member:
                raise Exception(f"Invalid signature '{signature}': contains "
                                f"wildcard '{last_element}' and "
                                f"member signature '{member[0]}'")
            wildcard = [last_element]
        elif last_element.islower():
            raise Exception(f"Invalid signature '{signature}': last element "
                            f"'{last_element}' is lower case but should be an "
                            f"upper case class name or wildcard")
        else:
            packages = elements[0:-1]
            # Split the class name into outer / inner classes
            #  0 - Character
            #  1 - UnicodeScript
            classes = last_element.removesuffix(";").split("$")

        # Assemble the parts into a single list, adding prefixes to identify
        # the different parts. If a wildcard is provided then it looks something
        # like this:
        #  0 - package:java
        #  1 - package:lang
        #  2 - *
        #
        # Otherwise, it looks something like this:
        #  0 - package:java
        #  1 - package:lang
        #  2 - class:Character
        #  3 - class:UnicodeScript
        #  4 - member:of(I)Ljava/lang/Character$UnicodeScript;
        return list(
            chain([f"package:{x}" for x in packages],
                  [f"class:{x}" for x in classes],
                  [f"member:{x}" for x in member],
                  [f"wildcard:{x}" for x in wildcard]))

    # pylint: enable=line-too-long

    @staticmethod
    def split_element(element):
        element_type, element_value = element.split(":", 1)
        return element_type, element_value

    @staticmethod
    def element_type(element):
        element_type, _ = InteriorNode.split_element(element)
        return element_type

    def add(self, signature, value):
        """Associate the value with the specific signature.

        :param signature: the member signature
        :param value: the value to associated with the signature
        :return: n/a
        """
        # Split the signature into elements.
        elements = self.signature_to_elements(signature)
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
        last_element = elements[-1]
        last_element_type = self.element_type(last_element)
        if last_element_type != "member":
            raise Exception(
                f"Invalid signature: {signature}, does not identify a "
                "specific member")
        if last_element in node.nodes:
            raise Exception(f"Duplicate signature: {signature}")
        node.nodes[last_element] = Leaf(value)

    def get_matching_rows(self, pattern):
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
        elements = self.signature_to_elements(pattern)
        node = self

        # Include all values from this node and all its children.
        selector = lambda x: True

        last_element = elements[-1]
        last_element_type, last_element_value = self.split_element(last_element)
        if last_element_type == "wildcard":
            elements = elements[:-1]
            if last_element_value == "*":
                # Do not include values from sub-packages.
                selector = lambda x: InteriorNode.element_type(x) != "package"

        for element in elements:
            if element in node.nodes:
                node = node.nodes[element]
            else:
                return []
        return chain.from_iterable(node.values(selector))

    def values(self, selector):
        values = []
        self.append_values(values, selector)
        return values

    def append_values(self, values, selector):
        for key, node in self.nodes.items():
            if selector(key):
                node.append_values(values, lambda x: True)


@dataclasses.dataclass()
class Leaf(Node):
    """A leaf of the trie"""

    # The value associated with this leaf.
    value: typing.Any

    def values(self, selector):
        return [[self.value]]

    def append_values(self, values, selector):
        values.append([self.value])


def signature_trie():
    return InteriorNode()
