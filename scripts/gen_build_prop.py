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
"""A tool for generating {partition}/build.prop"""

import argparse
import contextlib
import json
import os
import subprocess
import sys

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
  parser.add_argument("--build-fingerprint-file", required=True, type=argparse.FileType("r"))
  parser.add_argument("--build-hostname-file", required=True, type=argparse.FileType("r"))
  parser.add_argument("--build-number-file", required=True, type=argparse.FileType("r"))
  parser.add_argument("--build-thumbprint-file", type=argparse.FileType("r"))
  parser.add_argument("--build-username", required=True)
  parser.add_argument("--date-file", required=True, type=argparse.FileType("r"))
  parser.add_argument("--platform-preview-sdk-fingerprint-file", required=True, type=argparse.FileType("r"))
  parser.add_argument("--prop-files", action="append", type=argparse.FileType("r"), default=[])
  parser.add_argument("--product-config", required=True, type=argparse.FileType("r"))
  parser.add_argument("--partition", required=True)
  parser.add_argument("--build-broken-dup-sysprop", action="store_true", default=False)

  parser.add_argument("--out", required=True, type=argparse.FileType("w"))

  args = parser.parse_args()

  # post process parse_args requiring manual handling
  args.config = json.load(args.product_config)
  config = args.config

  config["BuildFlavor"] = get_build_flavor(config)
  config["BuildKeys"] = get_build_keys(config)
  config["BuildVariant"] = get_build_variant(config)

  config["BuildFingerprint"] = args.build_fingerprint_file.read().strip()
  config["BuildHostname"] = args.build_hostname_file.read().strip()
  config["BuildNumber"] = args.build_number_file.read().strip()
  config["BuildUsername"] = args.build_username

  build_version_tags_list = config["BuildVersionTags"]
  if config["BuildType"] == "debug":
    build_version_tags_list.append("debug")
  build_version_tags_list.append(config["BuildKeys"])
  build_version_tags = ",".join(sorted(set(build_version_tags_list)))
  config["BuildVersionTags"] = build_version_tags

  raw_date = args.date_file.read().strip()
  config["Date"] = subprocess.check_output(["date", "-d", f"@{raw_date}"], text=True).strip()
  config["DateUtc"] = subprocess.check_output(["date", "-d", f"@{raw_date}", "+%s"], text=True).strip()

  # build_desc is human readable strings that describe this build. This has the same info as the
  # build fingerprint.
  # e.g. "aosp_cf_x86_64_phone-userdebug VanillaIceCream MAIN eng.20240319.143939 test-keys"
  config["BuildDesc"] = f"{config['DeviceProduct']}-{config['BuildVariant']} " \
                        f"{config['Platform_version_name']} {config['BuildId']} " \
                        f"{config['BuildNumber']} {config['BuildVersionTags']}"

  config["PlatformPreviewSdkFingerprint"] = args.platform_preview_sdk_fingerprint_file.read().strip()

  if args.build_thumbprint_file:
    config["BuildThumbprint"] = args.build_thumbprint_file.read().strip()

  append_additional_system_props(args)
  append_additional_vendor_props(args)
  append_additional_product_props(args)

  return args

