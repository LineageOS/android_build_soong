#!/usr/bin/env python
#
# Copyright (C) 2021 The Android Open Source Project
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
"""Generate a set of signature patterns for a bootclasspath_fragment.

The patterns are generated from the modular flags produced by the
bootclasspath_fragment and are used to select a subset of the monolithic flags
against which the modular flags can be compared.
"""

import argparse
import csv
import sys


def dict_reader(csv_file):
    return csv.DictReader(
        csv_file, delimiter=',', quotechar='|', fieldnames=['signature'])


def dot_package_to_slash_package(pkg):
    return pkg.replace('.', '/')


def dot_packages_to_slash_packages(pkgs):
    return [dot_package_to_slash_package(p) for p in pkgs]


def slash_package_to_dot_package(pkg):
    return pkg.replace('/', '.')


def slash_packages_to_dot_packages(pkgs):
    return [slash_package_to_dot_package(p) for p in pkgs]


def is_split_package(split_packages, pkg):
    return split_packages and (pkg in split_packages or '*' in split_packages)


def matched_by_package_prefix_pattern(package_prefixes, prefix):
    for packagePrefix in package_prefixes:
        if prefix == packagePrefix:
            return packagePrefix
        if (prefix.startswith(packagePrefix) and
                prefix[len(packagePrefix)] == '/'):
            return packagePrefix
    return False


def validate_package_prefixes(split_packages, package_prefixes):
    # If there are no package prefixes then there is no possible conflict
    # between them and the split packages.
    if len(package_prefixes) == 0:
        return []

    # Check to make sure that the split packages and package prefixes do not
    # overlap.
    errors = []
    for split_package in split_packages:
        if split_package == '*':
            # A package prefix matches a split package.
            package_prefixes_for_output = ', '.join(
                slash_packages_to_dot_packages(package_prefixes))
            errors.append(
                "split package '*' conflicts with all package prefixes "
                f'{package_prefixes_for_output}\n'
                '    add split_packages:[] to fix')
        else:
            package_prefix = matched_by_package_prefix_pattern(
                package_prefixes, split_package)
            if package_prefix:
                # A package prefix matches a split package.
                split_package_for_output = slash_package_to_dot_package(
                    split_package)
                package_prefix_for_output = slash_package_to_dot_package(
                    package_prefix)
                errors.append(
                    f'split package {split_package_for_output} is matched by '
                    f'package prefix {package_prefix_for_output}')
    return errors


def validate_split_packages(split_packages):
    errors = []
    if '*' in split_packages and len(split_packages) > 1:
        errors.append('split packages are invalid as they contain both the'
                      ' wildcard (*) and specific packages, use the wildcard or'
                      ' specific packages, not a mixture')
    return errors


def produce_patterns_from_file(file,
                               split_packages=None,
                               package_prefixes=None):
    with open(file, 'r', encoding='utf8') as f:
        return produce_patterns_from_stream(f, split_packages, package_prefixes)


def produce_patterns_from_stream(stream,
                                 split_packages=None,
                                 package_prefixes=None):
    split_packages = set(split_packages or [])
    package_prefixes = list(package_prefixes or [])
    # Read in all the signatures into a list and remove any unnecessary class
    # and member names.
    patterns = set()
    for row in dict_reader(stream):
        signature = row['signature']
        text = signature.removeprefix('L')
        # Remove the class specific member signature
        pieces = text.split(';->')
        qualified_class_name = pieces[0]
        pieces = qualified_class_name.rsplit('/', maxsplit=1)
        pkg = pieces[0]
        # If the package is split across multiple modules then it cannot be used
        # to select the subset of the monolithic flags that this module
        # produces. In that case we need to keep the name of the class but can
        # discard any nested class names as an outer class cannot be split
        # across modules.
        #
        # If the package is not split then every class in the package must be
        # provided by this module so there is no need to list the classes
        # explicitly so just use the package name instead.
        if is_split_package(split_packages, pkg):
            # Remove inner class names.
            pieces = qualified_class_name.split('$', maxsplit=1)
            pattern = pieces[0]
        else:
            # Add a * to ensure that the pattern matches the classes in that
            # package.
            pattern = pkg + '/*'
        patterns.add(pattern)

    # Remove any patterns that would be matched by a package prefix pattern.
    patterns = [
        p for p in patterns
        if not matched_by_package_prefix_pattern(package_prefixes, p)
    ]
    # Add the package prefix patterns to the list. Add a ** to ensure that each
    # package prefix pattern will match the classes in that package and all
    # sub-packages.
    patterns = patterns + [f'{p}/**' for p in package_prefixes]
    # Sort the patterns.
    patterns.sort()
    return patterns


def main(args):
    args_parser = argparse.ArgumentParser(
        description='Generate a set of signature patterns '
        'that select a subset of monolithic hidden API files.')
    args_parser.add_argument(
        '--flags',
        help='The stub flags file which contains an entry for every dex member',
    )
    args_parser.add_argument(
        '--split-package',
        action='append',
        help='A package that is split across multiple bootclasspath_fragment '
        'modules')
    args_parser.add_argument(
        '--package-prefix',
        action='append',
        help='A package prefix unique to this set of flags')
    args_parser.add_argument('--output', help='Generated signature prefixes')
    args = args_parser.parse_args(args)

    split_packages = set(
        dot_packages_to_slash_packages(args.split_package or []))
    errors = validate_split_packages(split_packages)

    package_prefixes = dot_packages_to_slash_packages(args.package_prefix or [])

    if not errors:
        errors = validate_package_prefixes(split_packages, package_prefixes)

    if errors:
        for error in errors:
            print(error)
        sys.exit(1)

    # Read in all the patterns into a list.
    patterns = produce_patterns_from_file(args.flags, split_packages,
                                          package_prefixes)

    # Write out all the patterns.
    with open(args.output, 'w', encoding='utf8') as outputFile:
        for pattern in patterns:
            outputFile.write(pattern)
            outputFile.write('\n')


if __name__ == '__main__':
    main(sys.argv[1:])
