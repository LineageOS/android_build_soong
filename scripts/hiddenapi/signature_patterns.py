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


def dict_reader(csvfile):
    return csv.DictReader(
        csvfile, delimiter=',', quotechar='|', fieldnames=['signature'])


def dotPackageToSlashPackage(pkg):
    return pkg.replace('.', '/')


def slashPackageToDotPackage(pkg):
    return pkg.replace('/', '.')


def isSplitPackage(splitPackages, pkg):
    return splitPackages and (pkg in splitPackages or '*' in splitPackages)


def matchedByPackagePrefixPattern(packagePrefixes, prefix):
    for packagePrefix in packagePrefixes:
        if prefix == packagePrefix:
            return packagePrefix
        elif prefix.startswith(packagePrefix) and prefix[len(
                packagePrefix)] == '/':
            return packagePrefix
    return False


def validate_package_prefixes(splitPackages, packagePrefixes):
    # If there are no package prefixes then there is no possible conflict
    # between them and the split packages.
    if len(packagePrefixes) == 0:
        return

    # Check to make sure that the split packages and package prefixes do not
    # overlap.
    errors = []
    for splitPackage in splitPackages:
        if splitPackage == '*':
            # A package prefix matches a split package.
            packagePrefixesForOutput = ', '.join(
                map(slashPackageToDotPackage, packagePrefixes))
            errors.append(
                'split package "*" conflicts with all package prefixes %s\n'
                '    add split_packages:[] to fix' % packagePrefixesForOutput)
        else:
            packagePrefix = matchedByPackagePrefixPattern(
                packagePrefixes, splitPackage)
            if packagePrefix:
                # A package prefix matches a split package.
                splitPackageForOutput = slashPackageToDotPackage(splitPackage)
                packagePrefixForOutput = slashPackageToDotPackage(packagePrefix)
                errors.append(
                    'split package %s is matched by package prefix %s' %
                    (splitPackageForOutput, packagePrefixForOutput))
    return errors


def validate_split_packages(splitPackages):
    errors = []
    if '*' in splitPackages and len(splitPackages) > 1:
        errors.append('split packages are invalid as they contain both the'
                      ' wildcard (*) and specific packages, use the wildcard or'
                      ' specific packages, not a mixture')
    return errors


def produce_patterns_from_file(file, splitPackages=None, packagePrefixes=None):
    with open(file, 'r') as f:
        return produce_patterns_from_stream(f, splitPackages, packagePrefixes)


def produce_patterns_from_stream(stream,
                                 splitPackages=None,
                                 packagePrefixes=None):
    splitPackages = set(splitPackages or [])
    packagePrefixes = list(packagePrefixes or [])
    # Read in all the signatures into a list and remove any unnecessary class
    # and member names.
    patterns = set()
    for row in dict_reader(stream):
        signature = row['signature']
        text = signature.removeprefix('L')
        # Remove the class specific member signature
        pieces = text.split(';->')
        qualifiedClassName = pieces[0]
        pieces = qualifiedClassName.rsplit('/', maxsplit=1)
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
        if isSplitPackage(splitPackages, pkg):
            # Remove inner class names.
            pieces = qualifiedClassName.split('$', maxsplit=1)
            pattern = pieces[0]
        else:
            # Add a * to ensure that the pattern matches the classes in that
            # package.
            pattern = pkg + '/*'
        patterns.add(pattern)

    # Remove any patterns that would be matched by a package prefix pattern.
    patterns = list(
        filter(lambda p: not matchedByPackagePrefixPattern(packagePrefixes, p),
               patterns))
    # Add the package prefix patterns to the list. Add a ** to ensure that each
    # package prefix pattern will match the classes in that package and all
    # sub-packages.
    patterns = patterns + list(map(lambda x: x + '/**', packagePrefixes))
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
        help='A package that is split across multiple bootclasspath_fragment modules'
    )
    args_parser.add_argument(
        '--package-prefix',
        action='append',
        help='A package prefix unique to this set of flags')
    args_parser.add_argument('--output', help='Generated signature prefixes')
    args = args_parser.parse_args(args)

    splitPackages = set(map(dotPackageToSlashPackage, args.split_package or []))
    errors = validate_split_packages(splitPackages)

    packagePrefixes = list(
        map(dotPackageToSlashPackage, args.package_prefix or []))

    if not errors:
        errors = validate_package_prefixes(splitPackages, packagePrefixes)

    if errors:
        for error in errors:
            print(error)
        sys.exit(1)

    # Read in all the patterns into a list.
    patterns = produce_patterns_from_file(args.flags, splitPackages,
                                          packagePrefixes)

    # Write out all the patterns.
    with open(args.output, 'w') as outputFile:
        for pattern in patterns:
            outputFile.write(pattern)
            outputFile.write('\n')


if __name__ == '__main__':
    main(sys.argv[1:])