def generate_common_build_props(args):
  print("####################################")
  print("# from generate_common_build_props")
  print("# These properties identify this partition image.")
  print("####################################")

  config = args.config
  partition = args.partition

  if partition == "system":
    print(f"ro.product.{partition}.brand={config['SystemBrand']}")
    print(f"ro.product.{partition}.device={config['SystemDevice']}")
    print(f"ro.product.{partition}.manufacturer={config['SystemManufacturer']}")
    print(f"ro.product.{partition}.model={config['SystemModel']}")
    print(f"ro.product.{partition}.name={config['SystemName']}")
  else:
    print(f"ro.product.{partition}.brand={config['ProductBrand']}")
    print(f"ro.product.{partition}.device={config['DeviceName']}")
    print(f"ro.product.{partition}.manufacturer={config['ProductManufacturer']}")
    print(f"ro.product.{partition}.model={config['ProductModel']}")
    print(f"ro.product.{partition}.name={config['DeviceProduct']}")

  if partition != "system":
    if config["ModelForAttestation"]:
        print(f"ro.product.model_for_attestation={config['ModelForAttestation']}")
    if config["BrandForAttestation"]:
        print(f"ro.product.brand_for_attestation={config['BrandForAttestation']}")
    if config["NameForAttestation"]:
        print(f"ro.product.name_for_attestation={config['NameForAttestation']}")
    if config["DeviceForAttestation"]:
        print(f"ro.product.device_for_attestation={config['DeviceForAttestation']}")
    if config["ManufacturerForAttestation"]:
        print(f"ro.product.manufacturer_for_attestation={config['ManufacturerForAttestation']}")

  if config["ZygoteForce64"]:
    if partition == "vendor":
      print(f"ro.{partition}.product.cpu.abilist={config['DeviceAbiList64']}")
      print(f"ro.{partition}.product.cpu.abilist32=")
      print(f"ro.{partition}.product.cpu.abilist64={config['DeviceAbiList64']}")
  else:
    if partition == "system" or partition == "vendor" or partition == "odm":
      print(f"ro.{partition}.product.cpu.abilist={config['DeviceAbiList']}")
      print(f"ro.{partition}.product.cpu.abilist32={config['DeviceAbiList32']}")
      print(f"ro.{partition}.product.cpu.abilist64={config['DeviceAbiList64']}")

  print(f"ro.{partition}.build.date={config['Date']}")
  print(f"ro.{partition}.build.date.utc={config['DateUtc']}")
  # Allow optional assignments for ARC forward-declarations (b/249168657)
  # TODO: Remove any tag-related inconsistencies once the goals from
  # go/arc-android-sigprop-changes have been achieved.
  print(f"ro.{partition}.build.fingerprint?={config['BuildFingerprint']}")
  print(f"ro.{partition}.build.id?={config['BuildId']}")
  print(f"ro.{partition}.build.tags?={config['BuildVersionTags']}")
  print(f"ro.{partition}.build.type={config['BuildVariant']}")
  print(f"ro.{partition}.build.version.incremental={config['BuildNumber']}")
  print(f"ro.{partition}.build.version.release={config['Platform_version_last_stable']}")
  print(f"ro.{partition}.build.version.release_or_codename={config['Platform_version_name']}")
  print(f"ro.{partition}.build.version.sdk={config['Platform_sdk_version']}")

