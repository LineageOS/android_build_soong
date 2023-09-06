#!/usr/bin/env python3

import subprocess
import argparse
import re
import sys
import zipfile

def check_target_sdk_less_than_30(args):
    if not args.aapt2:
        sys.exit('--aapt2 is required')
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

def has_preprocessed_issues(args, *, fail=False):
    if not args.zipalign:
        sys.exit('--zipalign is required')
    ret = subprocess.run([args.zipalign, '-c', '-p', '4', args.apk], stdout=subprocess.DEVNULL).returncode
    if ret != 0:
        if fail:
            sys.exit(args.apk + ': Improper zip alignment')
        return True

    with zipfile.ZipFile(args.apk) as zf:
        for info in zf.infolist():
            if info.filename.startswith('lib/') and info.filename.endswith('.so') and info.compress_type != zipfile.ZIP_STORED:
                if fail:
                    sys.exit(args.apk + ': Contains compressed JNI libraries')
                return True
            # It's ok for non-privileged apps to have compressed dex files, see go/gms-uncompressed-jni-slides
            if args.privileged:
                if info.filename.endswith('.dex') and info.compress_type != zipfile.ZIP_STORED:
                    if fail:
                        sys.exit(args.apk + ': Contains compressed dex files and is privileged')
                    return True
    return False


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--aapt2', help = "the path to the aapt2 executable")
    parser.add_argument('--zipalign', help = "the path to the zipalign executable")
    parser.add_argument('--skip-preprocessed-apk-checks', action = 'store_true', help = "the value of the soong property with the same name")
    parser.add_argument('--preprocessed', action = 'store_true', help = "the value of the soong property with the same name")
    parser.add_argument('--privileged', action = 'store_true', help = "the value of the soong property with the same name")
    parser.add_argument('apk', help = "the apk to check")
    parser.add_argument('stampfile', help = "a file to touch if successful")
    args = parser.parse_args()

    if not args.preprocessed:
        check_target_sdk_less_than_30(args)
    elif args.skip_preprocessed_apk_checks:
        if not has_preprocessed_issues(args):
            sys.exit('This module sets `skip_preprocessed_apk_checks: true`, but does not actually have any issues. Please remove `skip_preprocessed_apk_checks`.')
    else:
        has_preprocessed_issues(args, fail=True)

    subprocess.check_call(["touch", args.stampfile])

if __name__ == "__main__":
    main()
