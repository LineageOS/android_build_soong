#!/usr/bin/env python
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
"""Utils to determine whether to enable UFFD GC."""

import re
import sys


def should_enable_uffd_gc(kernel_version_file):
  with open(kernel_version_file, 'r') as f:
    kernel_version = f.read().strip()
  return should_enable_uffd_gc_impl(kernel_version)

def should_enable_uffd_gc_impl(kernel_version):
  # See https://source.android.com/docs/core/architecture/kernel/gki-versioning#determine-release
  p = r"^(?P<w>\d+)[.](?P<x>\d+)[.](?P<y>\d+)(-android(?P<z>\d+)-(?P<k>\d+).*$)?"
  m = re.match(p, kernel_version)
  if m is not None:
    if m.group('z') is not None:
      android_release = int(m.group('z'))
      # No need to check w, x, y because all Android 12 kernels have backports.
      return android_release >= 12
    else:
      # Old kernel or non-GKI kernel.
      version = int(m.group('w'))
      patch_level = int(m.group('x'))
      if version < 5:
        # Old kernel.
        return False
      elif (version == 5 and patch_level >= 7) or version >= 6:
        # New non-GKI kernel. 5.7 supports MREMAP_DONTUNMAP without the need for
        # backports.
        return True
      else:
        # Non-GKI kernel between 5 and 5.6. It may have backports.
        raise exit_with_error(kernel_version)
  elif kernel_version == '<unknown-kernel>':
    # The kernel information isn't available to the build system, probably
    # because PRODUCT_OTA_ENFORCE_VINTF_KERNEL_REQUIREMENTS is set to false. We
    # assume that the kernel supports UFFD GC because it is the case for most of
    # the products today and it is the future.
    return True
  else:
    # Unrecognizable non-GKI kernel.
    raise exit_with_error(kernel_version)

def exit_with_error(kernel_version):
  sys.exit(f"""
Unable to determine UFFD GC flag for kernel version "{kernel_version}".
You can fix this by explicitly setting PRODUCT_ENABLE_UFFD_GC to "true" or
"false" based on the kernel version.
1. Set PRODUCT_ENABLE_UFFD_GC to "true" if the kernel supports userfaultfd(2)
   and MREMAP_DONTUNMAP.
2. Set PRODUCT_ENABLE_UFFD_GC to "false" otherwise.""")
