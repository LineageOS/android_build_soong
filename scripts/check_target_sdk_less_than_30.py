#!/usr/bin/env python3

import subprocess
import argparse
import re
import sys

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('aapt2', help = "the path to the aapt2 executable")
    parser.add_argument('apk', help = "the apk to check")
    parser.add_argument('stampfile', help = "a file to touch if successful")
    args = parser.parse_args()

    regex = re.compile(r"targetSdkVersion: *'([0-9]+)'")
    output = subprocess.check_output([args.aapt2, "dump", "badging", args.apk], text=True)
    targetSdkVersion = None
    for line in output.splitlines():
        match = regex.fullmatch(line.strip())
        if match:
            targetSdkVersion = int(match.group(1))
            break

    if targetSdkVersion is None or targetSdkVersion >= 30:
        sys.exit(args.apk + ": Prebuilt, presigned apks with targetSdkVersion >= 30 (or a codename targetSdkVersion) must set preprocessed: true in the Android.bp definition (because they must be signed with signature v2, and the build system would wreck that signature otherwise)")

    subprocess.check_call(["touch", args.stampfile])

if __name__ == "__main__":
    main()
