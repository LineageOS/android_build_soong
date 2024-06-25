#!/usr/bin/env python3
#
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
#
"""A tool for generating buildinfo.prop"""

import argparse
import contextlib
import json
import os
import subprocess

TEST_KEY_DIR = "build/make/target/product/security"

def get_build_variant(product_config):
  if product_config["Eng"]:
    return "eng"
  elif product_config["Debuggable"]:
    return "userdebug"
  else:
    return "user"

def get_build_flavor(product_config):
  build_flavor = product_config["DeviceProduct"] + "-" + get_build_variant(product_config)
  if "address" in product_config.get("SanitizeDevice", []) and "_asan" not in build_flavor:
    build_flavor += "_asan"
  return build_flavor

def get_build_keys(product_config):
  default_cert = product_config.get("DefaultAppCertificate", "")
  if default_cert == "" or default_cert == os.path.join(TEST_KEY_DIR, "testKey"):
    return "test-keys"
  return "dev-keys"

def parse_args():
  """Parse commandline arguments."""
  parser = argparse.ArgumentParser()
  parser.add_argument('--build-hostname-file', required=True, type=argparse.FileType('r')),
  parser.add_argument('--build-number-file', required=True, type=argparse.FileType('r'))
  parser.add_argument('--build-thumbprint-file', type=argparse.FileType('r'))
  parser.add_argument('--build-username', required=True)
  parser.add_argument('--date-file', required=True, type=argparse.FileType('r'))
  parser.add_argument('--platform-preview-sdk-fingerprint-file',
                      required=True,
                      type=argparse.FileType('r'))
  parser.add_argument('--product-config', required=True, type=argparse.FileType('r'))
  parser.add_argument('--out', required=True, type=argparse.FileType('w'))

  option = parser.parse_args()

  product_config = json.load(option.product_config)
  build_flags = product_config["BuildFlags"]

  option.build_flavor = get_build_flavor(product_config)
  option.build_keys = get_build_keys(product_config)
  option.build_id = product_config["BuildId"]
  option.build_type = product_config["BuildType"]
  option.build_variant = get_build_variant(product_config)
  option.build_version_tags = product_config["BuildVersionTags"]
  option.cpu_abis = product_config["DeviceAbi"]
  option.default_locale = None
  if len(product_config.get("ProductLocales", [])) > 0:
    option.default_locale = product_config["ProductLocales"][0]
  option.default_wifi_channels = product_config.get("ProductDefaultWifiChannels", [])
  option.device = product_config["DeviceName"]
  option.display_build_number = product_config["DisplayBuildNumber"]
  option.platform_base_os = product_config["Platform_base_os"]
  option.platform_display_version = product_config["Platform_display_version_name"]
  option.platform_min_supported_target_sdk_version = build_flags["RELEASE_PLATFORM_MIN_SUPPORTED_TARGET_SDK_VERSION"]
  option.platform_preview_sdk_version = product_config["Platform_preview_sdk_version"]
  option.platform_sdk_version = product_config["Platform_sdk_version"]
  option.platform_security_patch = product_config["Platform_security_patch"]
  option.platform_version = product_config["Platform_version_name"]
  option.platform_version_codename = product_config["Platform_sdk_codename"]
  option.platform_version_all_codenames = product_config["Platform_version_active_codenames"]
  option.platform_version_known_codenames = product_config["Platform_version_known_codenames"]
  option.platform_version_last_stable = product_config["Platform_version_last_stable"]
  option.product = product_config["DeviceProduct"]
  option.use_vbmeta_digest_in_fingerprint = product_config["BoardUseVbmetaDigestInFingerprint"]

  return option

