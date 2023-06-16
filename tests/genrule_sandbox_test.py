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
import os.path
import subprocess
import tempfile

SRC_ROOT_DIR = os.path.abspath(__file__ + "/../../../..")


def _module_graph_path(out_dir):
  return os.path.join(SRC_ROOT_DIR, out_dir, "soong", "module-actions.json")


def _build_with_soong(targets, target_product, out_dir, extra_env={}):
  env = {
      "TARGET_PRODUCT": target_product,
      "TARGET_BUILD_VARIANT": "userdebug",
  }
  env.update(os.environ)
  env.update(extra_env)
  args = [
      "build/soong/soong_ui.bash",
      "--make-mode",
      "--skip-soong-tests",
  ]
  args.extend(targets)
  try:
    out = subprocess.check_output(
        args,
        cwd=SRC_ROOT_DIR,
        env=env,
    )
  except subprocess.CalledProcessError as e:
    print(e)
    print(e.stdout)
    print(e.stderr)
    exit(1)


def _find_outputs_for_modules(modules, out_dir, target_product):
  module_path = os.path.join(
      SRC_ROOT_DIR, out_dir, "soong", "module-actions.json"
  )

  if not os.path.exists(module_path):
    _build_with_soong(["json-module-graph"], target_product, out_dir)

  action_graph = json.load(open(_module_graph_path(out_dir)))

  module_to_outs = collections.defaultdict(set)
  for mod in action_graph:
    name = mod["Name"]
    if name in modules:
      for act in mod["Module"]["Actions"]:
        if "}generate" in act["Desc"]:
          module_to_outs[name].update(act["Outputs"])
  return module_to_outs


def _store_outputs_to_tmp(output_files):
  try:
    tempdir = tempfile.TemporaryDirectory()
    for f in output_files:
      out = subprocess.check_output(
          ["cp", "--parents", f, tempdir.name],
          cwd=SRC_ROOT_DIR,
      )
    return tempdir
  except subprocess.CalledProcessError as e:
    print(e)
    print(e.stdout)
    print(e.stderr)


def _diff_outs(file1, file2, show_diff):
  output = None
  base_args = ["diff"]
  if not show_diff:
    base_args.append("--brief")
  try:
    args = base_args + [file1, file2]
    output = subprocess.check_output(
        args,
        cwd=SRC_ROOT_DIR,
    )
  except subprocess.CalledProcessError as e:
    if e.returncode == 1:
      if show_diff:
        return output
      return True
  return None


def _compare_outputs(module_to_outs, tempdir, show_diff):
  different_modules = collections.defaultdict(list)
  for module, outs in module_to_outs.items():
    for out in outs:
      output = None
      diff = _diff_outs(os.path.join(tempdir.name, out), out, show_diff)
      if diff:
        different_modules[module].append(diff)

  tempdir.cleanup()
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
      required=False,
      help="whether to display differing files",
  )
  parser.add_argument(
      "--output-paths-only",
      "-o",
      action="store_true",
      required=False,
      help="Whether to only return the output paths per module",
  )
  args = parser.parse_args()

  out_dir = os.environ.get("OUT_DIR", "out")
  target_product = args.target_product
  modules = set(args.modules)

  module_to_outs = _find_outputs_for_modules(modules, out_dir, target_product)
  if not module_to_outs:
    print("No outputs found")
    exit(1)

  if args.output_paths_only:
    for m, o in module_to_outs.items():
      print(f"{m} outputs: {o}")
    exit(0)

  all_outs = set()
  for outs in module_to_outs.values():
    all_outs.update(outs)
  print("build without sandboxing")
  _build_with_soong(list(all_outs), target_product, out_dir)
  tempdir = _store_outputs_to_tmp(all_outs)
  print("build with sandboxing")
  _build_with_soong(
      list(all_outs),
      target_product,
      out_dir,
      extra_env={"GENRULE_SANDBOXING": "true"},
  )
  diffs = _compare_outputs(module_to_outs, tempdir, args.show_diff)
  if len(diffs) == 0:
    print("All modules are correct")
  elif args.show_diff:
    for m, d in diffs.items():
      print(f"Module {m} has diffs {d}")
  else:
    print(f"Modules {list(diffs.keys())} have diffs")


if __name__ == "__main__":
  main()
