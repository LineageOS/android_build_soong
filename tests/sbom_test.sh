#!/bin/bash

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

set -uo pipefail

# Integration test for verifying generated SBOM for cuttlefish device.

if [ ! -e "build/make/core/Makefile" ]; then
  echo "$0 must be run from the top of the Android source tree."
  exit 1
fi

tmp_dir="$(mktemp -d tmp.XXXXXX)"
function cleanup {
  rm -rf "${tmp_dir}"
}
trap cleanup EXIT

out_dir=$tmp_dir
droid_target=droid

debug=false
if [ $debug = "true" ]; then
  out_dir=out
  droid_target=
fi
# m droid, build sbom later in case additional dependencies might be built and included in partition images.
TARGET_PRODUCT="aosp_cf_x86_64_phone" TARGET_BUILD_VARIANT=userdebug OUT_DIR=$out_dir \
  build/soong/soong_ui.bash --make-mode $droid_target dump.erofs

product_out=$out_dir/target/product/vsoc_x86_64
sbom_test=$product_out/sbom_test
mkdir $sbom_test
cp $product_out/*.img $sbom_test

# m sbom
TARGET_PRODUCT="aosp_cf_x86_64_phone" TARGET_BUILD_VARIANT=userdebug OUT_DIR=$out_dir \
  build/soong/soong_ui.bash --make-mode sbom

# Generate installed file list from .img files in PRODUCT_OUT
dump_erofs=$out_dir/host/linux-x86/bin/dump.erofs

declare -A diff_excludes
diff_excludes[odm]="-I /odm/lib/modules"
diff_excludes[vendor]=\
"-I /vendor/lib64/libkeystore2_crypto.so \
 -I /vendor/lib/modules \
 -I /vendor/odm"
diff_excludes[system]=\
"-I /acct/ \
 -I /adb_keys \
 -I /apex/ \
 -I /bin \
 -I /bugreports \
 -I /cache \
 -I /config/ \
 -I /d \
 -I /data/ \
 -I /data_mirror/ \
 -I /debug_ramdisk/ \
 -I /dev/ \
 -I /etc \
 -I /init \
 -I /init.environ.rc \
 -I /linkerconfig/ \
 -I /metadata/ \
 -I /mnt/ \
 -I /odm/app \
 -I /odm/bin \
 -I /odm_dlkm/etc \
 -I /odm/etc \
 -I /odm/firmware \
 -I /odm/framework \
 -I /odm/lib \
 -I /odm/lib64 \
 -I /odm/overlay \
 -I /odm/priv-app \
 -I /odm/usr \
 -I /oem/ \
 -I /postinstall/ \
 -I /proc/ \
 -I /product/ \
 -I /sdcard \
 -I /second_stage_resources/ \
 -I /storage/ \
 -I /sys/ \
 -I /system_dlkm/ \
 -I /system_ext/ \
 -I /system/lib64/android.hardware.confirmationui@1.0.so \
 -I /system/lib64/android.hardware.confirmationui-V1-ndk.so \
 -I /system/lib64/android.hardware.keymaster@4.1.so \
 -I /system/lib64/android.hardware.security.rkp-V3-ndk.so \
 -I /system/lib64/android.hardware.security.sharedsecret-V1-ndk.so \
 -I /system/lib64/android.security.compat-ndk.so \
 -I /system/lib64/libkeymaster4_1support.so \
 -I /system/lib64/libkeymint.so \
 -I /system/lib64/libkeystore2_aaid.so \
 -I /system/lib64/libkeystore2_apc_compat.so \
 -I /system/lib64/libkeystore2_crypto.so \
 -I /system/lib64/libkm_compat_service.so \
 -I /system/lib64/libkm_compat.so \
 -I /system/lib64/vndk-29 \
 -I /system/lib64/vndk-sp-29 \
 -I /system/lib/modules \
 -I /system/lib/vndk-29 \
 -I /system/lib/vndk-sp-29 \
 -I /system/product \
 -I /system/system_ext \
 -I /system/usr/icu \
 -I /system/vendor \
 -I /vendor/ \
 -I /vendor_dlkm/etc"

# Example output of dump.erofs is as below, and the data used in the test start
# at line 11. Column 1 is inode id, column 2 is inode type and column 3 is name.
# Each line is captured in variable "entry", sed is used to trim the leading
# spaces and cut is used to get field 1 every time. Once a field is extracted,
# "cut --complement" is used to remove the extracted field so next field can be
# processed in the same way and to be processed field is always field 1.
# Output of dump.erofs:
#     File : /
#     Size: 160  On-disk size: 160  directory
#     NID: 39   Links: 10   Layout: 2   Compression ratio: 100.00%
#     Inode size: 64   Extent size: 0   Xattr size: 16
#     Uid: 0   Gid: 0  Access: 0755/rwxr-xr-x
#     Timestamp: 2023-02-14 01:15:54.000000000
#
#            NID TYPE  FILENAME
#             39    2  .
#             39    2  ..
#             47    2  app
#        1286748    2  bin
#        1286754    2  etc
#        5304814    2  lib
#        5309056    2  lib64
#        5309130    2  media
#        5388910    2  overlay
#        5479537    2  priv-app
EROFS_IMAGES="\
  $sbom_test/product.img \
  $sbom_test/system.img \
  $sbom_test/system_ext.img \
  $sbom_test/system_dlkm.img \
  $sbom_test/system_other.img \
  $sbom_test/odm.img \
  $sbom_test/odm_dlkm.img \
  $sbom_test/vendor.img \
  $sbom_test/vendor_dlkm.img"
for f in $EROFS_IMAGES; do
  partition_name=$(basename $f | cut -d. -f1)
  file_list_file="${sbom_test}/sbom-${partition_name}-files.txt"
  files_in_spdx_file="${sbom_test}/sbom-${partition_name}-files-in-spdx.txt"
  rm "$file_list_file" > /dev/null 2>&1
  all_dirs="/"
  while [ ! -z "$all_dirs" ]; do
    dir=$(echo "$all_dirs" | cut -d ' ' -f1)
    all_dirs=$(echo "$all_dirs" | cut -d ' ' -f1 --complement -s)
    entries=$($dump_erofs --ls --path "$dir" $f | tail -n +11)
    while read -r entry; do
      nid=$(echo $entry | sed 's/^\s*//' | cut -d ' ' -f1)
      entry=$(echo $entry | sed 's/^\s*//' | cut -d ' ' -f1 --complement)
      type=$(echo $entry | sed 's/^\s*//' | cut -d ' ' -f1)
      entry=$(echo $entry | sed 's/^\s*//' | cut -d ' ' -f1 --complement)
      name=$(echo $entry | sed 's/^\s*//' | cut -d ' ' -f1)
      case $type in
        "2")  # directory
          all_dirs=$(echo "$all_dirs $dir/$name" | sed 's/^\s*//')
          ;;
        *)
          (
          if [ "$partition_name" != "system" ]; then
            # system partition is mounted to /, not to prepend partition name.
            printf %s "/$partition_name"
          fi
          echo "$dir/$name" | sed 's#^//#/#'
          ) >> "$file_list_file"
          ;;
      esac
    done <<< "$entries"
  done
  sort -n -o "$file_list_file" "$file_list_file"

  # Diff
  echo ============ Diffing files in $f and SBOM
  grep "FileName: /${partition_name}/" $product_out/sbom.spdx | sed 's/^FileName: //' | sort -n > "$files_in_spdx_file"
  exclude=
  if [ -v 'diff_excludes[$partition_name]' ]; then
    exclude=${diff_excludes[$partition_name]}
  fi
  diff "$file_list_file" "$files_in_spdx_file" $exclude
  if [ $? != "0" ]; then
    echo Found diffs in $f and SBOM.
    exit 1
  else
    echo No diffs.
  fi
done