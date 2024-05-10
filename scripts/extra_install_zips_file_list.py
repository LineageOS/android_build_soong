#!/usr/bin/env python3

import argparse
import os
import sys
import zipfile
from typing import List

def list_files_in_zip(zipfile_path: str) -> List[str]:
    with zipfile.ZipFile(zipfile_path, 'r') as zf:
        return zf.namelist()

def main():
    parser = argparse.ArgumentParser(
        description='Lists paths to all files inside an EXTRA_INSTALL_ZIPS zip file relative to a partition staging directory. '
        'This script is just a helper because its difficult to implement this logic in make code.'
    )
    parser.add_argument('staging_dir',
        help='Path to the partition staging directory')
    parser.add_argument('extra_install_zips', nargs='*',
        help='The value of EXTRA_INSTALL_ZIPS from make. It should be a list of extraction_dir:zip_file pairs.')
    args = parser.parse_args()

    staging_dir = args.staging_dir.removesuffix('/') + '/'

    for zip_pair in args.extra_install_zips:
        d, z = zip_pair.split(':')
        d = d.removesuffix('/') + '/'

        if d.startswith(staging_dir):
            d = os.path.relpath(d, staging_dir)
            if d == '.':
                d = ''
            for f in list_files_in_zip(z):
                print(os.path.join(d, f))


if __name__ == "__main__":
    main()