def main():
  option = parse_args()

  build_hostname = option.build_hostname_file.read().strip()
  build_number = option.build_number_file.read().strip()
  build_version_tags_list = option.build_version_tags
  if option.build_type == "debug":
    build_version_tags_list.append("debug")
  build_version_tags_list.append(option.build_keys)
  build_version_tags = ",".join(sorted(set(build_version_tags_list)))

  raw_date = option.date_file.read().strip()
  date = subprocess.check_output(["date", "-d", f"@{raw_date}"], text=True).strip()
  date_utc = subprocess.check_output(["date", "-d", f"@{raw_date}", "+%s"], text=True).strip()

  # build_desc is human readable strings that describe this build. This has the same info as the
  # build fingerprint.
  # e.g. "aosp_cf_x86_64_phone-userdebug VanillaIceCream MAIN eng.20240319.143939 test-keys"
  build_desc = f"{option.product}-{option.build_variant} {option.platform_version} " \
               f"{option.build_id} {build_number} {build_version_tags}"

  platform_preview_sdk_fingerprint = option.platform_preview_sdk_fingerprint_file.read().strip()

  with contextlib.redirect_stdout(option.out):
    print("# begin build properties")
    print("# autogenerated by buildinfo.py")

    # The ro.build.id will be set dynamically by init, by appending the unique vbmeta digest.
    if option.use_vbmeta_digest_in_fingerprint:
      print(f"ro.build.legacy.id={option.build_id}")
    else:
      print(f"ro.build.id?={option.build_id}")

    # ro.build.display.id is shown under Settings -> About Phone
    if option.build_variant == "user":
      # User builds should show:
      # release build number or branch.buld_number non-release builds

      # Dev. branches should have DISPLAY_BUILD_NUMBER set
      if option.display_build_number:
        print(f"ro.build.display.id?={option.build_id}.{build_number} {option.build_keys}")
      else:
        print(f"ro.build.display.id?={option.build_id} {option.build_keys}")
    else:
      # Non-user builds should show detailed build information (See build desc above)
      print(f"ro.build.display.id?={build_desc}")
    print(f"ro.build.version.incremental={build_number}")
    print(f"ro.build.version.sdk={option.platform_sdk_version}")
    print(f"ro.build.version.preview_sdk={option.platform_preview_sdk_version}")
    print(f"ro.build.version.preview_sdk_fingerprint={platform_preview_sdk_fingerprint}")
    print(f"ro.build.version.codename={option.platform_version_codename}")
    print(f"ro.build.version.all_codenames={','.join(option.platform_version_all_codenames)}")
    print(f"ro.build.version.known_codenames={option.platform_version_known_codenames}")
    print(f"ro.build.version.release={option.platform_version_last_stable}")
    print(f"ro.build.version.release_or_codename={option.platform_version}")
    print(f"ro.build.version.release_or_preview_display={option.platform_display_version}")
    print(f"ro.build.version.security_patch={option.platform_security_patch}")
    print(f"ro.build.version.base_os={option.platform_base_os}")
    print(f"ro.build.version.min_supported_target_sdk={option.platform_min_supported_target_sdk_version}")
    print(f"ro.build.date={date}")
    print(f"ro.build.date.utc={date_utc}")
    print(f"ro.build.type={option.build_variant}")
    print(f"ro.build.user={option.build_username}")
    print(f"ro.build.host={build_hostname}")
    # TODO: Remove any tag-related optional property declarations once the goals
    # from go/arc-android-sigprop-changes have been achieved.
    print(f"ro.build.tags?={build_version_tags}")
    # ro.build.flavor are used only by the test harness to distinguish builds.
    # Only add _asan for a sanitized build if it isn't already a part of the
    # flavor (via a dedicated lunch config for example).
    print(f"ro.build.flavor={option.build_flavor}")

    # These values are deprecated, use "ro.product.cpu.abilist"
    # instead (see below).
    print(f"# ro.product.cpu.abi and ro.product.cpu.abi2 are obsolete,")
    print(f"# use ro.product.cpu.abilist instead.")
    print(f"ro.product.cpu.abi={option.cpu_abis[0]}")
    if len(option.cpu_abis) > 1:
      print(f"ro.product.cpu.abi2={option.cpu_abis[1]}")

    if option.default_locale:
      print(f"ro.product.locale={option.default_locale}")
    print(f"ro.wifi.channels={' '.join(option.default_wifi_channels)}")

    print(f"# ro.build.product is obsolete; use ro.product.device")
    print(f"ro.build.product={option.device}")

    print(f"# Do not try to parse description or thumbprint")
    print(f"ro.build.description?={build_desc}")
    if option.build_thumbprint_file:
      build_thumbprint = option.build_thumbprint_file.read().strip()
      print(f"ro.build.thumbprint={build_thumbprint}")

    print(f"# end build properties")

if __name__ == "__main__":
  main()
