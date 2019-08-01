#!/usr/bin/env python
#
# Copyright (C) 2019 The Android Open Source Project
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


import argparse
import collections
import json
import sys

def follow_path(obj, path):
  cur = obj
  last_key = None
  for key in path.split('.'):
    if last_key:
      if last_key not in cur:
        return None,None
      cur = cur[last_key]
    last_key = key
  if last_key not in cur:
    return None,None
  return cur, last_key


def ensure_path(obj, path):
  cur = obj
  last_key = None
  for key in path.split('.'):
    if last_key:
      if last_key not in cur:
        cur[last_key] = dict()
      cur = cur[last_key]
    last_key = key
  return cur, last_key


class SetValue(str):
  def apply(self, obj, val):
    cur, key = ensure_path(obj, self)
    cur[key] = val


class Replace(str):
  def apply(self, obj, val):
    cur, key = follow_path(obj, self)
    if cur:
      cur[key] = val


class Remove(str):
  def apply(self, obj):
    cur, key = follow_path(obj, self)
    if cur:
      del cur[key]


class AppendList(str):
  def apply(self, obj, *args):
    cur, key = ensure_path(obj, self)
    if key not in cur:
      cur[key] = list()
    if not isinstance(cur[key], list):
      raise ValueError(self + " should be a array.")
    cur[key].extend(args)


def main():
  parser = argparse.ArgumentParser()
  parser.add_argument('-o', '--out',
                      help='write result to a file. If omitted, print to stdout',
                      metavar='output',
                      action='store')
  parser.add_argument('input', nargs='?', help='JSON file')
  parser.add_argument("-v", "--value", type=SetValue,
                      help='set value of the key specified by path. If path doesn\'t exist, creates new one.',
                      metavar=('path', 'value'),
                      nargs=2, dest='patch', default=[], action='append')
  parser.add_argument("-s", "--replace", type=Replace,
                      help='replace value of the key specified by path. If path doesn\'t exist, no op.',
                      metavar=('path', 'value'),
                      nargs=2, dest='patch', action='append')
  parser.add_argument("-r", "--remove", type=Remove,
                      help='remove the key specified by path. If path doesn\'t exist, no op.',
                      metavar='path',
                      nargs=1, dest='patch', action='append')
  parser.add_argument("-a", "--append_list", type=AppendList,
                      help='append values to the list specified by path. If path doesn\'t exist, creates new list for it.',
                      metavar=('path', 'value'),
                      nargs='+', dest='patch', default=[], action='append')
  args = parser.parse_args()

  if args.input:
    with open(args.input) as f:
      obj = json.load(f, object_pairs_hook=collections.OrderedDict)
  else:
    obj = json.load(sys.stdin, object_pairs_hook=collections.OrderedDict)

  for p in args.patch:
    p[0].apply(obj, *p[1:])

  if args.out:
    with open(args.out, "w") as f:
      json.dump(obj, f, indent=2)
  else:
    print(json.dumps(obj, indent=2))


if __name__ == '__main__':
  main()
