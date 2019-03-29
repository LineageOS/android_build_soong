// Copyright 2015 Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package android

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/bootstrap"
)

func init() {
	RegisterSingletonType("androidmk", AndroidMkSingleton)
}

type AndroidMkDataProvider interface {
	AndroidMk() AndroidMkData
	BaseModuleName() string
}

type AndroidMkData struct {
	Class      string
	SubName    string
	DistFile   OptionalPath
	OutputFile OptionalPath
	Disabled   bool
	Include    string
	Required   []string

	Custom func(w io.Writer, name, prefix, moduleDir string, data AndroidMkData)

	Extra []AndroidMkExtraFunc

	preamble bytes.Buffer
}

type AndroidMkExtraFunc func(w io.Writer, outputFile Path)

func AndroidMkSingleton() Singleton {
	return &androidMkSingleton{}
}

type androidMkSingleton struct{}

func (c *androidMkSingleton) GenerateBuildActions(ctx SingletonContext) {
	if !ctx.Config().EmbeddedInMake() {
		return
	}

	var androidMkModulesList []blueprint.Module

	ctx.VisitAllModulesBlueprint(func(module blueprint.Module) {
		androidMkModulesList = append(androidMkModulesList, module)
	})

	sort.SliceStable(androidMkModulesList, func(i, j int) bool {
		return ctx.ModuleName(androidMkModulesList[i]) < ctx.ModuleName(androidMkModulesList[j])
	})

	transMk := PathForOutput(ctx, "Android"+String(ctx.Config().productVariables.Make_suffix)+".mk")
	if ctx.Failed() {
		return
	}

	err := translateAndroidMk(ctx, transMk.String(), androidMkModulesList)
	if err != nil {
		ctx.Errorf(err.Error())
	}

	ctx.Build(pctx, BuildParams{
		Rule:   blueprint.Phony,
		Output: transMk,
	})
}

