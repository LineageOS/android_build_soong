#!/usr/bin/env python3
#
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
#

"""
This script is used as a replacement for the Rust linker. It converts a linker
command line into a rspfile that can be used during the link phase.
"""

import os
import shutil
import subprocess
import sys

def create_archive(out, objects, archives):
  mricmd = f'create {out}\n'
  for o in objects:
    mricmd += f'addmod {o}\n'
  for a in archives:
    mricmd += f'addlib {a}\n'
  mricmd += 'save\nend\n'
  subprocess.run([os.getenv('AR'), '-M'], encoding='utf-8', input=mricmd, check=True)

objects = []
archives = []
linkdirs = []
libs = []
temp_archives = []
version_script = None

for i, arg in enumerate(sys.argv):
  if arg == '-o':
    out = sys.argv[i+1]
  if arg == '-L':
    linkdirs.append(sys.argv[i+1])
  if arg.startswith('-l') or arg == '-shared':
    libs.append(arg)
  if arg.startswith('-Wl,--version-script='):
    version_script = arg[21:]
  if arg[0] == '-':
    continue
  if arg.endswith('.o') or arg.endswith('.rmeta'):
    objects.append(arg)
  if arg.endswith('.rlib'):
    if arg.startswith(os.getenv('TMPDIR')):
      temp_archives.append(arg)
    else:
      archives.append(arg)

create_archive(f'{out}.whole.a', objects, [])
create_archive(f'{out}.a', [], temp_archives)

with open(out, 'w') as f:
  print(f'-Wl,--whole-archive', file=f)
  print(f'{out}.whole.a', file=f)
  print(f'-Wl,--no-whole-archive', file=f)
  print(f'{out}.a', file=f)
  for a in archives:
    print(a, file=f)
  for linkdir in linkdirs:
    print(f'-L{linkdir}', file=f)
  for l in libs:
    print(l, file=f)
  if version_script:
    shutil.copyfile(version_script, f'{out}.version_script')
    print(f'-Wl,--version-script={out}.version_script', file=f)