def generate_build_info(args):
  print()
  print("####################################")
  print("# from gen_build_prop.py:generate_build_info")
  print("####################################")
  print("# begin build properties")

  config = args.config
  build_flags = config["BuildFlags"]

  # The ro.build.id will be set dynamically by init, by appending the unique vbmeta digest.
  if config["BoardUseVbmetaDigestInFingerprint"]:
    print(f"ro.build.legacy.id={config['BuildId']}")
  else:
    print(f"ro.build.id?={config['BuildId']}")

  # ro.build.display.id is shown under Settings -> About Phone
  if config["BuildVariant"] == "user":
    # User builds should show:
    # release build number or branch.buld_number non-release builds

    # Dev. branches should have DISPLAY_BUILD_NUMBER set
    if config["DisplayBuildNumber"]:
      print(f"ro.build.display.id?={config['BuildId']}.{config['BuildNumber']} {config['BuildKeys']}")
    else:
      print(f"ro.build.display.id?={config['BuildId']} {config['BuildKeys']}")
  else:
    # Non-user builds should show detailed build information (See build desc above)
    print(f"ro.build.display.id?={config['BuildDesc']}")
  print(f"ro.build.version.incremental={config['BuildNumber']}")
  print(f"ro.build.version.sdk={config['Platform_sdk_version']}")
  print(f"ro.build.version.preview_sdk={config['Platform_preview_sdk_version']}")
  print(f"ro.build.version.preview_sdk_fingerprint={config['PlatformPreviewSdkFingerprint']}")
  print(f"ro.build.version.codename={config['Platform_sdk_codename']}")
  print(f"ro.build.version.all_codenames={','.join(config['Platform_version_active_codenames'])}")
  print(f"ro.build.version.known_codenames={config['Platform_version_known_codenames']}")
  print(f"ro.build.version.release={config['Platform_version_last_stable']}")
  print(f"ro.build.version.release_or_codename={config['Platform_version_name']}")
  print(f"ro.build.version.release_or_preview_display={config['Platform_display_version_name']}")
  print(f"ro.build.version.security_patch={config['Platform_security_patch']}")
  print(f"ro.build.version.base_os={config['Platform_base_os']}")
  print(f"ro.build.version.min_supported_target_sdk={build_flags['RELEASE_PLATFORM_MIN_SUPPORTED_TARGET_SDK_VERSION']}")
  print(f"ro.build.date={config['Date']}")
  print(f"ro.build.date.utc={config['DateUtc']}")
  print(f"ro.build.type={config['BuildVariant']}")
  print(f"ro.build.user={config['BuildUsername']}")
  print(f"ro.build.host={config['BuildHostname']}")
  # TODO: Remove any tag-related optional property declarations once the goals
  # from go/arc-android-sigprop-changes have been achieved.
  print(f"ro.build.tags?={config['BuildVersionTags']}")
  # ro.build.flavor are used only by the test harness to distinguish builds.
  # Only add _asan for a sanitized build if it isn't already a part of the
  # flavor (via a dedicated lunch config for example).
  print(f"ro.build.flavor={config['BuildFlavor']}")

  # These values are deprecated, use "ro.product.cpu.abilist"
  # instead (see below).
  print(f"# ro.product.cpu.abi and ro.product.cpu.abi2 are obsolete,")
  print(f"# use ro.product.cpu.abilist instead.")
  print(f"ro.product.cpu.abi={config['DeviceAbi'][0]}")
  if len(config["DeviceAbi"]) > 1:
    print(f"ro.product.cpu.abi2={config['DeviceAbi'][1]}")

  if config["ProductLocales"]:
    print(f"ro.product.locale={config['ProductLocales'][0]}")
  print(f"ro.wifi.channels={' '.join(config['ProductDefaultWifiChannels'])}")

  print(f"# ro.build.product is obsolete; use ro.product.device")
  print(f"ro.build.product={config['DeviceName']}")

  print(f"# Do not try to parse description or thumbprint")
  print(f"ro.build.description?={config['BuildDesc']}")
  if "build_thumbprint" in config:
    print(f"ro.build.thumbprint={config['BuildThumbprint']}")

  print(f"# end build properties")

def write_properties_from_file(file):
  print()
  print("####################################")
  print(f"# from {file.name}")
  print("####################################")
  print(file.read(), end="")

def write_properties_from_variable(name, props, build_broken_dup_sysprop):
  print()
  print("####################################")
  print(f"# from variable {name}")
  print("####################################")

  # Implement the legacy behavior when BUILD_BROKEN_DUP_SYSPROP is on.
  # Optional assignments are all converted to normal assignments and
  # when their duplicates the first one wins.
  if build_broken_dup_sysprop:
    processed_props = []
    seen_props = set()
    for line in props:
      line = line.replace("?=", "=")
      key, value = line.split("=", 1)
      if key in seen_props:
        continue
      seen_props.add(key)
      processed_props.append(line)
    props = processed_props

  for line in props:
    print(line)

