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
"""Parser for Android's version script information."""
from __future__ import annotations

from dataclasses import dataclass, field
import logging
import re
from typing import (
    Dict,
    Iterable,
    Iterator,
    List,
    Mapping,
    NewType,
    Optional,
    TextIO,
    Tuple,
    Union,
)


ApiMap = Mapping[str, int]
Arch = NewType('Arch', str)
Tag = NewType('Tag', str)


ALL_ARCHITECTURES = (
    Arch('arm'),
    Arch('arm64'),
    Arch('riscv64'),
    Arch('x86'),
    Arch('x86_64'),
)

# TODO: it would be nice to dedupe with 'has_*_tag' property methods
SUPPORTED_TAGS = ALL_ARCHITECTURES + (
    Tag('apex'),
    Tag('llndk'),
    Tag('platform-only'),
    Tag('systemapi'),
    Tag('var'),
    Tag('weak'),
)

# Arbitrary magic number. We use the same one in api-level.h for this purpose.
FUTURE_API_LEVEL = 10000


def logger() -> logging.Logger:
    """Return the main logger for this module."""
    return logging.getLogger(__name__)


@dataclass
class Tags:
    """Container class for the tags attached to a symbol or version."""

    tags: tuple[Tag, ...] = field(default_factory=tuple)

    @classmethod
    def from_strs(cls, strs: Iterable[str]) -> Tags:
        """Constructs tags from a collection of strings.

        Does not decode API levels.
        """
        return Tags(tuple(Tag(s) for s in strs))

    def __contains__(self, tag: Union[Tag, str]) -> bool:
        return tag in self.tags

    def __iter__(self) -> Iterator[Tag]:
        yield from self.tags

    @property
    def has_mode_tags(self) -> bool:
        """Returns True if any mode tags (apex, llndk, etc) are set."""
        return self.has_apex_tags or self.has_llndk_tags or self.has_systemapi_tags

    @property
    def has_apex_tags(self) -> bool:
        """Returns True if any APEX tags are set."""
        return 'apex' in self.tags

    @property
    def has_systemapi_tags(self) -> bool:
        """Returns True if any APEX tags are set."""
        return 'systemapi' in self.tags

    @property
    def has_llndk_tags(self) -> bool:
        """Returns True if any LL-NDK tags are set."""
        return 'llndk' in self.tags

    @property
    def has_platform_only_tags(self) -> bool:
        """Returns True if any platform-only tags are set."""
        return 'platform-only' in self.tags


@dataclass
class Symbol:
    """A symbol definition from a symbol file."""

    name: str
    tags: Tags


@dataclass
class Version:
    """A version block of a symbol file."""

    name: str
    base: Optional[str]
    tags: Tags
    symbols: List[Symbol]

    @property
    def is_private(self) -> bool:
        """Returns True if this version block is private (platform only)."""
        return self.name.endswith('_PRIVATE') or self.name.endswith('_PLATFORM')


def get_tags(line: str, api_map: ApiMap) -> Tags:
    """Returns a list of all tags on this line."""
    _, _, all_tags = line.strip().partition('#')
    return Tags(tuple(
        decode_api_level_tag(Tag(e), api_map)
        for e in re.split(r'\s+', all_tags) if e.strip()
    ))


def is_api_level_tag(tag: Tag) -> bool:
    """Returns true if this tag has an API level that may need decoding."""
    if tag.startswith('llndk-deprecated='):
        return True
    if tag.startswith('introduced='):
        return True
    if tag.startswith('introduced-'):
        return True
    if tag.startswith('versioned='):
        return True
    return False


def decode_api_level(api: str, api_map: ApiMap) -> int:
    """Decodes the API level argument into the API level number.

    For the average case, this just decodes the integer value from the string,
    but for unreleased APIs we need to translate from the API codename (like
    "O") to the future API level for that codename.
    """
    try:
        return int(api)
    except ValueError:
        pass

    if api == "current":
        return FUTURE_API_LEVEL

    return api_map[api]


def decode_api_level_tag(tag: Tag, api_map: ApiMap) -> Tag:
    """Decodes API level code name in a tag.

    Raises:
        ParseError: An unknown version name was found in a tag.
    """
    if not is_api_level_tag(tag):
        if tag not in SUPPORTED_TAGS:
            raise ParseError(f'Unsupported tag: {tag}')

        return tag

    name, value = split_tag(tag)
    try:
        decoded = str(decode_api_level(value, api_map))
        return Tag(f'{name}={decoded}')
    except KeyError as ex:
        raise ParseError(f'Unknown version name in tag: {tag}') from ex


