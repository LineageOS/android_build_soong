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

package common

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"

	"android/soong"

	"github.com/google/blueprint"
)

func init() {
	soong.RegisterSingletonType("androidmk", AndroidMkSingleton)
}

type AndroidMkDataProvider interface {
	AndroidMk() AndroidMkData
}

type AndroidMkData struct {
	Class      string
	OutputFile OptionalPath

	Custom func(w io.Writer, name, prefix string)

	Extra func(name, prefix string, outputFile Path, arch Arch) []string
}

func AndroidMkSingleton() blueprint.Singleton {
	return &androidMkSingleton{}
}

type androidMkSingleton struct{}

func (c *androidMkSingleton) GenerateBuildActions(ctx blueprint.SingletonContext) {
	dirModules := make(map[string][]blueprint.Module)
	hasBPDir := make(map[string]bool)
	bpDirs := []string{}

	if !ctx.Config().(Config).EmbeddedInMake() {
		return
	}

	ctx.SetNinjaBuildDir(pctx, filepath.Join(ctx.Config().(Config).buildDir, ".."))

	ctx.VisitAllModules(func(module blueprint.Module) {
		if _, ok := module.(AndroidModule); ok {
			bpDir := filepath.Dir(ctx.BlueprintFile(module))

			if !hasBPDir[bpDir] {
				hasBPDir[bpDir] = true
				bpDirs = append(bpDirs, bpDir)
			}

			dirModules[bpDir] = append(dirModules[bpDir], module)
		}
	})

	// Gather list of eligible Android modules for translation
	androidMkModules := make(map[blueprint.Module]bool)
	sort.Strings(bpDirs)
	for _, bpDir := range bpDirs {
		mkFile := OptionalPathForSource(ctx, "androidmk", bpDir, "Android.mk")
		if !mkFile.Valid() {
			for _, mod := range dirModules[bpDir] {
				androidMkModules[mod] = true
			}
		}
	}

	// Validate that all modules have proper dependencies
	androidMkModulesList := make([]AndroidModule, 0, len(androidMkModules))
	for mod := range androidMkModules {
		ctx.VisitDepsDepthFirstIf(mod, isAndroidModule, func(module blueprint.Module) {
			if !androidMkModules[module] {
				ctx.Errorf("Module %q missing dependency for Android.mk: %q", ctx.ModuleName(mod), ctx.ModuleName(module))
			}
		})
		if amod, ok := mod.(AndroidModule); ok {
			androidMkModulesList = append(androidMkModulesList, amod)
		}
	}

	transMk := PathForOutput(ctx, "Android.mk")
	if ctx.Failed() {
		return
	}

	err := translateAndroidMk(ctx, transMk.String(), androidMkModulesList)
	if err != nil {
		ctx.Errorf(err.Error())
	}

	ctx.Build(pctx, blueprint.BuildParams{
		Rule:     blueprint.Phony,
		Outputs:  []string{transMk.String()},
		Optional: true,
	})
}

func translateAndroidMk(ctx blueprint.SingletonContext, mkFile string, mods []AndroidModule) error {
	buf := &bytes.Buffer{}

	io.WriteString(buf, "LOCAL_PATH := $(TOP)\n")
	io.WriteString(buf, "LOCAL_MODULE_MAKEFILE := $(lastword $(MAKEFILE_LIST))\n")

	for _, mod := range mods {
		err := translateAndroidMkModule(ctx, buf, mod)
		if err != nil {
			os.Remove(mkFile)
			return err
		}
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

func translateAndroidMkModule(ctx blueprint.SingletonContext, w io.Writer, mod blueprint.Module) error {
	if mod != ctx.PrimaryModule(mod) {
		// These will be handled by the primary module
		return nil
	}

	name := ctx.ModuleName(mod)

	type hostClass struct {
		host     bool
		class    string
		multilib string
	}

	type archSrc struct {
		arch  Arch
		src   Path
		extra []string
	}

	srcs := make(map[hostClass][]archSrc)
	var modules []hostClass

	ctx.VisitAllModuleVariants(mod, func(m blueprint.Module) {
		provider, ok := m.(AndroidMkDataProvider)
		if !ok {
			return
		}

		amod := m.(AndroidModule).base()
		data := provider.AndroidMk()

		arch := amod.commonProperties.CompileArch

		prefix := ""
		if amod.HostOrDevice() == Host {
			if arch.ArchType != ctx.Config().(Config).HostArches[amod.HostType()][0].ArchType {
				prefix = "2ND_"
			}
		} else {
			if arch.ArchType != ctx.Config().(Config).DeviceArches[0].ArchType {
				prefix = "2ND_"
			}
		}

		if data.Custom != nil {
			data.Custom(w, name, prefix)
			return
		}

		if !data.OutputFile.Valid() {
			return
		}

		hC := hostClass{
			host:     amod.HostOrDevice() == Host,
			class:    data.Class,
			multilib: amod.commonProperties.Compile_multilib,
		}

		src := archSrc{
			arch: arch,
			src:  data.OutputFile.Path(),
		}

		if data.Extra != nil {
			src.extra = data.Extra(name, prefix, src.src, arch)
		}

		if srcs[hC] == nil {
			modules = append(modules, hC)
		}
		srcs[hC] = append(srcs[hC], src)
	})

	for _, hC := range modules {
		archSrcs := srcs[hC]

		io.WriteString(w, "\ninclude $(CLEAR_VARS)\n")
		io.WriteString(w, "LOCAL_MODULE := "+name+"\n")
		io.WriteString(w, "LOCAL_MODULE_CLASS := "+hC.class+"\n")
		io.WriteString(w, "LOCAL_MULTILIB := "+hC.multilib+"\n")

		printed := make(map[string]bool)
		for _, src := range archSrcs {
			io.WriteString(w, "LOCAL_SRC_FILES_"+src.arch.ArchType.String()+" := "+src.src.String()+"\n")

			for _, extra := range src.extra {
				if !printed[extra] {
					printed[extra] = true
					io.WriteString(w, extra+"\n")
				}
			}
		}

		if hC.host {
			// TODO: this isn't true for every module
			io.WriteString(w, "LOCAL_ACP_UNAVAILABLE := true\n")

			io.WriteString(w, "LOCAL_IS_HOST_MODULE := true\n")
		}
		io.WriteString(w, "include $(BUILD_PREBUILT)\n")
	}

	return nil
}