def append_additional_system_props(args):
  props = []

  config = args.config

  # Add the product-defined properties to the build properties.
  if config["PropertySplitEnabled"] or config["VendorImageFileSystemType"]:
    if "PRODUCT_PROPERTY_OVERRIDES" in config:
      props += config["PRODUCT_PROPERTY_OVERRIDES"]

  props.append(f"ro.treble.enabled={'true' if config['FullTreble'] else 'false'}")
  # Set ro.llndk.api_level to show the maximum vendor API level that the LLNDK
  # in the system partition supports.
  if config["VendorApiLevel"]:
    props.append(f"ro.llndk.api_level={config['VendorApiLevel']}")

  # Sets ro.actionable_compatible_property.enabled to know on runtime whether
  # the allowed list of actionable compatible properties is enabled or not.
  props.append("ro.actionable_compatible_property.enabled=true")

  # Enable core platform API violation warnings on userdebug and eng builds.
  if config["BuildVariant"] != "user":
    props.append("persist.debug.dalvik.vm.core_platform_api_policy=just-warn")

  # Define ro.sanitize.<name> properties for all global sanitizers.
  for sanitize_target in config["SanitizeDevice"]:
    props.append(f"ro.sanitize.{sanitize_target}=true")

  # Sets the default value of ro.postinstall.fstab.prefix to /system.
  # Device board config should override the value to /product when needed by:
  #
  #     PRODUCT_PRODUCT_PROPERTIES += ro.postinstall.fstab.prefix=/product
  #
  # It then uses ${ro.postinstall.fstab.prefix}/etc/fstab.postinstall to
  # mount system_other partition.
  props.append("ro.postinstall.fstab.prefix=/system")

  enable_target_debugging = True
  if config["BuildVariant"] == "user" or config["BuildVariant"] == "userdebug":
    # Target is secure in user builds.
    props.append("ro.secure=1")
    props.append("security.perf_harden=1")

    if config["BuildVariant"] == "user":
      # Disable debugging in plain user builds.
      props.append("ro.adb.secure=1")
      enable_target_debugging = False

    # Disallow mock locations by default for user builds
    props.append("ro.allow.mock.location=0")
  else:
    # Turn on checkjni for non-user builds.
    props.append("ro.kernel.android.checkjni=1")
    # Set device insecure for non-user builds.
    props.append("ro.secure=0")
    # Allow mock locations by default for non user builds
    props.append("ro.allow.mock.location=1")

  if enable_target_debugging:
    # Enable Dalvik lock contention logging.
    props.append("dalvik.vm.lockprof.threshold=500")

    # Target is more debuggable and adbd is on by default
    props.append("ro.debuggable=1")
  else:
    # Target is less debuggable and adbd is off by default
    props.append("ro.debuggable=0")

  if config["BuildVariant"] == "eng":
    if "ro.setupwizard.mode=ENABLED" in props:
      # Don't require the setup wizard on eng builds
      props = list(filter(lambda x: not x.startswith("ro.setupwizard.mode="), props))
      props.append("ro.setupwizard.mode=OPTIONAL")

    if not config["SdkBuild"]:
      # To speedup startup of non-preopted builds, don't verify or compile the boot image.
      props.append("dalvik.vm.image-dex2oat-filter=extract")
    # b/323566535
    props.append("init.svc_debug.no_fatal.zygote=true")

  if config["SdkBuild"]:
    props.append("xmpp.auto-presence=true")
    props.append("ro.config.nocheckin=yes")

  props.append("net.bt.name=Android")

  # This property is set by flashing debug boot image, so default to false.
  props.append("ro.force.debuggable=0")

  config["ADDITIONAL_SYSTEM_PROPERTIES"] = props

