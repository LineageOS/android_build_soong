#!/usr/bin/env python
#
# Copyright (C) 2016 The Android Open Source Project
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
"""Generates source for stub shared libraries for the NDK."""
import argparse
import os
import re


ALL_ARCHITECTURES = (
    'arm',
    'arm64',
    'mips',
    'mips64',
    'x86',
    'x86_64',
)


class Scope(object):
    """Enum for version script scope.

    Top: Top level of the file.
    Global: In a version and visibility section where symbols should be visible
            to the NDK.
    Local: In a visibility section of a public version where symbols should be
           hidden to the NDK.
    Private: In a version where symbols should not be visible to the NDK.
    """
    Top = 1
    Global = 2
    Local = 3
    Private = 4


class Stack(object):
    """Basic stack implementation."""
    def __init__(self):
        self.stack = []

    def push(self, obj):
        """Push an item on to the stack."""
        self.stack.append(obj)

    def pop(self):
        """Remove and return the item on the top of the stack."""
        return self.stack.pop()

    @property
    def top(self):
        """Return the top of the stack."""
        return self.stack[-1]


def version_is_private(version):
    """Returns True if the version name should be treated as private."""
    return version.endswith('_PRIVATE') or version.endswith('_PLATFORM')


def enter_version(scope, line, version_file):
    """Enters a new version block scope."""
    if scope.top != Scope.Top:
        raise RuntimeError('Encountered nested version block.')

    # Entering a new version block. By convention symbols with versions ending
    # with "_PRIVATE" or "_PLATFORM" are not included in the NDK.
    version_name = line.split('{')[0].strip()
    if version_is_private(version_name):
        scope.push(Scope.Private)
    else:
        scope.push(Scope.Global)  # By default symbols are visible.
        version_file.write(line)


def leave_version(scope, line, version_file):
    """Leave a version block scope."""
    # There is no close to a visibility section, just the end of the version or
    # a new visiblity section.
    assert scope.top in (Scope.Global, Scope.Local, Scope.Private)
    if scope.top != Scope.Private:
        version_file.write(line)
    scope.pop()
    assert scope.top == Scope.Top


def enter_visibility(scope, line, version_file):
    """Enters a new visibility block scope."""
    leave_visibility(scope)
    version_file.write(line)
    visibility = line.split(':')[0].strip()
    if visibility == 'local':
        scope.push(Scope.Local)
    elif visibility == 'global':
        scope.push(Scope.Global)
    else:
        raise RuntimeError('Unknown visiblity label: ' + visibility)


def leave_visibility(scope):
    """Leaves a visibility block scope."""
    assert scope.top in (Scope.Global, Scope.Local)
    scope.pop()
    assert scope.top == Scope.Top


def handle_top_scope(scope, line, version_file):
    """Processes a line in the top level scope."""
    if '{' in line:
        enter_version(scope, line, version_file)
    else:
        raise RuntimeError('Unexpected contents at top level: ' + line)


def handle_private_scope(scope, line, version_file):
    """Eats all input."""
    if '}' in line:
        leave_version(scope, line, version_file)


def handle_local_scope(scope, line, version_file):
    """Passes through input."""
    if ':' in line:
        enter_visibility(scope, line, version_file)
    elif '}' in line:
        leave_version(scope, line, version_file)
    else:
        version_file.write(line)


def symbol_in_arch(tags, arch):
    """Returns true if the symbol is present for the given architecture."""
    has_arch_tags = False
    for tag in tags:
        if tag == arch:
            return True
        if tag in ALL_ARCHITECTURES:
            has_arch_tags = True

    # If there were no arch tags, the symbol is available for all
    # architectures. If there were any arch tags, the symbol is only available
    # for the tagged architectures.
    return not has_arch_tags


def symbol_in_version(tags, arch, version):
    """Returns true if the symbol is present for the given version."""
    introduced_tag = None
    arch_specific = False
    for tag in tags:
        # If there is an arch-specific tag, it should override the common one.
        if tag.startswith('introduced=') and not arch_specific:
            introduced_tag = tag
        elif tag.startswith('introduced-' + arch + '='):
            introduced_tag = tag
            arch_specific = True

    if introduced_tag is None:
        # We found no "introduced" tags, so the symbol has always been
        # available.
        return True

    # The tag is a key=value pair, and we only care about the value now.
    _, _, version_str = introduced_tag.partition('=')
    return version >= int(version_str)


def handle_global_scope(scope, line, src_file, version_file, arch, api):
    """Emits present symbols to the version file and stub source file."""
    if ':' in line:
        enter_visibility(scope, line, version_file)
        return
    if '}' in line:
        leave_version(scope, line, version_file)
        return

    if ';' not in line:
        raise RuntimeError('Expected ; to terminate symbol: ' + line)
    if '*' in line:
        raise RuntimeError('Wildcard global symbols are not permitted.')

    # Line is now in the format "<symbol-name>; # tags"
    # Tags are whitespace separated.
    symbol_name, _, rest = line.strip().partition(';')
    _, _, all_tags = rest.partition('#')
    tags = re.split(r'\s+', all_tags)

    if not symbol_in_arch(tags, arch):
        return
    if not symbol_in_version(tags, arch, api):
        return

    if 'var' in tags:
        src_file.write('int {} = 0;\n'.format(symbol_name))
    else:
        src_file.write('void {}() {{}}\n'.format(symbol_name))
    version_file.write(line)


def generate(symbol_file, src_file, version_file, arch, api):
    """Generates the stub source file and version script."""
    scope = Stack()
    scope.push(Scope.Top)
    for line in symbol_file:
        if line.strip() == '' or line.strip().startswith('#'):
            version_file.write(line)
        elif scope.top == Scope.Top:
            handle_top_scope(scope, line, version_file)
        elif scope.top == Scope.Private:
            handle_private_scope(scope, line, version_file)
        elif scope.top == Scope.Local:
            handle_local_scope(scope, line, version_file)
        elif scope.top == Scope.Global:
            handle_global_scope(scope, line, src_file, version_file, arch, api)


def parse_args():
    """Parses and returns command line arguments."""
    parser = argparse.ArgumentParser()

    parser.add_argument('--api', type=int, help='API level being targeted.')
    parser.add_argument(
        '--arch', choices=ALL_ARCHITECTURES,
        help='Architecture being targeted.')

    parser.add_argument(
        'symbol_file', type=os.path.realpath, help='Path to symbol file.')
    parser.add_argument(
        'stub_src', type=os.path.realpath,
        help='Path to output stub source file.')
    parser.add_argument(
        'version_script', type=os.path.realpath,
        help='Path to output version script.')

    return parser.parse_args()


def main():
    """Program entry point."""
    args = parse_args()

    with open(args.symbol_file) as symbol_file:
        with open(args.stub_src, 'w') as src_file:
            with open(args.version_script, 'w') as version_file:
                generate(symbol_file, src_file, version_file, args.arch,
                         args.api)


if __name__ == '__main__':
    main()
