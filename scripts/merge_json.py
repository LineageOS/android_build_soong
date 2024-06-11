#!/usr/bin/env python
#
# Copyright 2024 The Android Open Source Project
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
"""A tool for merging two or more JSON files."""

import argparse
import logging
import json
import sys

def parse_args():
  """Parse commandline arguments."""

  parser = argparse.ArgumentParser()
  parser.add_argument("output", help="output JSON file", type=argparse.FileType("w"))
  parser.add_argument("input", help="input JSON files", nargs="+", type=argparse.FileType("r"))
  return parser.parse_args()

def main():
  """Program entry point."""
  args = parse_args()
  merged_dict = {}
  has_error = False
  logger = logging.getLogger(__name__)

  for json_file in args.input:
    try:
      data = json.load(json_file)
    except json.JSONDecodeError as e:
      logger.error(f"Error parsing JSON in file: {json_file.name}. Reason: {e}")
      has_error = True
      continue

    for key, value in data.items():
      if key not in merged_dict:
        merged_dict[key] = value
      elif merged_dict[key] == value:
        logger.warning(f"Duplicate key '{key}' with identical values found.")
      else:
        logger.error(f"Conflicting values for key '{key}': {merged_dict[key]} != {value}")
        has_error = True

  if has_error:
    sys.exit(1)

  json.dump(merged_dict, args.output)

if __name__ == "__main__":
  main()
