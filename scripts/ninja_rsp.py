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

"""This file reads entries from a Ninja rsp file."""

class NinjaRspFileReader:
  """
  Reads entries from a Ninja rsp file.  Ninja escapes any entries in the file that contain a
  non-standard character by surrounding the whole entry with single quotes, and then replacing
  any single quotes in the entry with the escape sequence '\''.
  """

  def __init__(self, filename):
    self.f = open(filename, 'r')
    self.r = self.character_reader(self.f)

  def __iter__(self):
    return self

  def character_reader(self, f):
    """Turns a file into a generator that returns one character at a time."""
    while True:
      c = f.read(1)
      if c:
        yield c
      else:
        return

  def __next__(self):
    entry = self.read_entry()
    if entry:
      return entry
    else:
      raise StopIteration

  def read_entry(self):
    c = next(self.r, "")
    if not c:
      return ""
    elif c == "'":
      return self.read_quoted_entry()
    else:
      entry = c
      for c in self.r:
        if c == " " or c == "\n":
          break
        entry += c
      return entry

  def read_quoted_entry(self):
    entry = ""
    for c in self.r:
      if c == "'":
        # Either the end of the quoted entry, or the beginning of an escape sequence, read the next
        # character to find out.
        c = next(self.r)
        if not c or c == " " or c == "\n":
          # End of the item
          return entry
        elif c == "\\":
          # Escape sequence, expect a '
          c = next(self.r)
          if c != "'":
            # Malformed escape sequence
            raise "malformed escape sequence %s'\\%s" % (entry, c)
          entry += "'"
        else:
          raise "malformed escape sequence %s'%s" % (entry, c)
      else:
        entry += c
    raise "unterminated quoted entry %s" % entry
