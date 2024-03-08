#!/usr/bin/env python
#
# Copyright (C) 2020 The Android Open Source Project
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
#
"""Unit tests for construct_context.py."""

import sys
import unittest

import construct_context as cc

sys.dont_write_bytecode = True


CONTEXT_JSON = {
    '28': [
        {
            'Name': 'z',
            'Optional': False,
            'Host': 'out/zdir/z.jar',
            'Device': '/system/z.jar',
            'Subcontexts': [],
        },
    ],
    '29': [
        {
            'Name': 'x',
            'Optional': False,
            'Host': 'out/xdir/x.jar',
            'Device': '/system/x.jar',
            'Subcontexts': [],
        },
        {
            'Name': 'y',
            'Optional': False,
            'Host': 'out/ydir/y.jar',
            'Device': '/product/y.jar',
            'Subcontexts': [],
        },
    ],
    'any': [
        {
            'Name': 'a',
            'Optional': False,
            'Host': 'out/adir/a.jar',
            'Device': '/system/a.jar',
            'Subcontexts': [
                {  # Not installed optional, being the only child.
                    'Name': 'a1',
                    'Optional': True,
                    'Host': 'out/a1dir/a1.jar',
                    'Device': '/product/a1.jar',
                    'Subcontexts': [],
                },
            ],
        },
        {
            'Name': 'b',
            'Optional': True,
            'Host': 'out/bdir/b.jar',
            'Device': '/product/b.jar',
            'Subcontexts': [
                {  # Not installed but required.
                    'Name': 'b1',
                    'Optional': False,
                    'Host': 'out/b1dir/b1.jar',
                    'Device': '/product/b1.jar',
                    'Subcontexts': [],
                },
                {  # Installed optional.
                    'Name': 'b2',
                    'Optional': True,
                    'Host': 'out/b2dir/b2.jar',
                    'Device': '/product/b2.jar',
                    'Subcontexts': [],
                },
                {  # Not installed optional.
                    'Name': 'b3',
                    'Optional': True,
                    'Host': 'out/b3dir/b3.jar',
                    'Device': '/product/b3.jar',
                    'Subcontexts': [],
                },
                {  # Installed optional with one more level of nested deps.
                    'Name': 'b4',
                    'Optional': True,
                    'Host': 'out/b4dir/b4.jar',
                    'Device': '/product/b4.jar',
                    'Subcontexts': [
                        {
                            'Name': 'b41',
                            'Optional': True,
                            'Host': 'out/b41dir/b41.jar',
                            'Device': '/product/b41.jar',
                            'Subcontexts': [],
                        },
                        {
                            'Name': 'b42',
                            'Optional': True,
                            'Host': 'out/b42dir/b42.jar',
                            'Device': '/product/b42.jar',
                            'Subcontexts': [],
                        },
                    ],
                },
            ],
        },
        {  # Not installed optional, at the top-level.
            'Name': 'c',
            'Optional': True,
            'Host': 'out/cdir/c.jar',
            'Device': '/product/c.jar',
            'Subcontexts': [],
        },
    ],
}


PRODUCT_PACKAGES = ['a', 'b', 'b2', 'b4', 'b41', 'b42', 'x', 'y', 'z']


def construct_context_args(target_sdk):
    return cc.construct_context_args(target_sdk, CONTEXT_JSON, PRODUCT_PACKAGES)