def append_additional_vendor_props(args):
  props = []

  config = args.config
  build_flags = config["BuildFlags"]

  # Add cpu properties for bionic and ART.
  props.append(f"ro.bionic.arch={config['DeviceArch']}")
  props.append(f"ro.bionic.cpu_variant={config['DeviceCpuVariantRuntime']}")
  props.append(f"ro.bionic.2nd_arch={config['DeviceSecondaryArch']}")
  props.append(f"ro.bionic.2nd_cpu_variant={config['DeviceSecondaryCpuVariantRuntime']}")

  props.append(f"persist.sys.dalvik.vm.lib.2=libart.so")
  props.append(f"dalvik.vm.isa.{config['DeviceArch']}.variant={config['Dex2oatTargetCpuVariantRuntime']}")
  if config["Dex2oatTargetInstructionSetFeatures"]:
    props.append(f"dalvik.vm.isa.{config['DeviceArch']}.features={config['Dex2oatTargetInstructionSetFeatures']}")

  if config["DeviceSecondaryArch"]:
    props.append(f"dalvik.vm.isa.{config['DeviceSecondaryArch']}.variant={config['SecondaryDex2oatCpuVariantRuntime']}")
    if config["SecondaryDex2oatInstructionSetFeatures"]:
      props.append(f"dalvik.vm.isa.{config['DeviceSecondaryArch']}.features={config['SecondaryDex2oatInstructionSetFeatures']}")

  # Although these variables are prefixed with TARGET_RECOVERY_, they are also needed under charger
  # mode (via libminui).
  if config["RecoveryDefaultRotation"]:
    props.append(f"ro.minui.default_rotation={config['RecoveryDefaultRotation']}")

  if config["RecoveryOverscanPercent"]:
    props.append(f"ro.minui.overscan_percent={config['RecoveryOverscanPercent']}")

  if config["RecoveryPixelFormat"]:
    props.append(f"ro.minui.pixel_format={config['RecoveryPixelFormat']}")

  if "UseDynamicPartitions" in config:
    props.append(f"ro.boot.dynamic_partitions={'true' if config['UseDynamicPartitions'] else 'false'}")

  if "RetrofitDynamicPartitions" in config:
    props.append(f"ro.boot.dynamic_partitions_retrofit={'true' if config['RetrofitDynamicPartitions'] else 'false'}")

  if config["ShippingApiLevel"]:
    props.append(f"ro.product.first_api_level={config['ShippingApiLevel']}")

  if config["ShippingVendorApiLevel"]:
    props.append(f"ro.vendor.api_level={config['ShippingVendorApiLevel']}")

  if config["BuildVariant"] != "user" and config["BuildDebugfsRestrictionsEnabled"]:
    props.append(f"ro.product.debugfs_restrictions.enabled=true")

  # Vendors with GRF must define BOARD_SHIPPING_API_LEVEL for the vendor API level.
  # This must not be defined for the non-GRF devices.
  # The values of the GRF properties will be verified by post_process_props.py
  if config["BoardShippingApiLevel"]:
    props.append(f"ro.board.first_api_level={config['ProductShippingApiLevel']}")

  # Build system set BOARD_API_LEVEL to show the api level of the vendor API surface.
  # This must not be altered outside of build system.
  if config["VendorApiLevel"]:
    props.append(f"ro.board.api_level={config['VendorApiLevel']}")

  # RELEASE_BOARD_API_LEVEL_FROZEN is true when the vendor API surface is frozen.
  if build_flags["RELEASE_BOARD_API_LEVEL_FROZEN"]:
    props.append(f"ro.board.api_frozen=true")

  # Set build prop. This prop is read by ota_from_target_files when generating OTA,
  # to decide if VABC should be disabled.
  if config["DontUseVabcOta"]:
    props.append(f"ro.vendor.build.dont_use_vabc=true")

  # Set the flag in vendor. So VTS would know if the new fingerprint format is in use when
  # the system images are replaced by GSI.
  if config["BoardUseVbmetaDigestInFingerprint"]:
    props.append(f"ro.vendor.build.fingerprint_has_digest=1")

  props.append(f"ro.vendor.build.security_patch={config['VendorSecurityPatch']}")
  props.append(f"ro.product.board={config['BootloaderBoardName']}")
  props.append(f"ro.board.platform={config['BoardPlatform']}")
  props.append(f"ro.hwui.use_vulkan={'true' if config['UsesVulkan'] else 'false'}")

  if config["ScreenDensity"]:
    props.append(f"ro.sf.lcd_density={config['ScreenDensity']}")

  if "AbOtaUpdater" in config:
    props.append(f"ro.build.ab_update={'true' if config['AbOtaUpdater'] else 'false'}")
    if config["AbOtaUpdater"]:
      props.append(f"ro.vendor.build.ab_ota_partitions={config['AbOtaPartitions']}")

  config["ADDITIONAL_VENDOR_PROPERTIES"] = props

