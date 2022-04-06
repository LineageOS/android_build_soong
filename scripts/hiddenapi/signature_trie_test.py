#!/usr/bin/env python
#
# Copyright (C) 2022 The Android Open Source Project
#
# Licensed under the Apache License, Version 2.0 (the 'License');
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an 'AS IS' BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
"""Unit tests for verify_overlaps_test.py."""
import io
import unittest

from signature_trie import InteriorNode
from signature_trie import signature_trie


class TestSignatureToElements(unittest.TestCase):

    @staticmethod
    def signature_to_elements(signature):
        return InteriorNode.signature_to_elements(signature)

    @staticmethod
    def elements_to_signature(elements):
        return InteriorNode.elements_to_selector(elements)

    def test_nested_inner_classes(self):
        elements = [
            ("package", "java"),
            ("package", "lang"),
            ("class", "ProcessBuilder"),
            ("class", "Redirect"),
            ("class", "1"),
            ("member", "<init>()V"),
        ]
        signature = "Ljava/lang/ProcessBuilder$Redirect$1;-><init>()V"
        self.assertEqual(elements, self.signature_to_elements(signature))
        self.assertEqual(signature, "L" + self.elements_to_signature(elements))

    def test_basic_member(self):
        elements = [
            ("package", "java"),
            ("package", "lang"),
            ("class", "Object"),
            ("member", "hashCode()I"),
        ]
        signature = "Ljava/lang/Object;->hashCode()I"
        self.assertEqual(elements, self.signature_to_elements(signature))
        self.assertEqual(signature, "L" + self.elements_to_signature(elements))

    def test_double_dollar_class(self):
        elements = [
            ("package", "java"),
            ("package", "lang"),
            ("class", "CharSequence"),
            ("class", ""),
            ("class", "ExternalSyntheticLambda0"),
            ("member", "<init>(Ljava/lang/CharSequence;)V"),
        ]
        signature = "Ljava/lang/CharSequence$$ExternalSyntheticLambda0;" \
                    "-><init>(Ljava/lang/CharSequence;)V"
        self.assertEqual(elements, self.signature_to_elements(signature))
        self.assertEqual(signature, "L" + self.elements_to_signature(elements))

    def test_no_member(self):
        elements = [
            ("package", "java"),
            ("package", "lang"),
            ("class", "CharSequence"),
            ("class", ""),
            ("class", "ExternalSyntheticLambda0"),
        ]
        signature = "Ljava/lang/CharSequence$$ExternalSyntheticLambda0"
        self.assertEqual(elements, self.signature_to_elements(signature))
        self.assertEqual(signature, "L" + self.elements_to_signature(elements))

    def test_wildcard(self):
        elements = [
            ("package", "java"),
            ("package", "lang"),
            ("wildcard", "*"),
        ]
        signature = "java/lang/*"
        self.assertEqual(elements, self.signature_to_elements(signature))
        self.assertEqual(signature, self.elements_to_signature(elements))

    def test_recursive_wildcard(self):
        elements = [
            ("package", "java"),
            ("package", "lang"),
            ("wildcard", "**"),
        ]
        signature = "java/lang/**"
        self.assertEqual(elements, self.signature_to_elements(signature))
        self.assertEqual(signature, self.elements_to_signature(elements))

    def test_no_packages_wildcard(self):
        elements = [
            ("wildcard", "*"),
        ]
        signature = "*"
        self.assertEqual(elements, self.signature_to_elements(signature))
        self.assertEqual(signature, self.elements_to_signature(elements))

    def test_no_packages_recursive_wildcard(self):
        elements = [
            ("wildcard", "**"),
        ]
        signature = "**"
        self.assertEqual(elements, self.signature_to_elements(signature))
        self.assertEqual(signature, self.elements_to_signature(elements))

    def test_invalid_no_class_or_wildcard(self):
        signature = "java/lang"
        with self.assertRaises(Exception) as context:
            self.signature_to_elements(signature)
        self.assertIn(
            "last element 'lang' is lower case but should be an "
            "upper case class name or wildcard", str(context.exception))

    def test_non_standard_class_name(self):
        elements = [
            ("package", "javax"),
            ("package", "crypto"),
            ("class", "extObjectInputStream"),
        ]
        signature = "Ljavax/crypto/extObjectInputStream"
        self.assertEqual(elements, self.signature_to_elements(signature))
        self.assertEqual(signature, "L" + self.elements_to_signature(elements))

    def test_invalid_pattern_wildcard(self):
        pattern = "Ljava/lang/Class*"
        with self.assertRaises(Exception) as context:
            self.signature_to_elements(pattern)
        self.assertIn("invalid wildcard 'Class*'", str(context.exception))

    def test_invalid_pattern_wildcard_and_member(self):
        pattern = "Ljava/lang/*;->hashCode()I"
        with self.assertRaises(Exception) as context:
            self.signature_to_elements(pattern)
        self.assertIn(
            "contains wildcard '*' and member signature 'hashCode()I'",
            str(context.exception))


