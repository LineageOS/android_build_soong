# Copyright (C) 2024 The Android Open Source Project
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

import json
import os
import pathlib
import sys


def OpenModuleInfoFile():
    product_out = os.getenv("ANDROID_PRODUCT_OUT")
    if not product_out:
        if os.getenv("QUIET_VERIFYMODINFO") != "true":
            sys.stderr.write("No ANDROID_PRODUCT_OUT. Try running 'lunch' first.\n")
        sys.exit(1)
    try:
        return open(pathlib.Path(product_out) / "module-info.json")
    except (FileNotFoundError, PermissionError):
        if os.getenv("QUIET_VERIFYMODINFO") != "true":
            sys.stderr.write("Could not find module-info.json. Please run 'refreshmod' first.\n")
        sys.exit(1)


def ReadModuleInfo():
    with OpenModuleInfoFile() as f:
        return json.load(f)

def GetModule(modules, module_name):
    if module_name not in modules:
        sys.stderr.write(f"Could not find module '{module_name}' (try 'refreshmod' if there have been build changes?)\n")
        sys.exit(1)
    return modules[module_name]

