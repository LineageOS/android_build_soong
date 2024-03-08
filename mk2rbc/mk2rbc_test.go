// Copyright 2021 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mk2rbc

import (
	"bytes"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

var testCases = []struct {
	desc     string
	mkname   string
	in       string
	expected string
}{
	{
		desc:   "Comment",
		mkname: "product.mk",
		in: `
# Comment
# FOO= a\
     b
`,
		expected: `# Comment
# FOO= a
#     b
load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
`,
	},
	{
		desc:   "Name conversion",
		mkname: "path/bar-baz.mk",
		in: `
# Comment
`,
		expected: `# Comment
load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
`,
	},
	{
		desc:   "Item variable",
		mkname: "pixel3.mk",
		in: `
PRODUCT_NAME := Pixel 3
PRODUCT_MODEL :=
local_var = foo
local-var-with-dashes := bar
$(warning local-var-with-dashes: $(local-var-with-dashes))
GLOBAL-VAR-WITH-DASHES := baz
$(warning GLOBAL-VAR-WITH-DASHES: $(GLOBAL-VAR-WITH-DASHES))
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  cfg["PRODUCT_NAME"] = "Pixel 3"
  cfg["PRODUCT_MODEL"] = ""
  _local_var = "foo"
  _local_var_with_dashes = "bar"
  rblf.mkwarning("pixel3.mk", "local-var-with-dashes: %s" % _local_var_with_dashes)
  g["GLOBAL-VAR-WITH-DASHES"] = "baz"
  rblf.mkwarning("pixel3.mk", "GLOBAL-VAR-WITH-DASHES: %s" % g["GLOBAL-VAR-WITH-DASHES"])
`,
	},
	{
		desc:   "List variable",
		mkname: "pixel4.mk",
		in: `
PRODUCT_PACKAGES = package1  package2
PRODUCT_COPY_FILES += file2:target
PRODUCT_PACKAGES += package3
PRODUCT_COPY_FILES =
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  cfg["PRODUCT_PACKAGES"] = [
      "package1",
      "package2",
  ]
  rblf.setdefault(handle, "PRODUCT_COPY_FILES")
  cfg["PRODUCT_COPY_FILES"] += ["file2:target"]
  cfg["PRODUCT_PACKAGES"] += ["package3"]
  cfg["PRODUCT_COPY_FILES"] = []
`,
	},
	{
		desc:   "Unknown function",
		mkname: "product.mk",
		in: `
PRODUCT_NAME := $(call foo1, bar)
PRODUCT_NAME := $(call foo0)
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  cfg["PRODUCT_NAME"] = rblf.mk2rbc_error("product.mk:2", "cannot handle invoking foo1")
  cfg["PRODUCT_NAME"] = rblf.mk2rbc_error("product.mk:3", "cannot handle invoking foo0")
`,
	},
	{
		desc:   "Inherit configuration always",
		mkname: "product.mk",
		in: `
$(call inherit-product, part.mk)
ifdef PRODUCT_NAME
$(call inherit-product, part1.mk)
else # Comment
$(call inherit-product, $(LOCAL_PATH)/part.mk)
endif
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")
load(":part.star", _part_init = "init")
load(":part1.star|init", _part1_init = "init")

def init(g, handle):
  cfg = rblf.cfg(handle)
  rblf.inherit(handle, "part", _part_init)
  if cfg.get("PRODUCT_NAME", ""):
    if not _part1_init:
      rblf.mkerror("product.mk", "Cannot find %s" % (":part1.star"))
    rblf.inherit(handle, "part1", _part1_init)
  else:
    # Comment
    rblf.inherit(handle, "part", _part_init)
`,
	},
	{
		desc:   "Inherit configuration if it exists",
		mkname: "product.mk",
		in: `
$(call inherit-product-if-exists, part.mk)
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")
load(":part.star|init", _part_init = "init")

def init(g, handle):
  cfg = rblf.cfg(handle)
  if _part_init:
    rblf.inherit(handle, "part", _part_init)
`,
	},

	{
		desc:   "Include configuration",
		mkname: "product.mk",
		in: `
include part.mk
ifdef PRODUCT_NAME
include part1.mk
else
-include $(LOCAL_PATH)/part1.mk)
endif
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")
load(":part.star", _part_init = "init")
load(":part1.star|init", _part1_init = "init")

def init(g, handle):
  cfg = rblf.cfg(handle)
  _part_init(g, handle)
  if cfg.get("PRODUCT_NAME", ""):
    if not _part1_init:
      rblf.mkerror("product.mk", "Cannot find %s" % (":part1.star"))
    _part1_init(g, handle)
  else:
    if _part1_init != None:
      _part1_init(g, handle)
`,
	},

	{
		desc:   "Include with trailing whitespace",
		mkname: "product.mk",
		in: `
# has a trailing whitespace after cfg.mk
include vendor/$(foo)/cfg.mk 
`,
		expected: `# has a trailing whitespace after cfg.mk
load("//build/make/core:product_config.rbc", "rblf")
load("//vendor/foo1:cfg.star|init", _cfg_init = "init")
load("//vendor/bar/baz:cfg.star|init", _cfg1_init = "init")

def init(g, handle):
  cfg = rblf.cfg(handle)
  _entry = {
    "vendor/foo1/cfg.mk": ("vendor/foo1/cfg", _cfg_init),
    "vendor/bar/baz/cfg.mk": ("vendor/bar/baz/cfg", _cfg1_init),
  }.get("vendor/%s/cfg.mk" % _foo)
  (_varmod, _varmod_init) = _entry if _entry else (None, None)
  if not _varmod_init:
    rblf.mkerror("product.mk", "Cannot find %s" % ("vendor/%s/cfg.mk" % _foo))
  _varmod_init(g, handle)
`,
	},

	{
		desc:   "Synonymous inherited configurations",
		mkname: "path/product.mk",
		in: `
$(call inherit-product, */font.mk)
$(call inherit-product, $(sort $(wildcard */font.mk)))
$(call inherit-product, $(wildcard */font.mk))

include */font.mk
include $(sort $(wildcard */font.mk))
include $(wildcard */font.mk)
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")
load("//bar:font.star", _font_init = "init")
load("//foo:font.star", _font1_init = "init")

def init(g, handle):
  cfg = rblf.cfg(handle)
  rblf.inherit(handle, "bar/font", _font_init)
  rblf.inherit(handle, "foo/font", _font1_init)
  rblf.inherit(handle, "bar/font", _font_init)
  rblf.inherit(handle, "foo/font", _font1_init)
  rblf.inherit(handle, "bar/font", _font_init)
  rblf.inherit(handle, "foo/font", _font1_init)
  _font_init(g, handle)
  _font1_init(g, handle)
  _font_init(g, handle)
  _font1_init(g, handle)
  _font_init(g, handle)
  _font1_init(g, handle)
`,
	},
	{
		desc:   "Directive define",
		mkname: "product.mk",
		in: `
define some-macro
    $(info foo)
endef
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  rblf.mk2rbc_error("product.mk:2", "define is not supported: some-macro")
`,
	},
	{
		desc:   "Ifdef",
		mkname: "product.mk",
		in: `
ifdef  PRODUCT_NAME
  PRODUCT_NAME = gizmo
else
endif
local_var :=
ifdef local_var
endif
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  if cfg.get("PRODUCT_NAME", ""):
    cfg["PRODUCT_NAME"] = "gizmo"
  else:
    pass
  _local_var = ""
  if _local_var:
    pass
`,
	},
	{
		desc:   "Simple functions",
		mkname: "product.mk",
		in: `
$(warning this is the warning)
$(warning)
$(warning # this warning starts with a pound)
$(warning this warning has a # in the middle)
$(info this is the info)
$(error this is the error)
PRODUCT_NAME:=$(shell echo *)
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  rblf.mkwarning("product.mk", "this is the warning")
  rblf.mkwarning("product.mk", "")
  rblf.mkwarning("product.mk", "# this warning starts with a pound")
  rblf.mkwarning("product.mk", "this warning has a # in the middle")
  rblf.mkinfo("product.mk", "this is the info")
  rblf.mkerror("product.mk", "this is the error")
  cfg["PRODUCT_NAME"] = rblf.shell("echo *")
`,
	},
	{
		desc:   "Empty if",
		mkname: "product.mk",
		in: `
ifdef PRODUCT_NAME
# Comment
else
  TARGET_COPY_OUT_RECOVERY := foo
endif
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  if cfg.get("PRODUCT_NAME", ""):
    # Comment
    pass
  else:
    rblf.mk2rbc_error("product.mk:5", "cannot set predefined variable TARGET_COPY_OUT_RECOVERY to \"foo\", its value should be \"recovery\"")
`,
	},
	{
		desc:   "if/else/endif",
		mkname: "product.mk",
		in: `
ifndef PRODUCT_NAME
  PRODUCT_NAME=gizmo1
else
  PRODUCT_NAME=gizmo2
endif
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  if not cfg.get("PRODUCT_NAME", ""):
    cfg["PRODUCT_NAME"] = "gizmo1"
  else:
    cfg["PRODUCT_NAME"] = "gizmo2"
`,
	},
	{
		desc:   "else if",
		mkname: "product.mk",
		in: `
ifdef  PRODUCT_NAME
  PRODUCT_NAME = gizmo
else ifndef PRODUCT_PACKAGES   # Comment
endif
	`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  if cfg.get("PRODUCT_NAME", ""):
    cfg["PRODUCT_NAME"] = "gizmo"
  elif not cfg.get("PRODUCT_PACKAGES", []):
    # Comment
    pass
`,
	},
	{
		desc:   "ifeq / ifneq",
		mkname: "product.mk",
		in: `
ifeq (aosp_arm, $(TARGET_PRODUCT))
  PRODUCT_MODEL = pix2
else
  PRODUCT_MODEL = pix21
endif
ifneq (aosp_x86, $(TARGET_PRODUCT))
  PRODUCT_MODEL = pix3
endif
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  if "aosp_arm" == g["TARGET_PRODUCT"]:
    cfg["PRODUCT_MODEL"] = "pix2"
  else:
    cfg["PRODUCT_MODEL"] = "pix21"
  if "aosp_x86" != g["TARGET_PRODUCT"]:
    cfg["PRODUCT_MODEL"] = "pix3"
`,
	},
	{
		desc:   "ifeq with soong_config_get",
		mkname: "product.mk",
		in: `
ifeq (true,$(call soong_config_get,art_module,source_build))
endif
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  if "true" == rblf.soong_config_get(g, "art_module", "source_build"):
    pass
`,
	},
	{
		desc:   "ifeq with $(NATIVE_COVERAGE)",
		mkname: "product.mk",
		in: `
ifeq ($(NATIVE_COVERAGE),true)
endif
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  if g.get("NATIVE_COVERAGE", False):
    pass
`,
	},
	{
		desc:   "Check filter result",
		mkname: "product.mk",
		in: `
ifeq (,$(filter userdebug eng, $(TARGET_BUILD_VARIANT)))
endif
ifneq (,$(filter userdebug,$(TARGET_BUILD_VARIANT))
endif
ifneq (,$(filter plaf,$(PLATFORM_LIST)))
endif
ifeq ($(TARGET_BUILD_VARIANT), $(filter $(TARGET_BUILD_VARIANT), userdebug eng))
endif
ifneq (, $(filter $(TARGET_BUILD_VARIANT), userdebug eng))
endif
ifneq (,$(filter userdebug eng, $(TARGET_BUILD_VARIANT)))
endif
ifneq (,$(filter true, $(v1)$(v2)))
endif
ifeq (,$(filter barbet coral%,$(TARGET_PRODUCT)))
else ifneq (,$(filter barbet%,$(TARGET_PRODUCT)))
endif
ifeq (,$(filter-out sunfish_kasan, $(TARGET_PRODUCT)))
endif
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  if not rblf.filter("userdebug eng", g["TARGET_BUILD_VARIANT"]):
    pass
  if rblf.filter("userdebug", g["TARGET_BUILD_VARIANT"]):
    pass
  if "plaf" in g.get("PLATFORM_LIST", []):
    pass
  if g["TARGET_BUILD_VARIANT"] == " ".join(rblf.filter(g["TARGET_BUILD_VARIANT"], "userdebug eng")):
    pass
  if g["TARGET_BUILD_VARIANT"] in ["userdebug", "eng"]:
    pass
  if rblf.filter("userdebug eng", g["TARGET_BUILD_VARIANT"]):
    pass
  if rblf.filter("true", "%s%s" % (_v1, _v2)):
    pass
  if not rblf.filter("barbet coral%", g["TARGET_PRODUCT"]):
    pass
  elif rblf.filter("barbet%", g["TARGET_PRODUCT"]):
    pass
  if not rblf.filter_out("sunfish_kasan", g["TARGET_PRODUCT"]):
    pass
`,
	},
	{
		desc:   "Get filter result",
		mkname: "product.mk",
		in: `
PRODUCT_LIST2=$(filter-out %/foo.ko,$(wildcard path/*.ko))
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  cfg["PRODUCT_LIST2"] = rblf.filter_out("%/foo.ko", rblf.expand_wildcard("path/*.ko"))
`,
	},
	{
		desc:   "filter $(VAR), values",
		mkname: "product.mk",
		in: `
ifeq (,$(filter $(TARGET_PRODUCT), yukawa_gms yukawa_sei510_gms)
  ifneq (,$(filter $(TARGET_PRODUCT), yukawa_gms)
  endif
endif

`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  if g["TARGET_PRODUCT"] not in ["yukawa_gms", "yukawa_sei510_gms"]:
    if g["TARGET_PRODUCT"] == "yukawa_gms":
      pass
`,
	},
	{
		desc:   "filter $(V1), $(V2)",
		mkname: "product.mk",
		in: `
ifneq (, $(filter $(PRODUCT_LIST), $(TARGET_PRODUCT)))
endif
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  if rblf.filter(g.get("PRODUCT_LIST", []), g["TARGET_PRODUCT"]):
    pass
`,
	},
	{
		desc:   "ifeq",
		mkname: "product.mk",
		in: `
ifeq (aosp, $(TARGET_PRODUCT)) # Comment
else ifneq (, $(TARGET_PRODUCT))
endif
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  if "aosp" == g["TARGET_PRODUCT"]:
    # Comment
    pass
  elif g["TARGET_PRODUCT"]:
    pass
`,
	},
	{
		desc:   "Nested if",
		mkname: "product.mk",
		in: `
ifdef PRODUCT_NAME
  PRODUCT_PACKAGES = pack-if0
  ifdef PRODUCT_MODEL
    PRODUCT_PACKAGES = pack-if-if
  else ifdef PRODUCT_NAME
    PRODUCT_PACKAGES = pack-if-elif
  else
    PRODUCT_PACKAGES = pack-if-else
  endif
  PRODUCT_PACKAGES = pack-if
else ifneq (,$(TARGET_PRODUCT))
  PRODUCT_PACKAGES = pack-elif
else
  PRODUCT_PACKAGES = pack-else
endif
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  if cfg.get("PRODUCT_NAME", ""):
    cfg["PRODUCT_PACKAGES"] = ["pack-if0"]
    if cfg.get("PRODUCT_MODEL", ""):
      cfg["PRODUCT_PACKAGES"] = ["pack-if-if"]
    elif cfg.get("PRODUCT_NAME", ""):
      cfg["PRODUCT_PACKAGES"] = ["pack-if-elif"]
    else:
      cfg["PRODUCT_PACKAGES"] = ["pack-if-else"]
    cfg["PRODUCT_PACKAGES"] = ["pack-if"]
  elif g["TARGET_PRODUCT"]:
    cfg["PRODUCT_PACKAGES"] = ["pack-elif"]
  else:
    cfg["PRODUCT_PACKAGES"] = ["pack-else"]
`,
	},
	{
		desc:   "Wildcard",
		mkname: "product.mk",
		in: `
ifeq (,$(wildcard foo.mk))
endif
ifneq (,$(wildcard foo*.mk))
endif
ifeq (foo1.mk foo2.mk barxyz.mk,$(wildcard foo*.mk bar*.mk))
endif
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  if not rblf.expand_wildcard("foo.mk"):
    pass
  if rblf.expand_wildcard("foo*.mk"):
    pass
  if rblf.expand_wildcard("foo*.mk bar*.mk") == ["foo1.mk", "foo2.mk", "barxyz.mk"]:
    pass
`,
	},
	{
		desc:   "if with interpolation",
		mkname: "product.mk",
		in: `
ifeq ($(VARIABLE1)text$(VARIABLE2),true)
endif
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  if "%stext%s" % (g.get("VARIABLE1", ""), g.get("VARIABLE2", "")) == "true":
    pass
`,
	},
	{
		desc:   "ifneq $(X),true",
		mkname: "product.mk",
		in: `
ifneq ($(VARIABLE),true)
endif
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  if g.get("VARIABLE", "") != "true":
    pass
`,
	},
	{
		desc:   "Const neq",
		mkname: "product.mk",
		in: `
ifneq (1,0)
endif
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  if "1" != "0":
    pass
`,
	},
	{
		desc:   "is-board calls",
		mkname: "product.mk",
		in: `
ifeq ($(call is-board-platform-in-list,msm8998), true)
else ifneq ($(call is-board-platform,copper),true)
else ifneq ($(call is-vendor-board-platform,QCOM),true)
else ifeq ($(call is-product-in-list, $(PLATFORM_LIST)), true)
endif
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  if rblf.board_platform_in(g, "msm8998"):
    pass
  elif not rblf.board_platform_is(g, "copper"):
    pass
  elif g.get("TARGET_BOARD_PLATFORM", "") not in g.get("QCOM_BOARD_PLATFORMS", ""):
    pass
  elif g["TARGET_PRODUCT"] in g.get("PLATFORM_LIST", []):
    pass
`,
	},
	{
		desc:   "new is-board calls",
		mkname: "product.mk",
		in: `
ifneq (,$(call is-board-platform-in-list2,msm8998 $(X))
else ifeq (,$(call is-board-platform2,copper)
else ifneq (,$(call is-vendor-board-qcom))
endif
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  if rblf.board_platform_in(g, "msm8998 %s" % g.get("X", "")):
    pass
  elif not rblf.board_platform_is(g, "copper"):
    pass
  elif g.get("TARGET_BOARD_PLATFORM", "") in g.get("QCOM_BOARD_PLATFORMS", ""):
    pass
`,
	},
	{
		desc:   "findstring call",
		mkname: "product.mk",
		in: `
result := $(findstring a,a b c)
result := $(findstring b,x y z)
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  _result = rblf.findstring("a", "a b c")
  _result = rblf.findstring("b", "x y z")
`,
	},
	{
		desc:   "findstring in if statement",
		mkname: "product.mk",
		in: `
ifeq ($(findstring foo,$(PRODUCT_PACKAGES)),)
endif
ifneq ($(findstring foo,$(PRODUCT_PACKAGES)),)
endif
ifeq ($(findstring foo,$(PRODUCT_PACKAGES)),foo)
endif
ifneq ($(findstring foo,$(PRODUCT_PACKAGES)),foo)
endif
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  if (cfg.get("PRODUCT_PACKAGES", [])).find("foo") == -1:
    pass
  if (cfg.get("PRODUCT_PACKAGES", [])).find("foo") != -1:
    pass
  if (cfg.get("PRODUCT_PACKAGES", [])).find("foo") != -1:
    pass
  if (cfg.get("PRODUCT_PACKAGES", [])).find("foo") == -1:
    pass
`,
	},
	{
		desc:   "rhs call",
		mkname: "product.mk",
		in: `
PRODUCT_COPY_FILES = $(call add-to-product-copy-files-if-exists, path:distpath) \
 $(call find-copy-subdir-files, *, fromdir, todir) $(wildcard foo.*)
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  cfg["PRODUCT_COPY_FILES"] = (rblf.copy_if_exists("path:distpath") +
      rblf.find_and_copy("*", "fromdir", "todir") +
      rblf.expand_wildcard("foo.*"))
`,
	},
	{
		desc:   "inferred type",
		mkname: "product.mk",
		in: `
HIKEY_MODS := $(wildcard foo/*.ko)
BOARD_VENDOR_KERNEL_MODULES += $(HIKEY_MODS)
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  g["HIKEY_MODS"] = rblf.expand_wildcard("foo/*.ko")
  g.setdefault("BOARD_VENDOR_KERNEL_MODULES", [])
  g["BOARD_VENDOR_KERNEL_MODULES"] += g["HIKEY_MODS"]
`,
	},
	{
		desc:   "list with vars",
		mkname: "product.mk",
		in: `
PRODUCT_COPY_FILES += path1:$(TARGET_PRODUCT)/path1 $(PRODUCT_MODEL)/path2:$(TARGET_PRODUCT)/path2
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  rblf.setdefault(handle, "PRODUCT_COPY_FILES")
  cfg["PRODUCT_COPY_FILES"] += (("path1:%s/path1" % g["TARGET_PRODUCT"]).split() +
      ("%s/path2:%s/path2" % (cfg.get("PRODUCT_MODEL", ""), g["TARGET_PRODUCT"])).split())
`,
	},
	{
		desc:   "misc calls",
		mkname: "product.mk",
		in: `
$(call enforce-product-packages-exist,)
$(call enforce-product-packages-exist, foo)
$(call require-artifacts-in-path, foo, bar)
$(call require-artifacts-in-path-relaxed, foo, bar)
$(call dist-for-goals, goal, from:to)
$(call add-product-dex-preopt-module-config,MyModule,disable)
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  rblf.enforce_product_packages_exist(handle, "")
  rblf.enforce_product_packages_exist(handle, "foo")
  rblf.require_artifacts_in_path(handle, "foo", "bar")
  rblf.require_artifacts_in_path_relaxed(handle, "foo", "bar")
  rblf.mkdist_for_goals(g, "goal", "from:to")
  rblf.add_product_dex_preopt_module_config(handle, "MyModule", "disable")
`,
	},
	{
		desc:   "list with functions",
		mkname: "product.mk",
		in: `
PRODUCT_COPY_FILES := $(call find-copy-subdir-files,*.kl,from1,to1) \
 $(call find-copy-subdir-files,*.kc,from2,to2) \
 foo bar
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  cfg["PRODUCT_COPY_FILES"] = (rblf.find_and_copy("*.kl", "from1", "to1") +
      rblf.find_and_copy("*.kc", "from2", "to2") +
      [
          "foo",
          "bar",
      ])
`,
	},
	{
		desc:   "Text functions",
		mkname: "product.mk",
		in: `
PRODUCT_COPY_FILES := $(addprefix pfx-,a b c)
PRODUCT_COPY_FILES := $(addsuffix .sff, a b c)
PRODUCT_NAME := $(word 1, $(subst ., ,$(TARGET_BOARD_PLATFORM)))
ifeq (1,$(words $(SOME_UNKNOWN_VARIABLE)))
endif
ifeq ($(words $(SOME_OTHER_VARIABLE)),$(SOME_INT_VARIABLE))
endif
$(info $(patsubst %.pub,$(PRODUCT_NAME)%,$(PRODUCT_ADB_KEYS)))
$(info $$(dir foo/bar): $(dir foo/bar))
$(info $(firstword $(PRODUCT_COPY_FILES)))
$(info $(lastword $(PRODUCT_COPY_FILES)))
$(info $(dir $(lastword $(MAKEFILE_LIST))))
$(info $(dir $(lastword $(PRODUCT_COPY_FILES))))
$(info $(dir $(lastword $(foobar))))
$(info $(abspath foo/bar))
$(info $(notdir foo/bar))
$(call add_soong_config_namespace,snsconfig)
$(call add_soong_config_var_value,snsconfig,imagetype,odm_image)
$(call soong_config_set, snsconfig, foo, foo_value)
$(call soong_config_append, snsconfig, bar, bar_value)
PRODUCT_COPY_FILES := $(call copy-files,$(wildcard foo*.mk),etc)
PRODUCT_COPY_FILES := $(call product-copy-files-by-pattern,from/%,to/%,a b c)
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  cfg["PRODUCT_COPY_FILES"] = rblf.addprefix("pfx-", "a b c")
  cfg["PRODUCT_COPY_FILES"] = rblf.addsuffix(".sff", "a b c")
  cfg["PRODUCT_NAME"] = rblf.words((g.get("TARGET_BOARD_PLATFORM", "")).replace(".", " "))[0]
  if len(rblf.words(g.get("SOME_UNKNOWN_VARIABLE", ""))) == 1:
    pass
  if ("%d" % (len(rblf.words(g.get("SOME_OTHER_VARIABLE", ""))))) == g.get("SOME_INT_VARIABLE", ""):
    pass
  rblf.mkinfo("product.mk", rblf.mkpatsubst("%.pub", "%s%%" % cfg["PRODUCT_NAME"], g.get("PRODUCT_ADB_KEYS", "")))
  rblf.mkinfo("product.mk", "$(dir foo/bar): %s" % rblf.dir("foo/bar"))
  rblf.mkinfo("product.mk", rblf.first_word(cfg["PRODUCT_COPY_FILES"]))
  rblf.mkinfo("product.mk", rblf.last_word(cfg["PRODUCT_COPY_FILES"]))
  rblf.mkinfo("product.mk", rblf.dir(rblf.last_word("product.mk")))
  rblf.mkinfo("product.mk", rblf.dir(rblf.last_word(cfg["PRODUCT_COPY_FILES"])))
  rblf.mkinfo("product.mk", rblf.dir(rblf.last_word(_foobar)))
  rblf.mkinfo("product.mk", rblf.abspath("foo/bar"))
  rblf.mkinfo("product.mk", rblf.notdir("foo/bar"))
  rblf.soong_config_namespace(g, "snsconfig")
  rblf.soong_config_set(g, "snsconfig", "imagetype", "odm_image")
  rblf.soong_config_set(g, "snsconfig", "foo", "foo_value")
  rblf.soong_config_append(g, "snsconfig", "bar", "bar_value")
  cfg["PRODUCT_COPY_FILES"] = rblf.copy_files(rblf.expand_wildcard("foo*.mk"), "etc")
  cfg["PRODUCT_COPY_FILES"] = rblf.product_copy_files_by_pattern("from/%", "to/%", "a b c")
`,
	},
	{
		desc:   "subst in list",
		mkname: "product.mk",
		in: `
files = $(call find-copy-subdir-files,*,from,to)
PRODUCT_COPY_FILES += $(subst foo,bar,$(files))
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  _files = rblf.find_and_copy("*", "from", "to")
  rblf.setdefault(handle, "PRODUCT_COPY_FILES")
  cfg["PRODUCT_COPY_FILES"] += rblf.mksubst("foo", "bar", _files)
`,
	},
	{
		desc:   "assignment flavors",
		mkname: "product.mk",
		in: `
PRODUCT_LIST1 := a
PRODUCT_LIST2 += a
PRODUCT_LIST1 += b
PRODUCT_LIST2 += b
PRODUCT_LIST3 ?= a
PRODUCT_LIST1 = c
PLATFORM_LIST += x
PRODUCT_PACKAGES := $(PLATFORM_LIST)
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  cfg["PRODUCT_LIST1"] = ["a"]
  rblf.setdefault(handle, "PRODUCT_LIST2")
  cfg["PRODUCT_LIST2"] += ["a"]
  cfg["PRODUCT_LIST1"] += ["b"]
  cfg["PRODUCT_LIST2"] += ["b"]
  cfg["PRODUCT_LIST1"] = ["c"]
  g.setdefault("PLATFORM_LIST", [])
  g["PLATFORM_LIST"] += ["x"]
  cfg["PRODUCT_PACKAGES"] = g["PLATFORM_LIST"][:]
`,
	},
	{
		desc:   "assigment flavors2",
		mkname: "product.mk",
		in: `
PRODUCT_LIST1 = a
ifeq (0,1)
  PRODUCT_LIST1 += b
  PRODUCT_LIST2 += b
endif
PRODUCT_LIST1 += c
PRODUCT_LIST2 += c
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  cfg["PRODUCT_LIST1"] = ["a"]
  if "0" == "1":
    cfg["PRODUCT_LIST1"] += ["b"]
    rblf.setdefault(handle, "PRODUCT_LIST2")
    cfg["PRODUCT_LIST2"] += ["b"]
  cfg["PRODUCT_LIST1"] += ["c"]
  rblf.setdefault(handle, "PRODUCT_LIST2")
  cfg["PRODUCT_LIST2"] += ["c"]
`,
	},
	{
		desc:   "assigment setdefaults",
		mkname: "product.mk",
		in: `
# All of these should have a setdefault because they're self-referential and not defined before
PRODUCT_LIST1 = a $(PRODUCT_LIST1)
PRODUCT_LIST2 ?= a $(PRODUCT_LIST2)
PRODUCT_LIST3 += a

# Now doing them again should not have a setdefault because they've already been set, except 2
# which did not emit an assignment before
PRODUCT_LIST1 = a $(PRODUCT_LIST1)
PRODUCT_LIST2 = a $(PRODUCT_LIST2)
PRODUCT_LIST3 += a
`,
		expected: `# All of these should have a setdefault because they're self-referential and not defined before
load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  rblf.setdefault(handle, "PRODUCT_LIST1")
  cfg["PRODUCT_LIST1"] = (["a"] +
      cfg.get("PRODUCT_LIST1", []))
  rblf.setdefault(handle, "PRODUCT_LIST3")
  cfg["PRODUCT_LIST3"] += ["a"]
  # Now doing them again should not have a setdefault because they've already been set, except 2
  # which did not emit an assignment before
  cfg["PRODUCT_LIST1"] = (["a"] +
      cfg["PRODUCT_LIST1"])
  rblf.setdefault(handle, "PRODUCT_LIST2")
  cfg["PRODUCT_LIST2"] = (["a"] +
      cfg.get("PRODUCT_LIST2", []))
  cfg["PRODUCT_LIST3"] += ["a"]
`,
	},
	{
		desc:   "soong namespace assignments",
		mkname: "product.mk",
		in: `
SOONG_CONFIG_NAMESPACES += cvd
SOONG_CONFIG_cvd += launch_configs
SOONG_CONFIG_cvd_launch_configs = cvd_config_auto.json
SOONG_CONFIG_cvd += grub_config
SOONG_CONFIG_cvd_grub_config += grub.cfg
x := $(SOONG_CONFIG_cvd_grub_config)
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  rblf.soong_config_namespace(g, "cvd")
  rblf.soong_config_set(g, "cvd", "launch_configs", "cvd_config_auto.json")
  rblf.soong_config_append(g, "cvd", "grub_config", "grub.cfg")
  _x = rblf.mk2rbc_error("product.mk:7", "SOONG_CONFIG_ variables cannot be referenced, use soong_config_get instead: SOONG_CONFIG_cvd_grub_config")
`,
	}, {
		desc:   "soong namespace accesses",
		mkname: "product.mk",
		in: `
SOONG_CONFIG_NAMESPACES += cvd
SOONG_CONFIG_cvd += launch_configs
SOONG_CONFIG_cvd_launch_configs = cvd_config_auto.json
SOONG_CONFIG_cvd += grub_config
SOONG_CONFIG_cvd_grub_config += grub.cfg
x := $(call soong_config_get,cvd,grub_config)
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  rblf.soong_config_namespace(g, "cvd")
  rblf.soong_config_set(g, "cvd", "launch_configs", "cvd_config_auto.json")
  rblf.soong_config_append(g, "cvd", "grub_config", "grub.cfg")
  _x = rblf.soong_config_get(g, "cvd", "grub_config")
`,
	},
	{
		desc:   "string split",
		mkname: "product.mk",
		in: `
PRODUCT_LIST1 = a
local = b
local += c
FOO = d
FOO += e
PRODUCT_LIST1 += $(local)
PRODUCT_LIST1 += $(FOO)
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  cfg["PRODUCT_LIST1"] = ["a"]
  _local = "b"
  _local += " " + "c"
  g["FOO"] = "d"
  g["FOO"] += " " + "e"
  cfg["PRODUCT_LIST1"] += (_local).split()
  cfg["PRODUCT_LIST1"] += (g["FOO"]).split()
`,
	},
	{
		desc:   "apex_jars",
		mkname: "product.mk",
		in: `
PRODUCT_BOOT_JARS := $(ART_APEX_JARS) framework-minus-apex
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  cfg["PRODUCT_BOOT_JARS"] = (g.get("ART_APEX_JARS", []) +
      ["framework-minus-apex"])
`,
	},
	{
		desc:   "strip/sort functions",
		mkname: "product.mk",
		in: `
ifeq ($(filter hwaddress,$(PRODUCT_PACKAGES)),)
   PRODUCT_PACKAGES := $(strip $(PRODUCT_PACKAGES) hwaddress)
endif
MY_VAR := $(sort b a c)
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  if "hwaddress" not in cfg.get("PRODUCT_PACKAGES", []):
    rblf.setdefault(handle, "PRODUCT_PACKAGES")
    cfg["PRODUCT_PACKAGES"] = (rblf.mkstrip("%s hwaddress" % " ".join(cfg.get("PRODUCT_PACKAGES", [])))).split()
  g["MY_VAR"] = rblf.mksort("b a c")
`,
	},
	{
		desc:   "strip func in condition",
		mkname: "product.mk",
		in: `
ifneq ($(strip $(TARGET_VENDOR)),)
endif
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  if rblf.mkstrip(g.get("TARGET_VENDOR", "")):
    pass
`,
	},
	{
		desc:   "ref after set",
		mkname: "product.mk",
		in: `
PRODUCT_ADB_KEYS:=value
FOO := $(PRODUCT_ADB_KEYS)
ifneq (,$(PRODUCT_ADB_KEYS))
endif
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  g["PRODUCT_ADB_KEYS"] = "value"
  g["FOO"] = g["PRODUCT_ADB_KEYS"]
  if g["PRODUCT_ADB_KEYS"]:
    pass
`,
	},
	{
		desc:   "ref before set",
		mkname: "product.mk",
		in: `
V1 := $(PRODUCT_ADB_KEYS)
ifeq (,$(PRODUCT_ADB_KEYS))
  V2 := $(PRODUCT_ADB_KEYS)
  PRODUCT_ADB_KEYS:=foo
  V3 := $(PRODUCT_ADB_KEYS)
endif`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  g["V1"] = g.get("PRODUCT_ADB_KEYS", "")
  if not g.get("PRODUCT_ADB_KEYS", ""):
    g["V2"] = g.get("PRODUCT_ADB_KEYS", "")
    g["PRODUCT_ADB_KEYS"] = "foo"
    g["V3"] = g["PRODUCT_ADB_KEYS"]
`,
	},
	{
		desc:   "Dynamic inherit path",
		mkname: "product.mk",
		in: `
MY_PATH:=foo
$(call inherit-product,vendor/$(MY_PATH)/cfg.mk)
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")
load("//vendor/foo1:cfg.star|init", _cfg_init = "init")
load("//vendor/bar/baz:cfg.star|init", _cfg1_init = "init")

def init(g, handle):
  cfg = rblf.cfg(handle)
  g["MY_PATH"] = "foo"
  _entry = {
    "vendor/foo1/cfg.mk": ("vendor/foo1/cfg", _cfg_init),
    "vendor/bar/baz/cfg.mk": ("vendor/bar/baz/cfg", _cfg1_init),
  }.get("vendor/%s/cfg.mk" % g["MY_PATH"])
  (_varmod, _varmod_init) = _entry if _entry else (None, None)
  if not _varmod_init:
    rblf.mkerror("product.mk", "Cannot find %s" % ("vendor/%s/cfg.mk" % g["MY_PATH"]))
  rblf.inherit(handle, _varmod, _varmod_init)
`,
	},
	{
		desc:   "Dynamic inherit with hint",
		mkname: "product.mk",
		in: `
MY_PATH:=foo
#RBC# include_top vendor/foo1
$(call inherit-product,$(MY_PATH)/cfg.mk)
#RBC# include_top vendor/foo1
$(call inherit-product,$(MY_OTHER_PATH))
#RBC# include_top vendor/foo1
$(call inherit-product,vendor/$(MY_OTHER_PATH))
#RBC# include_top vendor/foo1
$(foreach f,$(MY_MAKEFILES), \
	$(call inherit-product,$(f)))
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")
load("//vendor/foo1:cfg.star|init", _cfg_init = "init")

def init(g, handle):
  cfg = rblf.cfg(handle)
  g["MY_PATH"] = "foo"
  _entry = {
    "vendor/foo1/cfg.mk": ("vendor/foo1/cfg", _cfg_init),
  }.get("%s/cfg.mk" % g["MY_PATH"])
  (_varmod, _varmod_init) = _entry if _entry else (None, None)
  if not _varmod_init:
    rblf.mkerror("product.mk", "Cannot find %s" % ("%s/cfg.mk" % g["MY_PATH"]))
  rblf.inherit(handle, _varmod, _varmod_init)
  _entry = {
    "vendor/foo1/cfg.mk": ("vendor/foo1/cfg", _cfg_init),
  }.get(g.get("MY_OTHER_PATH", ""))
  (_varmod, _varmod_init) = _entry if _entry else (None, None)
  if not _varmod_init:
    rblf.mkerror("product.mk", "Cannot find %s" % (g.get("MY_OTHER_PATH", "")))
  rblf.inherit(handle, _varmod, _varmod_init)
  _entry = {
    "vendor/foo1/cfg.mk": ("vendor/foo1/cfg", _cfg_init),
  }.get("vendor/%s" % g.get("MY_OTHER_PATH", ""))
  (_varmod, _varmod_init) = _entry if _entry else (None, None)
  if not _varmod_init:
    rblf.mkerror("product.mk", "Cannot find %s" % ("vendor/%s" % g.get("MY_OTHER_PATH", "")))
  rblf.inherit(handle, _varmod, _varmod_init)
  for f in rblf.words(g.get("MY_MAKEFILES", "")):
    _entry = {
      "vendor/foo1/cfg.mk": ("vendor/foo1/cfg", _cfg_init),
    }.get(f)
    (_varmod, _varmod_init) = _entry if _entry else (None, None)
    if not _varmod_init:
      rblf.mkerror("product.mk", "Cannot find %s" % (f))
    rblf.inherit(handle, _varmod, _varmod_init)
`,
	},
	{
		desc:   "Dynamic inherit with duplicated hint",
		mkname: "product.mk",
		in: `
MY_PATH:=foo
#RBC# include_top vendor/foo1
$(call inherit-product,$(MY_PATH)/cfg.mk)
#RBC# include_top vendor/foo1
#RBC# include_top vendor/foo1
$(call inherit-product,$(MY_PATH)/cfg.mk)
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")
load("//vendor/foo1:cfg.star|init", _cfg_init = "init")

def init(g, handle):
  cfg = rblf.cfg(handle)
  g["MY_PATH"] = "foo"
  _entry = {
    "vendor/foo1/cfg.mk": ("vendor/foo1/cfg", _cfg_init),
  }.get("%s/cfg.mk" % g["MY_PATH"])
  (_varmod, _varmod_init) = _entry if _entry else (None, None)
  if not _varmod_init:
    rblf.mkerror("product.mk", "Cannot find %s" % ("%s/cfg.mk" % g["MY_PATH"]))
  rblf.inherit(handle, _varmod, _varmod_init)
  _entry = {
    "vendor/foo1/cfg.mk": ("vendor/foo1/cfg", _cfg_init),
  }.get("%s/cfg.mk" % g["MY_PATH"])
  (_varmod, _varmod_init) = _entry if _entry else (None, None)
  if not _varmod_init:
    rblf.mkerror("product.mk", "Cannot find %s" % ("%s/cfg.mk" % g["MY_PATH"]))
  rblf.inherit(handle, _varmod, _varmod_init)
`,
	},
	{
		desc:   "Dynamic inherit path that lacks hint",
		mkname: "product.mk",
		in: `
#RBC# include_top foo
$(call inherit-product,$(MY_VAR)/font.mk)

#RBC# include_top foo

# There's some space and even this comment between the include_top and the inherit-product

$(call inherit-product,$(MY_VAR)/font.mk)

$(call inherit-product,$(MY_VAR)/font.mk)
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")
load("//foo:font.star|init", _font_init = "init")
load("//bar:font.star|init", _font1_init = "init")

def init(g, handle):
  cfg = rblf.cfg(handle)
  _entry = {
    "foo/font.mk": ("foo/font", _font_init),
  }.get("%s/font.mk" % g.get("MY_VAR", ""))
  (_varmod, _varmod_init) = _entry if _entry else (None, None)
  if not _varmod_init:
    rblf.mkerror("product.mk", "Cannot find %s" % ("%s/font.mk" % g.get("MY_VAR", "")))
  rblf.inherit(handle, _varmod, _varmod_init)
  # There's some space and even this comment between the include_top and the inherit-product
  _entry = {
    "foo/font.mk": ("foo/font", _font_init),
  }.get("%s/font.mk" % g.get("MY_VAR", ""))
  (_varmod, _varmod_init) = _entry if _entry else (None, None)
  if not _varmod_init:
    rblf.mkerror("product.mk", "Cannot find %s" % ("%s/font.mk" % g.get("MY_VAR", "")))
  rblf.inherit(handle, _varmod, _varmod_init)
  rblf.mkwarning("product.mk:11", "Please avoid starting an include path with a variable. See https://source.android.com/setup/build/bazel/product_config/issues/includes for details.")
  _entry = {
    "foo/font.mk": ("foo/font", _font_init),
    "bar/font.mk": ("bar/font", _font1_init),
  }.get("%s/font.mk" % g.get("MY_VAR", ""))
  (_varmod, _varmod_init) = _entry if _entry else (None, None)
  if not _varmod_init:
    rblf.mkerror("product.mk", "Cannot find %s" % ("%s/font.mk" % g.get("MY_VAR", "")))
  rblf.inherit(handle, _varmod, _varmod_init)
`,
	},
	{
		desc:   "Ignore make rules",
		mkname: "product.mk",
		in: `
foo: PRIVATE_VARIABLE = some_tool $< $@
foo: foo.c
	gcc -o $@ $*`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  rblf.mk2rbc_error("product.mk:2", "Only simple variables are handled")
  rblf.mk2rbc_error("product.mk:3", "unsupported line rule:       foo: foo.c\n#gcc -o $@ $*")
`,
	},
	{
		desc:   "Flag override",
		mkname: "product.mk",
		in: `
override FOO:=`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  rblf.mk2rbc_error("product.mk:2", "cannot handle override directive")
`,
	},
	{
		desc:   "Bad expression",
		mkname: "build/product.mk",
		in: `
ifeq (,$(call foobar))
endif
my_sources := $(local-generated-sources-dir)
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  if rblf.mk2rbc_error("build/product.mk:2", "cannot handle invoking foobar"):
    pass
  _my_sources = rblf.mk2rbc_error("build/product.mk:4", "local-generated-sources-dir is not supported")
`,
	},
	{
		desc:   "if expression",
		mkname: "product.mk",
		in: `
TEST_VAR := foo
TEST_VAR_LIST := foo
TEST_VAR_LIST += bar
TEST_VAR_2 := $(if $(TEST_VAR),bar)
TEST_VAR_3 := $(if $(TEST_VAR),bar,baz)
TEST_VAR_4 := $(if $(TEST_VAR),$(TEST_VAR_LIST))
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  g["TEST_VAR"] = "foo"
  g["TEST_VAR_LIST"] = ["foo"]
  g["TEST_VAR_LIST"] += ["bar"]
  g["TEST_VAR_2"] = ("bar" if g["TEST_VAR"] else "")
  g["TEST_VAR_3"] = ("bar" if g["TEST_VAR"] else "baz")
  g["TEST_VAR_4"] = (g["TEST_VAR_LIST"] if g["TEST_VAR"] else [])
`,
	},
	{
		desc:   "substitution references",
		mkname: "product.mk",
		in: `
SOURCES := foo.c bar.c
OBJECTS := $(SOURCES:.c=.o)
OBJECTS2 := $(SOURCES:%.c=%.o)
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  g["SOURCES"] = "foo.c bar.c"
  g["OBJECTS"] = rblf.mkpatsubst("%.c", "%.o", g["SOURCES"])
  g["OBJECTS2"] = rblf.mkpatsubst("%.c", "%.o", g["SOURCES"])
`,
	},
	{
		desc:   "foreach expressions",
		mkname: "product.mk",
		in: `
BOOT_KERNEL_MODULES := foo.ko bar.ko
BOOT_KERNEL_MODULES_FILTER := $(foreach m,$(BOOT_KERNEL_MODULES),%/$(m))
BOOT_KERNEL_MODULES_LIST := foo.ko
BOOT_KERNEL_MODULES_LIST += bar.ko
BOOT_KERNEL_MODULES_FILTER_2 := $(foreach m,$(BOOT_KERNEL_MODULES_LIST),%/$(m))
NESTED_LISTS := $(foreach m,$(SOME_VAR),$(BOOT_KERNEL_MODULES_LIST))
NESTED_LISTS_2 := $(foreach x,$(SOME_VAR),$(foreach y,$(x),prefix$(y)))

FOREACH_WITH_IF := $(foreach module,\
  $(BOOT_KERNEL_MODULES_LIST),\
  $(if $(filter $(module),foo.ko),,$(error module "$(module)" has an error!)))

# Same as above, but not assigning it to a variable allows it to be converted to statements
$(foreach module,\
  $(BOOT_KERNEL_MODULES_LIST),\
  $(if $(filter $(module),foo.ko),,$(error module "$(module)" has an error!)))
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  g["BOOT_KERNEL_MODULES"] = "foo.ko bar.ko"
  g["BOOT_KERNEL_MODULES_FILTER"] = ["%%/%s" % m for m in rblf.words(g["BOOT_KERNEL_MODULES"])]
  g["BOOT_KERNEL_MODULES_LIST"] = ["foo.ko"]
  g["BOOT_KERNEL_MODULES_LIST"] += ["bar.ko"]
  g["BOOT_KERNEL_MODULES_FILTER_2"] = ["%%/%s" % m for m in g["BOOT_KERNEL_MODULES_LIST"]]
  g["NESTED_LISTS"] = rblf.flatten_2d_list([g["BOOT_KERNEL_MODULES_LIST"] for m in rblf.words(g.get("SOME_VAR", ""))])
  g["NESTED_LISTS_2"] = rblf.flatten_2d_list([["prefix%s" % y for y in rblf.words(x)] for x in rblf.words(g.get("SOME_VAR", ""))])
  g["FOREACH_WITH_IF"] = [("" if rblf.filter(module, "foo.ko") else rblf.mkerror("product.mk", "module \"%s\" has an error!" % module)) for module in g["BOOT_KERNEL_MODULES_LIST"]]
  # Same as above, but not assigning it to a variable allows it to be converted to statements
  for module in g["BOOT_KERNEL_MODULES_LIST"]:
    if not rblf.filter(module, "foo.ko"):
      rblf.mkerror("product.mk", "module \"%s\" has an error!" % module)
`,
	},
	{
		desc:   "List appended to string",
		mkname: "product.mk",
		in: `
NATIVE_BRIDGE_PRODUCT_PACKAGES := \
    libnative_bridge_vdso.native_bridge \
    native_bridge_guest_app_process.native_bridge \
    native_bridge_guest_linker.native_bridge

NATIVE_BRIDGE_MODIFIED_GUEST_LIBS := \
    libaaudio \
    libamidi \
    libandroid \
    libandroid_runtime

NATIVE_BRIDGE_PRODUCT_PACKAGES += \
    $(addsuffix .native_bridge,$(NATIVE_BRIDGE_ORIG_GUEST_LIBS))
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  g["NATIVE_BRIDGE_PRODUCT_PACKAGES"] = "libnative_bridge_vdso.native_bridge      native_bridge_guest_app_process.native_bridge      native_bridge_guest_linker.native_bridge"
  g["NATIVE_BRIDGE_MODIFIED_GUEST_LIBS"] = "libaaudio      libamidi      libandroid      libandroid_runtime"
  g["NATIVE_BRIDGE_PRODUCT_PACKAGES"] += " " + " ".join(rblf.addsuffix(".native_bridge", g.get("NATIVE_BRIDGE_ORIG_GUEST_LIBS", "")))
`,
	},
	{
		desc:   "Math functions",
		mkname: "product.mk",
		in: `
# Test the math functions defined in build/make/common/math.mk
ifeq ($(call math_max,2,5),5)
endif
ifeq ($(call math_min,2,5),2)
endif
ifeq ($(call math_gt_or_eq,2,5),true)
endif
ifeq ($(call math_gt,2,5),true)
endif
ifeq ($(call math_lt,2,5),true)
endif
ifeq ($(call math_gt_or_eq,2,5),)
endif
ifeq ($(call math_gt,2,5),)
endif
ifeq ($(call math_lt,2,5),)
endif
ifeq ($(call math_gt_or_eq,$(MY_VAR), 5),true)
endif
ifeq ($(call math_gt_or_eq,$(MY_VAR),$(MY_OTHER_VAR)),true)
endif
ifeq ($(call math_gt_or_eq,100$(MY_VAR),10),true)
endif
`,
		expected: `# Test the math functions defined in build/make/common/math.mk
load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  if max(2, 5) == 5:
    pass
  if min(2, 5) == 2:
    pass
  if 2 >= 5:
    pass
  if 2 > 5:
    pass
  if 2 < 5:
    pass
  if 2 < 5:
    pass
  if 2 <= 5:
    pass
  if 2 >= 5:
    pass
  if int(g.get("MY_VAR", "")) >= 5:
    pass
  if int(g.get("MY_VAR", "")) >= int(g.get("MY_OTHER_VAR", "")):
    pass
  if int("100%s" % g.get("MY_VAR", "")) >= 10:
    pass
`,
	},
	{
		desc:   "Type hints",
		mkname: "product.mk",
		in: `
# Test type hints
#RBC# type_hint list MY_VAR MY_VAR_2
# Unsupported type
#RBC# type_hint bool MY_VAR_3
# Invalid syntax
#RBC# type_hint list
# Duplicated variable
#RBC# type_hint list MY_VAR_2
#RBC# type_hint list my-local-var-with-dashes
#RBC# type_hint string MY_STRING_VAR

MY_VAR := foo
MY_VAR_UNHINTED := foo

# Vars set after other statements still get the hint
MY_VAR_2 := foo

# You can't specify a type hint after the first statement
#RBC# type_hint list MY_VAR_4
MY_VAR_4 := foo

my-local-var-with-dashes := foo

MY_STRING_VAR := $(wildcard foo/bar.mk)
`,
		expected: `# Test type hints
# Unsupported type
load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  rblf.mk2rbc_error("product.mk:5", "Invalid type_hint annotation. Only list/string types are accepted, found bool")
  # Invalid syntax
  rblf.mk2rbc_error("product.mk:7", "Invalid type_hint annotation: list. Must be a variable type followed by a list of variables of that type")
  # Duplicated variable
  rblf.mk2rbc_error("product.mk:9", "Duplicate type hint for variable MY_VAR_2")
  g["MY_VAR"] = ["foo"]
  g["MY_VAR_UNHINTED"] = "foo"
  # Vars set after other statements still get the hint
  g["MY_VAR_2"] = ["foo"]
  # You can't specify a type hint after the first statement
  rblf.mk2rbc_error("product.mk:20", "type_hint annotations must come before the first Makefile statement")
  g["MY_VAR_4"] = "foo"
  _my_local_var_with_dashes = ["foo"]
  g["MY_STRING_VAR"] = " ".join(rblf.expand_wildcard("foo/bar.mk"))
`,
	},
	{
		desc:   "Set LOCAL_PATH to my-dir",
		mkname: "product.mk",
		in: `
LOCAL_PATH := $(call my-dir)
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  
`,
	},
	{
		desc:   "Evals",
		mkname: "product.mk",
		in: `
$(eval)
$(eval MY_VAR := foo)
$(eval # This is a test of eval functions)
$(eval $(TOO_COMPLICATED) := bar)
$(eval include foo/font.mk)
$(eval $(call inherit-product,vendor/foo1/cfg.mk))

$(foreach x,$(MY_LIST_VAR), \
  $(eval PRODUCT_COPY_FILES += foo/bar/$(x):$(TARGET_COPY_OUT_VENDOR)/etc/$(x)) \
  $(if $(MY_OTHER_VAR),$(eval PRODUCT_COPY_FILES += $(MY_OTHER_VAR):foo/bar/$(x))))

$(foreach x,$(MY_LIST_VAR), \
  $(eval include foo/$(x).mk))

# Check that we get as least close to correct line numbers for errors on statements inside evals
$(eval $(call inherit-product,$(A_VAR)))
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")
load("//foo:font.star", _font_init = "init")
load("//vendor/foo1:cfg.star", _cfg_init = "init")

def init(g, handle):
  cfg = rblf.cfg(handle)
  g["MY_VAR"] = "foo"
  # This is a test of eval functions
  rblf.mk2rbc_error("product.mk:5", "Eval expression too complex; only assignments, comments, includes, and inherit-products are supported")
  _font_init(g, handle)
  rblf.inherit(handle, "vendor/foo1/cfg", _cfg_init)
  for x in rblf.words(g.get("MY_LIST_VAR", "")):
    rblf.setdefault(handle, "PRODUCT_COPY_FILES")
    cfg["PRODUCT_COPY_FILES"] += ("foo/bar/%s:%s/etc/%s" % (x, g.get("TARGET_COPY_OUT_VENDOR", ""), x)).split()
    if g.get("MY_OTHER_VAR", ""):
      cfg["PRODUCT_COPY_FILES"] += ("%s:foo/bar/%s" % (g.get("MY_OTHER_VAR", ""), x)).split()
  for x in rblf.words(g.get("MY_LIST_VAR", "")):
    _entry = {
      "foo/font.mk": ("foo/font", _font_init),
    }.get("foo/%s.mk" % x)
    (_varmod, _varmod_init) = _entry if _entry else (None, None)
    if not _varmod_init:
      rblf.mkerror("product.mk", "Cannot find %s" % ("foo/%s.mk" % x))
    _varmod_init(g, handle)
  # Check that we get as least close to correct line numbers for errors on statements inside evals
  rblf.mk2rbc_error("product.mk:17", "inherit-product/include argument is too complex")
`,
	},
	{
		desc:   ".KATI_READONLY",
		mkname: "product.mk",
		in: `
MY_VAR := foo
.KATI_READONLY := MY_VAR
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  g["MY_VAR"] = "foo"
`,
	},
	{
		desc:   "Complicated variable references",
		mkname: "product.mk",
		in: `
MY_VAR := foo
MY_VAR_2 := MY_VAR
MY_VAR_3 := $($(MY_VAR_2))
MY_VAR_4 := $(foo bar)
MY_VAR_5 := $($(MY_VAR_2) bar)
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  g["MY_VAR"] = "foo"
  g["MY_VAR_2"] = "MY_VAR"
  g["MY_VAR_3"] = (cfg).get(g["MY_VAR_2"], (g).get(g["MY_VAR_2"], ""))
  g["MY_VAR_4"] = rblf.mk2rbc_error("product.mk:5", "cannot handle invoking foo")
  g["MY_VAR_5"] = rblf.mk2rbc_error("product.mk:6", "reference is too complex: $(MY_VAR_2) bar")
`,
	},
	{
		desc:   "Conditional functions",
		mkname: "product.mk",
		in: `
B := foo
X := $(or $(A))
X := $(or $(A),$(B))
X := $(or $(A),$(B),$(C))
X := $(and $(A))
X := $(and $(A),$(B))
X := $(and $(A),$(B),$(C))
X := $(or $(A),$(B)) Y

D := $(wildcard *.mk)
X := $(or $(B),$(D))
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  g["B"] = "foo"
  g["X"] = g.get("A", "")
  g["X"] = g.get("A", "") or g["B"]
  g["X"] = g.get("A", "") or g["B"] or g.get("C", "")
  g["X"] = g.get("A", "")
  g["X"] = g.get("A", "") and g["B"]
  g["X"] = g.get("A", "") and g["B"] and g.get("C", "")
  g["X"] = "%s Y" % g.get("A", "") or g["B"]
  g["D"] = rblf.expand_wildcard("*.mk")
  g["X"] = rblf.mk2rbc_error("product.mk:12", "Expected all arguments to $(or) or $(and) to have the same type, found \"string\" and \"list\"")
`,
	},
	{

		desc:   "is-lower/is-upper",
		mkname: "product.mk",
		in: `
X := $(call to-lower,aBc)
X := $(call to-upper,aBc)
X := $(call to-lower,$(VAR))
X := $(call to-upper,$(VAR))
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  g["X"] = ("aBc").lower()
  g["X"] = ("aBc").upper()
  g["X"] = (g.get("VAR", "")).lower()
  g["X"] = (g.get("VAR", "")).upper()
`,
	},
}

var known_variables = []struct {
	name  string
	class varClass
	starlarkType
}{
	{"NATIVE_COVERAGE", VarClassSoong, starlarkTypeBool},
	{"PRODUCT_NAME", VarClassConfig, starlarkTypeString},
	{"PRODUCT_MODEL", VarClassConfig, starlarkTypeString},
	{"PRODUCT_PACKAGES", VarClassConfig, starlarkTypeList},
	{"PRODUCT_BOOT_JARS", VarClassConfig, starlarkTypeList},
	{"PRODUCT_COPY_FILES", VarClassConfig, starlarkTypeList},
	{"PRODUCT_IS_64BIT", VarClassConfig, starlarkTypeString},
	{"PRODUCT_LIST1", VarClassConfig, starlarkTypeList},
	{"PRODUCT_LIST2", VarClassConfig, starlarkTypeList},
	{"PRODUCT_LIST3", VarClassConfig, starlarkTypeList},
	{"TARGET_PRODUCT", VarClassSoong, starlarkTypeString},
	{"TARGET_BUILD_VARIANT", VarClassSoong, starlarkTypeString},
	{"TARGET_BOARD_PLATFORM", VarClassSoong, starlarkTypeString},
	{"QCOM_BOARD_PLATFORMS", VarClassSoong, starlarkTypeString},
	{"PLATFORM_LIST", VarClassSoong, starlarkTypeList}, // TODO(asmundak): make it local instead of soong
}

type testMakefileFinder struct {
	fs    fs.FS
	root  string
	files []string
}

func (t *testMakefileFinder) Find(root string) []string {
	if t.files != nil || root == t.root {
		return t.files
	}
	t.files = make([]string, 0)
	fs.WalkDir(t.fs, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			base := filepath.Base(path)
			if base[0] == '.' && len(base) > 1 {
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".mk") {
			t.files = append(t.files, path)
		}
		return nil
	})
	return t.files
}

func TestGood(t *testing.T) {
	for _, v := range known_variables {
		KnownVariables.NewVariable(v.name, v.class, v.starlarkType)
	}
	fs := NewFindMockFS([]string{
		"vendor/foo1/cfg.mk",
		"vendor/bar/baz/cfg.mk",
		"part.mk",
		"foo/font.mk",
		"bar/font.mk",
	})
	for _, test := range testCases {
		t.Run(test.desc,
			func(t *testing.T) {
				ss, err := Convert(Request{
					MkFile:         test.mkname,
					Reader:         bytes.NewBufferString(test.in),
					OutputSuffix:   ".star",
					SourceFS:       fs,
					MakefileFinder: &testMakefileFinder{fs: fs},
				})
				if err != nil {
					t.Error(err)
					return
				}
				got := ss.String()
				if got != test.expected {
					t.Errorf("%q failed\nExpected:\n%s\nActual:\n%s\n", test.desc,
						strings.ReplaceAll(test.expected, "\n", "␤\n"),
						strings.ReplaceAll(got, "\n", "␤\n"))
				}
			})
	}
}
