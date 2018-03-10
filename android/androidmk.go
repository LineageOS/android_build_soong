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

	var androidMkModulesList []Module

	ctx.VisitAllModules(func(module Module) {
		androidMkModulesList = append(androidMkModulesList, module)
	})

	sort.Sort(AndroidModulesByName{androidMkModulesList, ctx})

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

func translateAndroidMk(ctx SingletonContext, mkFile string, mods []Module) error {
	buf := &bytes.Buffer{}

	fmt.Fprintln(buf, "LOCAL_MODULE_MAKEFILE := $(lastword $(MAKEFILE_LIST))")

	type_stats := make(map[string]int)
	for _, mod := range mods {
		err := translateAndroidMkModule(ctx, buf, mod)
		if err != nil {
			os.Remove(mkFile)
			return err
		}

		if ctx.PrimaryModule(mod) == mod {
			type_stats[ctx.ModuleType(mod)] += 1
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
	provider, ok := mod.(AndroidMkDataProvider)
	if !ok {
		return nil
	}

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

	data.Required = amod.commonProperties.Required

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

		if amod.Arch().ArchType != ctx.Config().Targets[amod.Os().Class][0].Arch.ArchType {
			prefix = "2ND_" + prefix
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
		if amod.commonProperties.Owner != nil {
			fmt.Fprintln(&data.preamble, "LOCAL_MODULE_OWNER :=", *amod.commonProperties.Owner)
		}
		if amod.commonProperties.Notice != nil {
			fmt.Fprintln(&data.preamble, "LOCAL_NOTICE_FILE :=", "$(LOCAL_PATH)/"+*amod.commonProperties.Notice)
		}
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