class TestValues(unittest.TestCase):
    def test_add_then_get(self):
        trie = signature_trie()
        trie.add("La/b/C;->l()", 1)
        trie.add("La/b/C$D;->m()", "A")
        trie.add("La/b/C$D;->n()", {})

        package_a_node = next(iter(trie.child_nodes()))
        self.assertEqual("package", package_a_node.type)
        self.assertEqual("a", package_a_node.selector)

        package_b_node = next(iter(package_a_node.child_nodes()))
        self.assertEqual("package", package_b_node.type)
        self.assertEqual("a/b", package_b_node.selector)

        class_c_node = next(iter(package_b_node.child_nodes()))
        self.assertEqual("class", class_c_node.type)
        self.assertEqual("a/b/C", class_c_node.selector)

        self.assertEqual([1, "A", {}], class_c_node.values(lambda _: True))

class TestGetMatchingRows(unittest.TestCase):
    extractInput = """
Ljava/lang/Character$UnicodeScript;->of(I)Ljava/lang/Character$UnicodeScript;
Ljava/lang/Character;->serialVersionUID:J
Ljava/lang/Object;->hashCode()I
Ljava/lang/Object;->toString()Ljava/lang/String;
Ljava/lang/ProcessBuilder$Redirect$1;-><init>()V
Ljava/util/zip/ZipFile;-><clinit>()V
"""

    def read_trie(self):
        trie = signature_trie()
        with io.StringIO(self.extractInput.strip()) as f:
            for line in iter(f.readline, ""):
                line = line.rstrip()
                trie.add(line, line)
        return trie

    def check_patterns(self, pattern, expected):
        trie = self.read_trie()
        self.check_node_patterns(trie, pattern, expected)

    def check_node_patterns(self, node, pattern, expected):
        actual = list(node.get_matching_rows(pattern))
        actual.sort()
        self.assertEqual(expected, actual)

    def test_member_pattern(self):
        self.check_patterns("java/util/zip/ZipFile;-><clinit>()V",
                            ["Ljava/util/zip/ZipFile;-><clinit>()V"])

    def test_class_pattern(self):
        self.check_patterns("java/lang/Object", [
            "Ljava/lang/Object;->hashCode()I",
            "Ljava/lang/Object;->toString()Ljava/lang/String;",
        ])

    # pylint: disable=line-too-long
    def test_nested_class_pattern(self):
        self.check_patterns("java/lang/Character", [
            "Ljava/lang/Character$UnicodeScript;->of(I)Ljava/lang/Character$UnicodeScript;",
            "Ljava/lang/Character;->serialVersionUID:J",
        ])

    def test_wildcard(self):
        self.check_patterns("java/lang/*", [
            "Ljava/lang/Character$UnicodeScript;->of(I)Ljava/lang/Character$UnicodeScript;",
            "Ljava/lang/Character;->serialVersionUID:J",
            "Ljava/lang/Object;->hashCode()I",
            "Ljava/lang/Object;->toString()Ljava/lang/String;",
            "Ljava/lang/ProcessBuilder$Redirect$1;-><init>()V",
        ])

    def test_recursive_wildcard(self):
        self.check_patterns("java/**", [
            "Ljava/lang/Character$UnicodeScript;->of(I)Ljava/lang/Character$UnicodeScript;",
            "Ljava/lang/Character;->serialVersionUID:J",
            "Ljava/lang/Object;->hashCode()I",
            "Ljava/lang/Object;->toString()Ljava/lang/String;",
            "Ljava/lang/ProcessBuilder$Redirect$1;-><init>()V",
            "Ljava/util/zip/ZipFile;-><clinit>()V",
        ])

    def test_node_wildcard(self):
        trie = self.read_trie()
        node = list(trie.child_nodes())[0]
        self.check_node_patterns(node, "**", [
            "Ljava/lang/Character$UnicodeScript;->of(I)Ljava/lang/Character$UnicodeScript;",
            "Ljava/lang/Character;->serialVersionUID:J",
            "Ljava/lang/Object;->hashCode()I",
            "Ljava/lang/Object;->toString()Ljava/lang/String;",
            "Ljava/lang/ProcessBuilder$Redirect$1;-><init>()V",
            "Ljava/util/zip/ZipFile;-><clinit>()V",
        ])

    # pylint: enable=line-too-long


if __name__ == "__main__":
    unittest.main(verbosity=2)
