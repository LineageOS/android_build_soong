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

function setup {
  tmp_dir="$(mktemp -d tmp.XXXXXX)"
  trap 'cleanup "${tmp_dir}"' EXIT
  echo "${tmp_dir}"
}

function cleanup {
  tmp_dir="$1"; shift
  rm -rf "${tmp_dir}"
}

function run_soong {
  local out_dir="$1"; shift
  local targets="$1"; shift
  if [ "$#" -ge 1 ]; then
    local apps=$1; shift
    TARGET_PRODUCT="${target_product}" TARGET_RELEASE="${target_release}" TARGET_BUILD_VARIANT="${target_build_variant}" OUT_DIR="${out_dir}" TARGET_BUILD_UNBUNDLED=true TARGET_BUILD_APPS=$apps \
        build/soong/soong_ui.bash --make-mode ${targets}
  else
    TARGET_PRODUCT="${target_product}" TARGET_RELEASE="${target_release}" TARGET_BUILD_VARIANT="${target_build_variant}" OUT_DIR="${out_dir}" \
        build/soong/soong_ui.bash --make-mode ${targets}
  fi
}

function diff_files {
  local file_list_file="$1"; shift
  local files_in_spdx_file="$1"; shift
  local partition_name="$1"; shift
  local exclude="$1"; shift

  diff "$file_list_file" "$files_in_spdx_file" $exclude
  if [ $? != "0" ]; then
   echo Found diffs in $f and SBOM.
   exit 1
  else
   echo No diffs.
  fi
}

function test_sbom_aosp_cf_x86_64_phone {
  # Setup
  out_dir="$(setup)"

  # Test
  # m droid, build sbom later in case additional dependencies might be built and included in partition images.
  run_soong "${out_dir}" "droid dump.erofs lz4"

  product_out=$out_dir/target/product/vsoc_x86_64
  sbom_test=$product_out/sbom_test
  mkdir -p $sbom_test
  cp $product_out/*.img $sbom_test

  # m sbom
  run_soong "${out_dir}" sbom

  # Generate installed file list from .img files in PRODUCT_OUT
  dump_erofs=$out_dir/host/linux-x86/bin/dump.erofs
  lz4=$out_dir/host/linux-x86/bin/lz4

  # Example output of dump.erofs is as below, and the data used in the test start
  # at line 11. Column 1 is inode id, column 2 is inode type and column 3 is name.
  # Each line is captured in variable "entry", awk is used to get type and name.
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
    rm "$file_list_file" > /dev/null 2>&1 || true
    all_dirs="/"
    while [ ! -z "$all_dirs" ]; do
      dir=$(echo "$all_dirs" | cut -d ' ' -f1)
      all_dirs=$(echo "$all_dirs" | cut -d ' ' -f1 --complement -s)
      entries=$($dump_erofs --ls --path "$dir" $f | tail -n +11)
      while read -r entry; do
        inode_type=$(echo $entry | awk -F ' ' '{print $2}')
        name=$(echo $entry | awk -F ' ' '{print $3}')
        case $inode_type in
          "2")  # directory
            all_dirs=$(echo "$all_dirs $dir/$name" | sed 's/^\s*//')
            ;;
          "1"|"7")  # 1: file, 7: symlink
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

    grep "FileName: /${partition_name}/" $product_out/sbom.spdx | sed 's/^FileName: //' > "$files_in_spdx_file"
    if [ "$partition_name" = "system" ]; then
      # system partition is mounted to /, so include FileName starts with /root/ too.
      grep "FileName: /root/" $product_out/sbom.spdx | sed 's/^FileName: \/root//' >> "$files_in_spdx_file"
    fi
    sort -n -o "$files_in_spdx_file" "$files_in_spdx_file"

    echo ============ Diffing files in $f and SBOM
    diff_files "$file_list_file" "$files_in_spdx_file" "$partition_name" ""
  done

  RAMDISK_IMAGES="$product_out/ramdisk.img"
  for f in $RAMDISK_IMAGES; do
    partition_name=$(basename $f | cut -d. -f1)
    file_list_file="${sbom_test}/sbom-${partition_name}-files.txt"
    files_in_spdx_file="${sbom_test}/sbom-${partition_name}-files-in-spdx.txt"
    # lz4 decompress $f to stdout
    # cpio list all entries like ls -l
    # grep filter normal files and symlinks
    # awk get entry names
    # sed remove partition name from entry names
    $lz4 -c -d $f | cpio -tv 2>/dev/null | grep '^[-l]' | awk -F ' ' '{print $9}' | sed "s:^:/$partition_name/:" | sort -n > "$file_list_file"

    grep "FileName: /${partition_name}/" $product_out/sbom.spdx | sed 's/^FileName: //' | sort -n > "$files_in_spdx_file"

    echo ============ Diffing files in $f and SBOM
    diff_files "$file_list_file" "$files_in_spdx_file" "$partition_name" ""
  done

  verify_package_verification_code "$product_out/sbom.spdx"

  # Teardown
  cleanup "${out_dir}"
}