class ConstructContextTest(unittest.TestCase):
    def test_construct_context_27(self):
        actual = construct_context_args('27')
        # The order matters.
        expected = (
            'class_loader_context_arg='
            '--class-loader-context=PCL[]{'
            'PCL[out/xdir/x.jar]#'
            'PCL[out/ydir/y.jar]#'
            'PCL[out/zdir/z.jar]#'
            'PCL[out/adir/a.jar]#'
            'PCL[out/bdir/b.jar]{'
            'PCL[out/b1dir/b1.jar]#'
            'PCL[out/b2dir/b2.jar]#'
            'PCL[out/b4dir/b4.jar]{'
            'PCL[out/b41dir/b41.jar]#'
            'PCL[out/b42dir/b42.jar]'
            '}'
            '}'
            '}'
            ' ; '
            'stored_class_loader_context_arg='
            '--stored-class-loader-context=PCL[]{'
            'PCL[/system/x.jar]#'
            'PCL[/product/y.jar]#'
            'PCL[/system/z.jar]#'
            'PCL[/system/a.jar]#'
            'PCL[/product/b.jar]{'
            'PCL[/product/b1.jar]#'
            'PCL[/product/b2.jar]#'
            'PCL[/product/b4.jar]{'
            'PCL[/product/b41.jar]#'
            'PCL[/product/b42.jar]'
            '}'
            '}'
            '}')
        self.assertEqual(actual, expected)

    def test_construct_context_28(self):
        actual = construct_context_args('28')
        expected = (
            'class_loader_context_arg='
            '--class-loader-context=PCL[]{'
            'PCL[out/xdir/x.jar]#'
            'PCL[out/ydir/y.jar]#'
            'PCL[out/adir/a.jar]#'
            'PCL[out/bdir/b.jar]{'
            'PCL[out/b1dir/b1.jar]#'
            'PCL[out/b2dir/b2.jar]#'
            'PCL[out/b4dir/b4.jar]{'
            'PCL[out/b41dir/b41.jar]#'
            'PCL[out/b42dir/b42.jar]'
            '}'
            '}'
            '}'
            ' ; '
            'stored_class_loader_context_arg='
            '--stored-class-loader-context=PCL[]{'
            'PCL[/system/x.jar]#'
            'PCL[/product/y.jar]#'
            'PCL[/system/a.jar]#'
            'PCL[/product/b.jar]{'
            'PCL[/product/b1.jar]#'
            'PCL[/product/b2.jar]#'
            'PCL[/product/b4.jar]{'
            'PCL[/product/b41.jar]#'
            'PCL[/product/b42.jar]'
            '}'
            '}'
            '}')
        self.assertEqual(actual, expected)

    def test_construct_context_29(self):
        actual = construct_context_args('29')
        expected = (
            'class_loader_context_arg='
            '--class-loader-context=PCL[]{'
            'PCL[out/adir/a.jar]#'
            'PCL[out/bdir/b.jar]{'
            'PCL[out/b1dir/b1.jar]#'
            'PCL[out/b2dir/b2.jar]#'
            'PCL[out/b4dir/b4.jar]{'
            'PCL[out/b41dir/b41.jar]#'
            'PCL[out/b42dir/b42.jar]'
            '}'
            '}'
            '}'
            ' ; '
            'stored_class_loader_context_arg='
            '--stored-class-loader-context=PCL[]{'
            'PCL[/system/a.jar]#'
            'PCL[/product/b.jar]{'
            'PCL[/product/b1.jar]#'
            'PCL[/product/b2.jar]#'
            'PCL[/product/b4.jar]{'
            'PCL[/product/b41.jar]#'
            'PCL[/product/b42.jar]'
            '}'
            '}'
            '}')
        self.assertEqual(actual, expected)

    def test_construct_context_S(self):
        actual = construct_context_args('S')
        expected = (
            'class_loader_context_arg='
            '--class-loader-context=PCL[]{'
            'PCL[out/adir/a.jar]#'
            'PCL[out/bdir/b.jar]{'
            'PCL[out/b1dir/b1.jar]#'
            'PCL[out/b2dir/b2.jar]#'
            'PCL[out/b4dir/b4.jar]{'
            'PCL[out/b41dir/b41.jar]#'
            'PCL[out/b42dir/b42.jar]'
            '}'
            '}'
            '}'
            ' ; '
            'stored_class_loader_context_arg='
            '--stored-class-loader-context=PCL[]{'
            'PCL[/system/a.jar]#'
            'PCL[/product/b.jar]{'
            'PCL[/product/b1.jar]#'
            'PCL[/product/b2.jar]#'
            'PCL[/product/b4.jar]{'
            'PCL[/product/b41.jar]#'
            'PCL[/product/b42.jar]'
            '}'
            '}'
            '}')
        self.assertEqual(actual, expected)


if __name__ == '__main__':
    unittest.main(verbosity=2)