func translateAndroidMk(ctx SingletonContext, mkFile string, mods []blueprint.Module) error {
	buf := &bytes.Buffer{}

	fmt.Fprintln(buf, "LOCAL_MODULE_MAKEFILE := $(lastword $(MAKEFILE_LIST))")

	type_stats := make(map[string]int)
	for _, mod := range mods {
		err := translateAndroidMkModule(ctx, buf, mod)
		if err != nil {
			os.Remove(mkFile)
			return err
		}

		if amod, ok := mod.(Module); ok && ctx.PrimaryModule(amod) == amod {
			type_stats[ctx.ModuleType(amod)] += 1
		}
	}

	keys := []string{}
	fmt.Fprintln(buf, "\nSTATS.SOONG_MODULE_TYPE :=")
	for k := range type_stats {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, mod_type := range keys {
		fmt.Fprintln(buf, "STATS.SOONG_MODULE_TYPE +=", mod_type)
		fmt.Fprintf(buf, "STATS.SOONG_MODULE_TYPE.%s := %d\n", mod_type, type_stats[mod_type])
	}

	// Don't write to the file if it hasn't changed
	if _, err := os.Stat(mkFile); !os.IsNotExist(err) {
		if data, err := ioutil.ReadFile(mkFile); err == nil {
			matches := buf.Len() == len(data)

			if matches {
				for i, value := range buf.Bytes() {
					if value != data[i] {
						matches = false
						break
					}
				}
			}

			if matches {
				return nil
			}
		}
	}

	return ioutil.WriteFile(mkFile, buf.Bytes(), 0666)
}

func translateAndroidMkModule(ctx SingletonContext, w io.Writer, mod blueprint.Module) error {
	defer func() {
		if r := recover(); r != nil {
			panic(fmt.Errorf("%s in translateAndroidMkModule for module %s variant %s",
				r, ctx.ModuleName(mod), ctx.ModuleSubDir(mod)))
		}
	}()

	switch x := mod.(type) {
	case AndroidMkDataProvider:
		return translateAndroidModule(ctx, w, mod, x)
	case bootstrap.GoBinaryTool:
		return translateGoBinaryModule(ctx, w, mod, x)
	default:
		return nil
	}
}

func translateGoBinaryModule(ctx SingletonContext, w io.Writer, mod blueprint.Module,
	goBinary bootstrap.GoBinaryTool) error {

	name := ctx.ModuleName(mod)
	fmt.Fprintln(w, ".PHONY:", name)
	fmt.Fprintln(w, name+":", goBinary.InstallPath())
	fmt.Fprintln(w, "")

	return nil
}

func translateAndroidModule(ctx SingletonContext, w io.Writer, mod blueprint.Module,
	provider AndroidMkDataProvider) error {

	name := provider.BaseModuleName()
	amod := mod.(Module).base()

	if !amod.Enabled() {
		return nil
	}

	if amod.commonProperties.SkipInstall {
		return nil
	}

	if !amod.commonProperties.NamespaceExportedToMake {
		// TODO(jeffrygaston) do we want to validate that there are no modules being
		// exported to Kati that depend on this module?
		return nil
	}

	data := provider.AndroidMk()

	if data.Include == "" {
		data.Include = "$(BUILD_PREBUILT)"
	}

	data.Required = append(data.Required, amod.commonProperties.Required...)

	// Make does not understand LinuxBionic
	if amod.Os() == LinuxBionic {
		return nil
	}

	prefix := ""
	if amod.ArchSpecific() {
		switch amod.Os().Class {
		case Host:
			prefix = "HOST_"
		case HostCross:
			prefix = "HOST_CROSS_"
		case Device:
			prefix = "TARGET_"

		}

		if amod.Arch().ArchType != ctx.Config().Targets[amod.Os()][0].Arch.ArchType {
			prefix = "2ND_" + prefix
		}
	}

	if len(amod.commonProperties.Dist.Targets) > 0 {
		distFile := data.DistFile
		if !distFile.Valid() {
			distFile = data.OutputFile
		}
		if distFile.Valid() {
			dest := filepath.Base(distFile.String())

			if amod.commonProperties.Dist.Dest != nil {
				var err error
				dest, err = validateSafePath(*amod.commonProperties.Dist.Dest)
				if err != nil {
					// This was checked in ModuleBase.GenerateBuildActions
					panic(err)
				}
			}

			if amod.commonProperties.Dist.Suffix != nil {
				ext := filepath.Ext(dest)
				suffix := *amod.commonProperties.Dist.Suffix
				dest = strings.TrimSuffix(dest, ext) + suffix + ext
			}

			if amod.commonProperties.Dist.Dir != nil {
				var err error
				dest, err = validateSafePath(*amod.commonProperties.Dist.Dir, dest)
				if err != nil {
					// This was checked in ModuleBase.GenerateBuildActions
					panic(err)
				}
			}

			goals := strings.Join(amod.commonProperties.Dist.Targets, " ")
			fmt.Fprintln(&data.preamble, ".PHONY:", goals)
			fmt.Fprintf(&data.preamble, "$(call dist-for-goals,%s,%s:%s)\n",
				goals, distFile.String(), dest)
		}
	}

	fmt.Fprintln(&data.preamble, "\ninclude $(CLEAR_VARS)")
	fmt.Fprintln(&data.preamble, "LOCAL_PATH :=", filepath.Dir(ctx.BlueprintFile(mod)))
	fmt.Fprintln(&data.preamble, "LOCAL_MODULE :=", name+data.SubName)
	fmt.Fprintln(&data.preamble, "LOCAL_MODULE_CLASS :=", data.Class)
	fmt.Fprintln(&data.preamble, "LOCAL_PREBUILT_MODULE_FILE :=", data.OutputFile.String())

	if len(data.Required) > 0 {
		fmt.Fprintln(&data.preamble, "LOCAL_REQUIRED_MODULES := "+strings.Join(data.Required, " "))
	}

	archStr := amod.Arch().ArchType.String()
	host := false
	switch amod.Os().Class {
	case Host:
		// Make cannot identify LOCAL_MODULE_HOST_ARCH:= common.
		if archStr != "common" {
			fmt.Fprintln(&data.preamble, "LOCAL_MODULE_HOST_ARCH :=", archStr)
		}
		host = true
	case HostCross:
		// Make cannot identify LOCAL_MODULE_HOST_CROSS_ARCH:= common.
		if archStr != "common" {
			fmt.Fprintln(&data.preamble, "LOCAL_MODULE_HOST_CROSS_ARCH :=", archStr)
		}
		host = true
	case Device:
		// Make cannot identify LOCAL_MODULE_TARGET_ARCH:= common.
		if archStr != "common" {
			fmt.Fprintln(&data.preamble, "LOCAL_MODULE_TARGET_ARCH :=", archStr)
		}

		if len(amod.commonProperties.Init_rc) > 0 {
			fmt.Fprintln(&data.preamble, "LOCAL_INIT_RC := ", strings.Join(amod.commonProperties.Init_rc, " "))
		}
		if len(amod.commonProperties.Vintf_fragments) > 0 {
			fmt.Fprintln(&data.preamble, "LOCAL_VINTF_FRAGMENTS := ", strings.Join(amod.commonProperties.Vintf_fragments, " "))
		}
		if Bool(amod.commonProperties.Proprietary) {
			fmt.Fprintln(&data.preamble, "LOCAL_PROPRIETARY_MODULE := true")
		}
		if Bool(amod.commonProperties.Vendor) || Bool(amod.commonProperties.Soc_specific) {
			fmt.Fprintln(&data.preamble, "LOCAL_VENDOR_MODULE := true")
		}
		if Bool(amod.commonProperties.Device_specific) {
			fmt.Fprintln(&data.preamble, "LOCAL_ODM_MODULE := true")
		}
		if Bool(amod.commonProperties.Product_specific) {
			fmt.Fprintln(&data.preamble, "LOCAL_PRODUCT_MODULE := true")
		}
		if Bool(amod.commonProperties.Product_services_specific) {
			fmt.Fprintln(&data.preamble, "LOCAL_PRODUCT_SERVICES_MODULE := true")
		}
		if amod.commonProperties.Owner != nil {
			fmt.Fprintln(&data.preamble, "LOCAL_MODULE_OWNER :=", *amod.commonProperties.Owner)
		}
	}

	if amod.noticeFile.Valid() {
		fmt.Fprintln(&data.preamble, "LOCAL_NOTICE_FILE :=", amod.noticeFile.String())
	}

	if host {
		makeOs := amod.Os().String()
		if amod.Os() == Linux || amod.Os() == LinuxBionic {
			makeOs = "linux"
		}
		fmt.Fprintln(&data.preamble, "LOCAL_MODULE_HOST_OS :=", makeOs)
		fmt.Fprintln(&data.preamble, "LOCAL_IS_HOST_MODULE := true")
	}

	blueprintDir := filepath.Dir(ctx.BlueprintFile(mod))

	if data.Custom != nil {
		data.Custom(w, name, prefix, blueprintDir, data)
	} else {
		WriteAndroidMkData(w, data)
	}

	return nil
}

func WriteAndroidMkData(w io.Writer, data AndroidMkData) {
	if data.Disabled {
		return
	}

	if !data.OutputFile.Valid() {
		return
	}

	w.Write(data.preamble.Bytes())

	for _, extra := range data.Extra {
		extra(w, data.OutputFile.Path())
	}

	fmt.Fprintln(w, "include "+data.Include)
}
