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
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  cfg["PRODUCT_NAME"] = "Pixel 3"
  cfg["PRODUCT_MODEL"] = ""
  _local_var = "foo"
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
		expected: `# MK2RBC TRANSLATION ERROR: cannot handle invoking foo1
# PRODUCT_NAME := $(call foo1, bar)
# MK2RBC TRANSLATION ERROR: cannot handle invoking foo0
# PRODUCT_NAME := $(call foo0)
load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  rblf.warning("product.mk", "partially successful conversion")
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
$(call inherit-product, $(LOCAL_PATH)/part1.mk)
endif
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")
load(":part.star", _part_init = "init")
load(":part1.star|init", _part1_init = "init")

def init(g, handle):
  cfg = rblf.cfg(handle)
  rblf.inherit(handle, "part", _part_init)
  if g.get("PRODUCT_NAME") != None:
    rblf.inherit(handle, "part1", _part1_init)
  else:
    # Comment
    rblf.inherit(handle, "part1", _part1_init)
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
  if g.get("PRODUCT_NAME") != None:
    _part1_init(g, handle)
  else:
    if _part1_init != None:
      _part1_init(g, handle)
`,
	},

	{
		desc:   "Synonymous inherited configurations",
		mkname: "path/product.mk",
		in: `
$(call inherit-product, */font.mk)
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")
load("//foo:font.star", _font_init = "init")
load("//bar:font.star", _font1_init = "init")

def init(g, handle):
  cfg = rblf.cfg(handle)
  rblf.inherit(handle, "foo/font", _font_init)
  rblf.inherit(handle, "bar/font", _font1_init)
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
		expected: `# MK2RBC TRANSLATION ERROR: define is not supported: some-macro
# define  some-macro
#     $(info foo)
# endef
load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  rblf.warning("product.mk", "partially successful conversion")
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
  if g.get("PRODUCT_NAME") != None:
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
$(info this is the info)
$(error this is the error)
PRODUCT_NAME:=$(shell echo *)
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  rblf.mkwarning("product.mk", "this is the warning")
  rblf.mkwarning("product.mk", "")
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
  if g.get("PRODUCT_NAME") != None:
    # Comment
    pass
  else:
    # MK2RBC TRANSLATION ERROR: cannot set predefined variable TARGET_COPY_OUT_RECOVERY to "foo", its value should be "recovery"
    pass
  rblf.warning("product.mk", "partially successful conversion")
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
  if not g.get("PRODUCT_NAME") != None:
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
  if g.get("PRODUCT_NAME") != None:
    cfg["PRODUCT_NAME"] = "gizmo"
  elif not g.get("PRODUCT_PACKAGES") != None:
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
ifneq (,$(filter true, $(v1)$(v2)))
endif
ifeq (,$(filter barbet coral%,$(TARGET_PRODUCT)))
else ifneq (,$(filter barbet%,$(TARGET_PRODUCT)))
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
  if g["TARGET_BUILD_VARIANT"] in ["userdebug", "eng"]:
    pass
  if rblf.filter("true", "%s%s" % (_v1, _v2)):
    pass
  if not rblf.filter("barbet coral%", g["TARGET_PRODUCT"]):
    pass
  elif rblf.filter("barbet%", g["TARGET_PRODUCT"]):
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
  if g.get("PRODUCT_NAME") != None:
    cfg["PRODUCT_PACKAGES"] = ["pack-if0"]
    if g.get("PRODUCT_MODEL") != None:
      cfg["PRODUCT_PACKAGES"] = ["pack-if-if"]
    elif g.get("PRODUCT_NAME") != None:
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
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  if not rblf.file_exists("foo.mk"):
    pass
  if rblf.file_wildcard_exists("foo*.mk"):
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
  if g.get("TARGET_BOARD_PLATFORM", "") in ["msm8998"]:
    pass
  elif g.get("TARGET_BOARD_PLATFORM", "") != "copper":
    pass
  elif g.get("TARGET_BOARD_PLATFORM", "") not in g["QCOM_BOARD_PLATFORMS"]:
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
  elif g.get("TARGET_BOARD_PLATFORM", "") in g["QCOM_BOARD_PLATFORMS"]:
    pass
`,
	},
	{
		desc:   "findstring call",
		mkname: "product.mk",
		in: `
ifneq ($(findstring foo,$(PRODUCT_PACKAGES)),)
endif
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  if (cfg.get("PRODUCT_PACKAGES", [])).find("foo") != -1:
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
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  rblf.enforce_product_packages_exist("")
  rblf.enforce_product_packages_exist("foo")
  rblf.require_artifacts_in_path("foo", "bar")
  rblf.require_artifacts_in_path_relaxed("foo", "bar")
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
$(info $(patsubst %.pub,$(PRODUCT_NAME)%,$(PRODUCT_ADB_KEYS)))
$(info $(dir foo/bar))
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
  cfg["PRODUCT_NAME"] = ((g.get("TARGET_BOARD_PLATFORM", "")).replace(".", " ")).split()[0]
  rblf.mkinfo("product.mk", rblf.mkpatsubst("%.pub", "%s%%" % cfg["PRODUCT_NAME"], g.get("PRODUCT_ADB_KEYS", "")))
  rblf.mkinfo("product.mk", rblf.dir("foo/bar"))
  rblf.mkinfo("product.mk", cfg["PRODUCT_COPY_FILES"][0])
  rblf.mkinfo("product.mk", cfg["PRODUCT_COPY_FILES"][-1])
  rblf.mkinfo("product.mk", rblf.dir("product.mk"))
  rblf.mkinfo("product.mk", rblf.dir(cfg["PRODUCT_COPY_FILES"][-1]))
  rblf.mkinfo("product.mk", rblf.dir((_foobar).split()[-1]))
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
  if cfg.get("PRODUCT_LIST3") == None:
    cfg["PRODUCT_LIST3"] = ["a"]
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
  # MK2RBC TRANSLATION ERROR: SOONG_CONFIG_ variables cannot be referenced: SOONG_CONFIG_cvd_grub_config
  # x := $(SOONG_CONFIG_cvd_grub_config)
  rblf.warning("product.mk", "partially successful conversion")
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
		desc:   "strip function",
		mkname: "product.mk",
		in: `
ifeq ($(filter hwaddress,$(PRODUCT_PACKAGES)),)
   PRODUCT_PACKAGES := $(strip $(PRODUCT_PACKAGES) hwaddress)
endif
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  if "hwaddress" not in cfg.get("PRODUCT_PACKAGES", []):
    cfg["PRODUCT_PACKAGES"] = (rblf.mkstrip("%s hwaddress" % " ".join(cfg.get("PRODUCT_PACKAGES", [])))).split()
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
    "vendor/foo1/cfg.mk": ("_cfg", _cfg_init),
    "vendor/bar/baz/cfg.mk": ("_cfg1", _cfg1_init),
  }.get("vendor/%s/cfg.mk" % g["MY_PATH"])
  (_varmod, _varmod_init) = _entry if _entry else (None, None)
  if not _varmod_init:
    rblf.mkerror("cannot")
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
`,
		expected: `load("//build/make/core:product_config.rbc", "rblf")
load("//vendor/foo1:cfg.star|init", _cfg_init = "init")

def init(g, handle):
  cfg = rblf.cfg(handle)
  g["MY_PATH"] = "foo"
  #RBC# include_top vendor/foo1
  _entry = {
    "vendor/foo1/cfg.mk": ("_cfg", _cfg_init),
  }.get("%s/cfg.mk" % g["MY_PATH"])
  (_varmod, _varmod_init) = _entry if _entry else (None, None)
  if not _varmod_init:
    rblf.mkerror("cannot")
  rblf.inherit(handle, _varmod, _varmod_init)
`,
	},
	{
		desc:   "Ignore make rules",
		mkname: "product.mk",
		in: `
foo: foo.c
	gcc -o $@ $*`,
		expected: `# MK2RBC TRANSLATION ERROR: unsupported line rule:       foo: foo.c
#gcc -o $@ $*
# rule:       foo: foo.c
# gcc -o $@ $*
load("//build/make/core:product_config.rbc", "rblf")

def init(g, handle):
  cfg = rblf.cfg(handle)
  rblf.warning("product.mk", "partially successful conversion")
`,
	},
}

var known_variables = []struct {
	name  string
	class varClass
	starlarkType
}{
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
					MkFile:             test.mkname,
					Reader:             bytes.NewBufferString(test.in),
					RootDir:            ".",
					OutputSuffix:       ".star",
					WarnPartialSuccess: true,
					SourceFS:           fs,
					MakefileFinder:     &testMakefileFinder{fs: fs},
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
