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
"""A tool for constructing class loader context."""

from __future__ import print_function

import argparse
import json
import sys

from manifest import compare_version_gt


def parse_args(args):
    """Parse commandline arguments."""
    parser = argparse.ArgumentParser()
    parser.add_argument(
        '--target-sdk-version',
        default='',
        dest='sdk',
        help='specify target SDK version (as it appears in the manifest)')
    parser.add_argument(
        '--context-json',
        default='',
        dest='context_json',
    )
    parser.add_argument(
        '--product-packages',
        default='',
        dest='product_packages_file',
    )
    return parser.parse_args(args)


# Special keyword that means that the context should be added to class loader
# context regardless of the target SDK version.
any_sdk = 'any'

context_sep = '#'


def encode_class_loader(context, product_packages):
    host_sub_contexts, target_sub_contexts = encode_class_loaders(
            context['Subcontexts'], product_packages)

    return ('PCL[%s]%s' % (context['Host'], host_sub_contexts),
            'PCL[%s]%s' % (context['Device'], target_sub_contexts))


def encode_class_loaders(contexts, product_packages):
    host_contexts = []
    target_contexts = []

    for context in contexts:
        if not context['Optional'] or context['Name'] in product_packages:
            host_context, target_context = encode_class_loader(
                    context, product_packages)
            host_contexts.append(host_context)
            target_contexts.append(target_context)

    if host_contexts:
        return ('{%s}' % context_sep.join(host_contexts),
                '{%s}' % context_sep.join(target_contexts))
    else:
        return '', ''


def construct_context_args(target_sdk, context_json, product_packages):
    all_contexts = []

    # CLC for different SDK versions should come in specific order that agrees
    # with PackageManager. Since PackageManager processes SDK versions in
    # ascending order and prepends compatibility libraries at the front, the
    # required order is descending, except for any_sdk that has numerically
    # the largest order, but must be the last one. Example of correct order:
    # [30, 29, 28, any_sdk]. There are Python tests to ensure that someone
    # doesn't change this by accident, but there is no way to guard against
    # changes in the PackageManager, except for grepping logcat on the first
    # boot for absence of the following messages:
    #
    #   `logcat | grep -E 'ClassLoaderContext [a-z ]+ mismatch`

    for sdk, contexts in sorted(
            ((sdk, contexts)
             for sdk, contexts in context_json.items()
             if sdk != any_sdk and compare_version_gt(sdk, target_sdk)),
            key=lambda item: int(item[0]), reverse=True):
        all_contexts += contexts

    if any_sdk in context_json:
        all_contexts += context_json[any_sdk]

    host_contexts, target_contexts = encode_class_loaders(
            all_contexts, product_packages)

    return (
        'class_loader_context_arg=--class-loader-context=PCL[]%s ; ' %
        host_contexts +
        'stored_class_loader_context_arg='
        '--stored-class-loader-context=PCL[]%s'
        % target_contexts)


def main():
    """Program entry point."""
    try:
        args = parse_args(sys.argv[1:])
        if not args.sdk:
            raise SystemExit('target sdk version is not set')

        context_json = json.loads(args.context_json)
        with open(args.product_packages_file, 'r') as f:
            product_packages = set(line.strip() for line in f if line.strip())

        print(construct_context_args(args.sdk, context_json, product_packages))

    # pylint: disable=broad-except
    except Exception as err:
        print('error: ' + str(err), file=sys.stderr)
        sys.exit(-1)


if __name__ == '__main__':
    main()