def append_additional_product_props(args):
  props = []

  config = args.config

  # Add the system server compiler filter if they are specified for the product.
  if config["SystemServerCompilerFilter"]:
    props.append(f"dalvik.vm.systemservercompilerfilter={config['SystemServerCompilerFilter']}")

  # Add the 16K developer args if it is defined for the product.
  props.append(f"ro.product.build.16k_page.enabled={'true' if config['Product16KDeveloperOption'] else 'false'}")

  props.append(f"ro.build.characteristics={config['AAPTCharacteristics']}")

  if "AbOtaUpdater" in config and config["AbOtaUpdater"]:
    props.append(f"ro.product.ab_ota_partitions={config['AbOtaPartitions']}")

  # Set this property for VTS to skip large page size tests on unsupported devices.
  props.append(f"ro.product.cpu.pagesize.max={config['DeviceMaxPageSizeSupported']}")

  if config["NoBionicPageSizeMacro"]:
    props.append(f"ro.product.build.no_bionic_page_size_macro=true")

  # If the value is "default", it will be mangled by post_process_props.py.
  props.append(f"ro.dalvik.vm.enable_uffd_gc={config['EnableUffdGc']}")

  config["ADDITIONAL_PRODUCT_PROPERTIES"] = props

def build_system_prop(args):
  config = args.config

  # Order matters here. When there are duplicates, the last one wins.
  # TODO(b/117892318): don't allow duplicates so that the ordering doesn't matter
  variables = [
    "ADDITIONAL_SYSTEM_PROPERTIES",
    "PRODUCT_SYSTEM_PROPERTIES",
    # TODO(b/117892318): deprecate this
    "PRODUCT_SYSTEM_DEFAULT_PROPERTIES",
  ]

  if not config["PropertySplitEnabled"]:
    variables += [
      "ADDITIONAL_VENDOR_PROPERTIES",
      "PRODUCT_VENDOR_PROPERTIES",
    ]

  build_prop(args, gen_build_info=True, gen_common_build_props=True, variables=variables)

'''
def build_vendor_prop(args):
  config = args.config

  # Order matters here. When there are duplicates, the last one wins.
  # TODO(b/117892318): don't allow duplicates so that the ordering doesn't matter
  variables = []
  if config["PropertySplitEnabled"]:
    variables += [
      "ADDITIONAL_VENDOR_PROPERTIES",
      "PRODUCT_VENDOR_PROPERTIES",
      # TODO(b/117892318): deprecate this
      "PRODUCT_DEFAULT_PROPERTY_OVERRIDES",
      "PRODUCT_PROPERTY_OVERRIDES",
    ]

  build_prop(args, gen_build_info=False, gen_common_build_props=True, variables=variables)

def build_product_prop(args):
  config = args.config

  # Order matters here. When there are duplicates, the last one wins.
  # TODO(b/117892318): don't allow duplicates so that the ordering doesn't matter
  variables = [
    "ADDITIONAL_PRODUCT_PROPERTIES",
    "PRODUCT_PRODUCT_PROPERTIES",
  ]
  build_prop(args, gen_build_info=False, gen_common_build_props=True, variables=variables)
'''

def build_prop(args, gen_build_info, gen_common_build_props, variables):
  config = args.config

  if gen_common_build_props:
    generate_common_build_props(args)

  if gen_build_info:
    generate_build_info(args)

  for prop_file in args.prop_files:
    write_properties_from_file(prop_file)

  for variable in variables:
    if variable in config:
      write_properties_from_variable(variable, config[variable], args.build_broken_dup_sysprop)

def main():
  args = parse_args()

  with contextlib.redirect_stdout(args.out):
    if args.partition == "system":
      build_system_prop(args)
      '''
    elif args.partition == "vendor":
      build_vendor_prop(args)
    elif args.partition == "product":
      build_product_prop(args)
      '''
    else:
      sys.exit(f"not supported partition {args.partition}")

if __name__ == "__main__":
  main()