def split_tag(tag: Tag) -> Tuple[str, str]:
    """Returns a key/value tuple of the tag.

    Raises:
        ValueError: Tag is not a key/value type tag.

    Returns: Tuple of (key, value) of the tag. Both components are strings.
    """
    if '=' not in tag:
        raise ValueError('Not a key/value tag: ' + tag)
    key, _, value = tag.partition('=')
    return key, value


def get_tag_value(tag: Tag) -> str:
    """Returns the value of a key/value tag.

    Raises:
        ValueError: Tag is not a key/value type tag.

    Returns: Value part of tag as a string.
    """
    return split_tag(tag)[1]

class Filter:
    """A filter encapsulates a condition that tells whether a version or a
    symbol should be omitted or not
    """

    def __init__(self, arch: Arch, api: int, llndk: bool = False, apex: bool = False, systemapi:
                 bool = False, ndk: bool = True):
        self.arch = arch
        self.api = api
        self.llndk = llndk
        self.apex = apex
        self.systemapi = systemapi
        self.ndk = ndk

    def _should_omit_tags(self, tags: Tags) -> bool:
        """Returns True if the tagged object should be omitted.

        This defines the rules shared between version tagging and symbol tagging.
        """
        # The apex and llndk tags will only exclude APIs from other modes. If in
        # APEX or LLNDK mode and neither tag is provided, we fall back to the
        # default behavior because all NDK symbols are implicitly available to
        # APEX and LLNDK.
        if tags.has_mode_tags:
            if self.apex and tags.has_apex_tags:
                return False
            if self.llndk and tags.has_llndk_tags:
                return False
            if self.systemapi and tags.has_systemapi_tags:
                return False
            return True
        if not symbol_in_arch(tags, self.arch):
            return True
        if not symbol_in_api(tags, self.arch, self.api):
            return True
        return False

    def should_omit_version(self, version: Version) -> bool:
        """Returns True if the version section should be omitted.

        We want to omit any sections that do not have any symbols we'll have in
        the stub library. Sections that contain entirely future symbols or only
        symbols for certain architectures.
        """
        if version.is_private:
            return True
        if version.tags.has_platform_only_tags:
            return True
        return self._should_omit_tags(version.tags)

    def should_omit_symbol(self, symbol: Symbol) -> bool:
        """Returns True if the symbol should be omitted."""
        if not symbol.tags.has_mode_tags and not self.ndk:
            # Symbols that don't have mode tags are NDK. They are usually
            # included, but have to be omitted if NDK symbols are explicitly
            # filtered-out
            return True

        return self._should_omit_tags(symbol.tags)

def symbol_in_arch(tags: Tags, arch: Arch) -> bool:
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


def symbol_in_api(tags: Iterable[Tag], arch: Arch, api: int) -> bool:
    """Returns true if the symbol is present for the given API level."""
    introduced_tag = None
    arch_specific = False
    for tag in tags:
        # If there is an arch-specific tag, it should override the common one.
        if tag.startswith('introduced=') and not arch_specific:
            introduced_tag = tag
        elif tag.startswith('introduced-' + arch + '='):
            introduced_tag = tag
            arch_specific = True
        elif tag == 'future':
            return api == FUTURE_API_LEVEL

    if introduced_tag is None:
        # We found no "introduced" tags, so the symbol has always been
        # available.
        return True

    return api >= int(get_tag_value(introduced_tag))


def symbol_versioned_in_api(tags: Iterable[Tag], api: int) -> bool:
    """Returns true if the symbol should be versioned for the given API.

    This models the `versioned=API` tag. This should be a very uncommonly
    needed tag, and is really only needed to fix versioning mistakes that are
    already out in the wild.

    For example, some of libc's __aeabi_* functions were originally placed in
    the private version, but that was incorrect. They are now in LIBC_N, but
    when building against any version prior to N we need the symbol to be
    unversioned (otherwise it won't resolve on M where it is private).
    """
    for tag in tags:
        if tag.startswith('versioned='):
            return api >= int(get_tag_value(tag))
    # If there is no "versioned" tag, the tag has been versioned for as long as
    # it was introduced.
    return True