function verify_package_verification_code {
  local sbom_file="$1"; shift

  local -a file_checksums
  local package_product_found=
  while read -r line;
  do
    if grep -q 'PackageVerificationCode' <<<"$line"
    then
      package_product_found=true
    fi
    if [ -n "$package_product_found" ]
    then
      if grep -q 'FileChecksum' <<< "$line"
      then
        checksum=$(echo $line | sed 's/^.*: //')
        file_checksums+=("$checksum")
      fi
    fi
  done <<< "$(grep -E 'PackageVerificationCode|FileChecksum' $sbom_file)"
  IFS=$'\n' file_checksums=($(sort <<<"${file_checksums[*]}")); unset IFS
  IFS= expected_package_verification_code=$(printf "${file_checksums[*]}" | sha1sum | sed 's/[[:space:]]*-//'); unset IFS

  actual_package_verification_code=$(grep PackageVerificationCode $sbom_file | sed 's/PackageVerificationCode: //g')
  if [ $actual_package_verification_code = $expected_package_verification_code ]
  then
    echo "Package verification code is correct."
  else
    echo "Unexpected package verification code."
    exit 1
  fi
}

function test_sbom_unbundled_apex {
  # Setup
  out_dir="$(setup)"

  # run_soong to build com.android.adbd.apex
  run_soong "${out_dir}" "sbom deapexer" "com.android.adbd"

  deapexer=${out_dir}/host/linux-x86/bin/deapexer
  debugfs=${out_dir}/host/linux-x86/bin/debugfs_static
  apex_file=${out_dir}/target/product/module_arm64/system/apex/com.android.adbd.apex
  echo "============ Diffing files in $apex_file and SBOM"
  set +e
  # deapexer prints the list of all files and directories
  # sed extracts the file/directory names
  # grep removes directories
  # sed removes leading ./ in file names
  diff -I /system/apex/com.android.adbd.apex -I apex_manifest.pb \
      <($deapexer --debugfs_path=$debugfs list --extents ${apex_file} | sed -E 's#(.*) \[.*\]$#\1#' | grep -v "/$" | sed -E 's#^\./(.*)#\1#' | sort -n) \
      <(grep '"fileName": ' ${apex_file}.spdx.json | sed -E 's/.*"fileName": "(.*)",/\1/' | sort -n )

  if [ $? != "0" ]; then
    echo "Diffs found in $apex_file and SBOM"
    exit 1
  else
    echo "No diffs."
  fi
  set -e

  # Teardown
  cleanup "${out_dir}"
}

function test_sbom_unbundled_apk {
  # Setup
  out_dir="$(setup)"

  # run_soong to build Browser2.apk
  run_soong "${out_dir}" "sbom" "Browser2"

  sbom_file=${out_dir}/target/product/module_arm64/system/product/app/Browser2/Browser2.apk.spdx.json
  echo "============ Diffing files in Browser2.apk and SBOM"
  set +e
  # There is only one file in SBOM of APKs
  diff \
      <(echo "/system/product/app/Browser2/Browser2.apk" ) \
      <(grep '"fileName": ' ${sbom_file} | sed -E 's/.*"fileName": "(.*)",/\1/' )

  if [ $? != "0" ]; then
    echo "Diffs found in $sbom_file"
    exit 1
  else
    echo "No diffs."
  fi
  set -e

  # Teardown
  cleanup "${out_dir}"
}

target_product=aosp_cf_x86_64_phone
target_release=trunk_staging
target_build_variant=userdebug
for i in "$@"; do
  case $i in
    TARGET_PRODUCT=*)
      target_product=${i#*=}
      shift
      ;;
    TARGET_RELEASE=*)
      target_release=${i#*=}
      shift
      ;;
    TARGET_BUILD_VARIANT=*)
      target_build_variant=${i#*=}
      shift
      ;;
    *)
      echo "Unknown command line arguments: $i"
      exit 1
      ;;
  esac
done

echo "target product: $target_product, target_release: $target_release, target build variant: $target_build_variant"
case $target_product in
  aosp_cf_x86_64_phone)
    test_sbom_aosp_cf_x86_64_phone
    ;;
  module_arm64)
    test_sbom_unbundled_apex
    test_sbom_unbundled_apk
    ;;
  *)
    echo "Unknown TARGET_PRODUCT: $target_product"
    exit 1
    ;;
esac