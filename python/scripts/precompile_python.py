#!/usr/bin/env python3
# Copyright 2023 Google Inc. All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import argparse
import py_compile
import os
import sys
import shutil
import tempfile
import zipfile

# This file needs to support both python 2 and 3.


def process_one_file(name, infile, outzip):
    # Create a ZipInfo instance with a fixed date to ensure a deterministic output.
    # Date was chosen to be the same as
    # https://cs.android.com/android/platform/superproject/main/+/main:build/soong/jar/jar.go;l=36;drc=2863e4535eb65e15f955dc8ed48fa99b1d2a1db5
    info = zipfile.ZipInfo(filename=name, date_time=(2008, 1, 1, 0, 0, 0))

    if not info.filename.endswith('.py'):
        outzip.writestr(info, infile.read())
        return

    # Unfortunately py_compile requires the input/output files to be written
    # out to disk.
    with tempfile.NamedTemporaryFile(prefix="Soong_precompile_", delete=False) as tmp:
        shutil.copyfileobj(infile, tmp)
        in_name = tmp.name
    with tempfile.NamedTemporaryFile(prefix="Soong_precompile_", delete=False) as tmp:
        out_name = tmp.name
    try:
        # Ensure a deterministic .pyc output by using the hash rather than the timestamp.
        # Only works on Python 3.7+
        # See https://docs.python.org/3/library/py_compile.html#py_compile.PycInvalidationMode
        if sys.version_info >= (3, 7):
            py_compile.compile(in_name, out_name, info.filename, doraise=True, invalidation_mode=py_compile.PycInvalidationMode.CHECKED_HASH)
        else:
            py_compile.compile(in_name, out_name, info.filename, doraise=True)
        with open(out_name, 'rb') as f:
            info.filename = info.filename + 'c'
            outzip.writestr(info, f.read())
    finally:
        os.remove(in_name)
        os.remove(out_name)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('src_zip')
    parser.add_argument('dst_zip')
    args = parser.parse_args()

    with open(args.dst_zip, 'wb') as outf, open(args.src_zip, 'rb') as inf:
        with zipfile.ZipFile(outf, mode='w') as outzip, zipfile.ZipFile(inf, mode='r') as inzip:
            for name in inzip.namelist():
                with inzip.open(name, mode='r') as inzipf:
                    process_one_file(name, inzipf, outzip)


if __name__ == "__main__":
    main()