class ParseError(RuntimeError):
    """An error that occurred while parsing a symbol file."""


class MultiplyDefinedSymbolError(RuntimeError):
    """A symbol name was multiply defined."""
    def __init__(self, multiply_defined_symbols: Iterable[str]) -> None:
        super().__init__(
            'Version script contains multiple definitions for: {}'.format(
                ', '.join(multiply_defined_symbols)))
        self.multiply_defined_symbols = multiply_defined_symbols


class SymbolFileParser:
    """Parses NDK symbol files."""
    def __init__(self, input_file: TextIO, api_map: ApiMap, filt: Filter) -> None:
        self.input_file = input_file
        self.api_map = api_map
        self.filter = filt
        self.current_line: Optional[str] = None

    def parse(self) -> List[Version]:
        """Parses the symbol file and returns a list of Version objects."""
        versions = []
        while self.next_line():
            assert self.current_line is not None
            if '{' in self.current_line:
                versions.append(self.parse_version())
            else:
                raise ParseError(
                    f'Unexpected contents at top level: {self.current_line}')

        self.check_no_duplicate_symbols(versions)
        return versions

    def check_no_duplicate_symbols(self, versions: Iterable[Version]) -> None:
        """Raises errors for multiply defined symbols.

        This situation is the normal case when symbol versioning is actually
        used, but this script doesn't currently handle that. The error message
        will be a not necessarily obvious "error: redefition of 'foo'" from
        stub.c, so it's better for us to catch this situation and raise a
        better error.
        """
        symbol_names = set()
        multiply_defined_symbols = set()
        for version in versions:
            if self.filter.should_omit_version(version):
                continue

            for symbol in version.symbols:
                if self.filter.should_omit_symbol(symbol):
                    continue

                if symbol.name in symbol_names:
                    multiply_defined_symbols.add(symbol.name)
                symbol_names.add(symbol.name)
        if multiply_defined_symbols:
            raise MultiplyDefinedSymbolError(
                sorted(list(multiply_defined_symbols)))

    def parse_version(self) -> Version:
        """Parses a single version section and returns a Version object."""
        assert self.current_line is not None
        name = self.current_line.split('{')[0].strip()
        tags = get_tags(self.current_line, self.api_map)
        symbols: List[Symbol] = []
        global_scope = True
        cpp_symbols = False
        while self.next_line():
            if '}' in self.current_line:
                # Line is something like '} BASE; # tags'. Both base and tags
                # are optional here.
                base = self.current_line.partition('}')[2]
                base = base.partition('#')[0].strip()
                if not base.endswith(';'):
                    raise ParseError(
                        'Unterminated version/export "C++" block (expected ;).')
                if cpp_symbols:
                    cpp_symbols = False
                else:
                    base = base.rstrip(';').rstrip()
                    return Version(name, base or None, tags, symbols)
            elif 'extern "C++" {' in self.current_line:
                cpp_symbols = True
            elif not cpp_symbols and ':' in self.current_line:
                visibility = self.current_line.split(':')[0].strip()
                if visibility == 'local':
                    global_scope = False
                elif visibility == 'global':
                    global_scope = True
                else:
                    raise ParseError('Unknown visiblity label: ' + visibility)
            elif global_scope and not cpp_symbols:
                symbols.append(self.parse_symbol())
            else:
                # We're in a hidden scope or in 'extern "C++"' block. Ignore
                # everything.
                pass
        raise ParseError('Unexpected EOF in version block.')

    def parse_symbol(self) -> Symbol:
        """Parses a single symbol line and returns a Symbol object."""
        assert self.current_line is not None
        if ';' not in self.current_line:
            raise ParseError(
                'Expected ; to terminate symbol: ' + self.current_line)
        if '*' in self.current_line:
            raise ParseError(
                'Wildcard global symbols are not permitted.')
        # Line is now in the format "<symbol-name>; # tags"
        name, _, _ = self.current_line.strip().partition(';')
        tags = get_tags(self.current_line, self.api_map)
        return Symbol(name, tags)

    def next_line(self) -> str:
        """Returns the next non-empty non-comment line.

        A return value of '' indicates EOF.
        """
        line = self.input_file.readline()
        while not line.strip() or line.strip().startswith('#'):
            line = self.input_file.readline()

            # We want to skip empty lines, but '' indicates EOF.
            if not line:
                break
        self.current_line = line
        return self.current_line
