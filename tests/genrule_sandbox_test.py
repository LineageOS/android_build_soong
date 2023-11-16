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
import asyncio
import collections
import json
import os
import socket
import subprocess
import sys
import textwrap

def get_top() -> str:
  path = '.'
  while not os.path.isfile(os.path.join(path, 'build/soong/tests/genrule_sandbox_test.py')):
    if os.path.abspath(path) == '/':
      sys.exit('Could not find android source tree root.')
    path = os.path.join(path, '..')
  return os.path.abspath(path)

async def _build_with_soong(out_dir, targets, *, extra_env={}):
  env = os.environ | extra_env

  # Use nsjail to remap the out_dir to out/, because some genrules write the path to the out
  # dir into their artifacts, so if the out directories were different it would cause a diff
  # that doesn't really matter.
  args = [
      'prebuilts/build-tools/linux-x86/bin/nsjail',
      '-q',
      '--cwd',
      os.getcwd(),
      '-e',
      '-B',
      '/',
      '-B',
      f'{os.path.abspath(out_dir)}:{os.path.abspath("out")}',
      '--time_limit',
      '0',
      '--skip_setsid',
      '--keep_caps',
      '--disable_clone_newcgroup',
      '--disable_clone_newnet',
      '--rlimit_as',
      'soft',
      '--rlimit_core',
      'soft',
      '--rlimit_cpu',
      'soft',
      '--rlimit_fsize',
      'soft',
      '--rlimit_nofile',
      'soft',
      '--proc_rw',
      '--hostname',
      socket.gethostname(),
      '--',
      "build/soong/soong_ui.bash",
      "--make-mode",
      "--skip-soong-tests",
  ]
  args.extend(targets)
  process = await asyncio.create_subprocess_exec(
      *args,
      stdout=asyncio.subprocess.PIPE,
      stderr=asyncio.subprocess.PIPE,
      env=env,
  )
  stdout, stderr = await process.communicate()
  if process.returncode != 0:
    print(stdout)
    print(stderr)
    sys.exit(process.returncode)


async def _find_outputs_for_modules(modules):
  module_path = "out/soong/module-actions.json"

  if not os.path.exists(module_path):
    await _build_with_soong('out', ["json-module-graph"])

  with open(module_path) as f:
    action_graph = json.load(f)

  module_to_outs = collections.defaultdict(set)
  for mod in action_graph:
    name = mod["Name"]
    if name in modules:
      for act in (mod["Module"]["Actions"] or []):
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


async def main():
  parser = argparse.ArgumentParser()
  parser.add_argument(
      "modules",
      nargs="+",
      help="modules to compare builds with genrule sandboxing enabled/not",
  )
  parser.add_argument(
      "--check-determinism",
      action="store_true",
      help="Don't check for working sandboxing. Instead, run two default builds, and compare their outputs. This is used to check for nondeterminsim, which would also affect the sandboxed test.",
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

  if "TARGET_PRODUCT" not in os.environ:
    sys.exit("Please run lunch first")
  if os.environ.get("OUT_DIR", "out") != "out":
    sys.exit(f"This script expects OUT_DIR to be 'out', got: '{os.environ.get('OUT_DIR')}'")

  print("finding output files for the modules...")
  module_to_outs = await _find_outputs_for_modules(set(args.modules))
  if not module_to_outs:
    sys.exit("No outputs found")

  if args.output_paths_only:
    for m, o in module_to_outs.items():
      print(f"{m} outputs: {o}")
    sys.exit(0)

  all_outs = list(set.union(*module_to_outs.values()))
  for i, out in enumerate(all_outs):
    if not out.startswith("out/"):
      sys.exit("Expected output file to start with out/, found: " + out)

  other_out_dir = "out_check_determinism" if args.check_determinism else "out_not_sandboxed"
  other_env = {"GENRULE_SANDBOXING": "false"}
  if args.check_determinism:
    other_env = {}

  # nsjail will complain if the out dir doesn't exist
  os.makedirs("out", exist_ok=True)
  os.makedirs(other_out_dir, exist_ok=True)

  print("building...")
  await asyncio.gather(
    _build_with_soong("out", all_outs),
    _build_with_soong(other_out_dir, all_outs, extra_env=other_env)
  )

  diffs = collections.defaultdict(dict)
  for module, outs in module_to_outs.items():
    for out in outs:
      try:
        subprocess.check_output(["diff", os.path.join(other_out_dir, out.removeprefix("out/")), out])
      except subprocess.CalledProcessError as e:
        diffs[module][out] = e.stdout

  if len(diffs) == 0:
    print("All modules are correct")
  elif args.show_diff:
    for m, files in diffs.items():
      print(f"Module {m} has diffs:")
      for f, d in files.items():
        print("  "+f+":")
        print(textwrap.indent(d, "    "))
  else:
    print(f"Modules {list(diffs.keys())} have diffs in these files:")
    all_diff_files = [f for m in diffs.values() for f in m]
    for f in all_diff_files:
      print(f)



if __name__ == "__main__":
  asyncio.run(main())
