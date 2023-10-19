#!/usr/bin/env python3

# Copyright (C) 2023 The Android Open Source Project
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
import os
import subprocess
import sys
import tempfile

def get_top() -> str:
  path = '.'
  while not os.path.isfile(os.path.join(path, 'build/soong/tests/genrule_sandbox_test.py')):
    if os.path.abspath(path) == '/':
      sys.exit('Could not find android source tree root.')
    path = os.path.join(path, '..')
  return os.path.abspath(path)

def _build_with_soong(targets, target_product, *, keep_going = False, extra_env={}):
  env = {
      **os.environ,
      "TARGET_PRODUCT": target_product,
      "TARGET_BUILD_VARIANT": "userdebug",
  }
  env.update(extra_env)
  args = [
      "build/soong/soong_ui.bash",
      "--make-mode",
      "--skip-soong-tests",
  ]
  if keep_going:
    args.append("-k")
  args.extend(targets)
  try:
    subprocess.check_output(
        args,
        env=env,
    )
  except subprocess.CalledProcessError as e:
    print(e)
    print(e.stdout)
    print(e.stderr)
    exit(1)


def _find_outputs_for_modules(modules, out_dir, target_product):
  module_path = os.path.join(out_dir, "soong", "module-actions.json")

  if not os.path.exists(module_path):
    # Use GENRULE_SANDBOXING=false so that we don't cause re-analysis later when we do the no-sandboxing build
    _build_with_soong(["json-module-graph"], target_product, extra_env={"GENRULE_SANDBOXING": "false"})

  with open(module_path) as f:
    action_graph = json.load(f)

  module_to_outs = collections.defaultdict(set)
  for mod in action_graph:
    name = mod["Name"]
    if name in modules:
      for act in mod["Module"]["Actions"]:
        if "}generate" in act["Desc"]:
          module_to_outs[name].update(act["Outputs"])
  return module_to_outs


def _compare_outputs(module_to_outs, tempdir) -> dict[str, list[str]]:
  different_modules = collections.defaultdict(list)
  for module, outs in module_to_outs.items():
    for out in outs:
      try:
        subprocess.check_output(["diff", os.path.join(tempdir, out), out])
      except subprocess.CalledProcessError as e:
        different_modules[module].append(e.stdout)

  return different_modules


def main():
  parser = argparse.ArgumentParser()
  parser.add_argument(
      "--target_product",
      "-t",
      default="aosp_cf_arm64_phone",
      help="optional, target product, always runs as eng",
  )
  parser.add_argument(
      "modules",
      nargs="+",
      help="modules to compare builds with genrule sandboxing enabled/not",
  )
  parser.add_argument(
      "--show-diff",
      "-d",
      action="store_true",
      help="whether to display differing files",
  )
  parser.add_argument(
      "--output-paths-only",
      "-o",
      action="store_true",
      help="Whether to only return the output paths per module",
  )
  args = parser.parse_args()
  os.chdir(get_top())

  out_dir = os.environ.get("OUT_DIR", "out")

  print("finding output files for the modules...")
  module_to_outs = _find_outputs_for_modules(set(args.modules), out_dir, args.target_product)
  if not module_to_outs:
    sys.exit("No outputs found")

  if args.output_paths_only:
    for m, o in module_to_outs.items():
      print(f"{m} outputs: {o}")
    sys.exit(0)

  all_outs = list(set.union(*module_to_outs.values()))

  print("building without sandboxing...")
  _build_with_soong(all_outs, args.target_product, extra_env={"GENRULE_SANDBOXING": "false"})
  with tempfile.TemporaryDirectory() as tempdir:
    for f in all_outs:
      subprocess.check_call(["cp", "--parents", f, tempdir])

    print("building with sandboxing...")
    _build_with_soong(
        all_outs,
        args.target_product,
        # We've verified these build without sandboxing already, so do the sandboxing build
        # with keep_going = True so that we can find all the genrules that fail to build with
        # sandboxing.
        keep_going = True,
        extra_env={"GENRULE_SANDBOXING": "true"},
    )

    diffs = _compare_outputs(module_to_outs, tempdir)
    if len(diffs) == 0:
      print("All modules are correct")
    elif args.show_diff:
      for m, d in diffs.items():
        print(f"Module {m} has diffs {d}")
    else:
      print(f"Modules {list(diffs.keys())} have diffs")


if __name__ == "__main__":
  main()
