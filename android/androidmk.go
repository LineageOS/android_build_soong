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
	RegisterAndroidMkBuildComponents(InitRegistrationContext)
}

func RegisterAndroidMkBuildComponents(ctx RegistrationContext) {
	ctx.RegisterSingletonType("androidmk", AndroidMkSingleton)
}

// Deprecated: consider using AndroidMkEntriesProvider instead, especially if you're not going to
// use the Custom function.
type AndroidMkDataProvider interface {
	AndroidMk() AndroidMkData
	BaseModuleName() string
}

type AndroidMkData struct {
	Class           string
	SubName         string
	DistFiles       TaggedDistFiles
	OutputFile      OptionalPath
	Disabled        bool
	Include         string
	Required        []string
	Host_required   []string
	Target_required []string

	Custom func(w io.Writer, name, prefix, moduleDir string, data AndroidMkData)

	Extra []AndroidMkExtraFunc

	preamble bytes.Buffer
}

type AndroidMkExtraFunc func(w io.Writer, outputFile Path)

// Allows modules to customize their Android*.mk output.
type AndroidMkEntriesProvider interface {
	AndroidMkEntries() []AndroidMkEntries
	BaseModuleName() string
}

type AndroidMkEntries struct {
	Class           string
	SubName         string
	DistFiles       TaggedDistFiles
	OutputFile      OptionalPath
	Disabled        bool
	Include         string
	Required        []string
	Host_required   []string
	Target_required []string

	header bytes.Buffer
	footer bytes.Buffer

	ExtraEntries []AndroidMkExtraEntriesFunc
	ExtraFooters []AndroidMkExtraFootersFunc

	EntryMap   map[string][]string
	entryOrder []string
}

type AndroidMkExtraEntriesFunc func(entries *AndroidMkEntries)
type AndroidMkExtraFootersFunc func(w io.Writer, name, prefix, moduleDir string, entries *AndroidMkEntries)

func (a *AndroidMkEntries) SetString(name, value string) {
	if _, ok := a.EntryMap[name]; !ok {
		a.entryOrder = append(a.entryOrder, name)
	}
	a.EntryMap[name] = []string{value}
}

func (a *AndroidMkEntries) SetPath(name string, path Path) {
	if _, ok := a.EntryMap[name]; !ok {
		a.entryOrder = append(a.entryOrder, name)
	}
	a.EntryMap[name] = []string{path.String()}
}

func (a *AndroidMkEntries) SetOptionalPath(name string, path OptionalPath) {
	if path.Valid() {
		a.SetPath(name, path.Path())
	}
}

func (a *AndroidMkEntries) AddPath(name string, path Path) {
	if _, ok := a.EntryMap[name]; !ok {
		a.entryOrder = append(a.entryOrder, name)
	}
	a.EntryMap[name] = append(a.EntryMap[name], path.String())
}

func (a *AndroidMkEntries) AddOptionalPath(name string, path OptionalPath) {
	if path.Valid() {
		a.AddPath(name, path.Path())
	}
}

func (a *AndroidMkEntries) SetPaths(name string, paths Paths) {
	if _, ok := a.EntryMap[name]; !ok {
		a.entryOrder = append(a.entryOrder, name)
	}
	a.EntryMap[name] = paths.Strings()
}

func (a *AndroidMkEntries) SetOptionalPaths(name string, paths Paths) {
	if len(paths) > 0 {
		a.SetPaths(name, paths)
	}
}

func (a *AndroidMkEntries) AddPaths(name string, paths Paths) {
	if _, ok := a.EntryMap[name]; !ok {
		a.entryOrder = append(a.entryOrder, name)
	}
	a.EntryMap[name] = append(a.EntryMap[name], paths.Strings()...)
}

func (a *AndroidMkEntries) SetBoolIfTrue(name string, flag bool) {
	if flag {
		if _, ok := a.EntryMap[name]; !ok {
			a.entryOrder = append(a.entryOrder, name)
		}
		a.EntryMap[name] = []string{"true"}
	}
}

func (a *AndroidMkEntries) SetBool(name string, flag bool) {
	if _, ok := a.EntryMap[name]; !ok {
		a.entryOrder = append(a.entryOrder, name)
	}
	if flag {
		a.EntryMap[name] = []string{"true"}
	} else {
		a.EntryMap[name] = []string{"false"}
	}
}

func (a *AndroidMkEntries) AddStrings(name string, value ...string) {
	if len(value) == 0 {
		return
	}
	if _, ok := a.EntryMap[name]; !ok {
		a.entryOrder = append(a.entryOrder, name)
	}
	a.EntryMap[name] = append(a.EntryMap[name], value...)
}

// AddCompatibilityTestSuites adds the supplied test suites to the EntryMap, with special handling
// for partial MTS test suites.
func (a *AndroidMkEntries) AddCompatibilityTestSuites(suites ...string) {
	// MTS supports a full test suite and partial per-module MTS test suites, with naming mts-${MODULE}.
	// To reduce repetition, if we find a partial MTS test suite without an full MTS test suite,
	// we add the full test suite to our list.
	if PrefixInList(suites, "mts-") && !InList("mts", suites) {
		suites = append(suites, "mts")
	}
	a.AddStrings("LOCAL_COMPATIBILITY_SUITE", suites...)
}

// Compute the list of Make strings to declare phone goals and dist-for-goals
// calls from the module's dist and dists properties.
func (a *AndroidMkEntries) GetDistForGoals(mod blueprint.Module) []string {
	amod := mod.(Module).base()
	name := amod.BaseModuleName()

	var ret []string
	var availableTaggedDists TaggedDistFiles

	if a.DistFiles != nil {
		availableTaggedDists = a.DistFiles
	} else if a.OutputFile.Valid() {
		availableTaggedDists = MakeDefaultDistFiles(a.OutputFile.Path())
	} else {
		// Nothing dist-able for this module.
		return nil
	}

	// Iterate over this module's dist structs, merged from the dist and dists properties.
	for _, dist := range amod.Dists() {
		// Get the list of goals this dist should be enabled for. e.g. sdk, droidcore
		goals := strings.Join(dist.Targets, " ")

		// Get the tag representing the output files to be dist'd. e.g. ".jar", ".proguard_map"
		var tag string
		if dist.Tag == nil {
			// If the dist struct does not specify a tag, use the default output files tag.
			tag = ""
		} else {
			tag = *dist.Tag
		}

		// Get the paths of the output files to be dist'd, represented by the tag.
		// Can be an empty list.
		tagPaths := availableTaggedDists[tag]
		if len(tagPaths) == 0 {
			// Nothing to dist for this tag, continue to the next dist.
			continue
		}

		if len(tagPaths) > 1 && (dist.Dest != nil || dist.Suffix != nil) {
			errorMessage := "Cannot apply dest/suffix for more than one dist " +
				"file for %s goals in module %s. The list of dist files, " +
				"which should have a single element, is:\n%s"
			panic(fmt.Errorf(errorMessage, goals, name, tagPaths))
		}

		ret = append(ret, fmt.Sprintf(".PHONY: %s\n", goals))

		// Create dist-for-goals calls for each path in the dist'd files.
		for _, path := range tagPaths {
			// It's possible that the Path is nil from errant modules. Be defensive here.
			if path == nil {
				tagName := "default" // for error message readability
				if dist.Tag != nil {
					tagName = *dist.Tag
				}
				panic(fmt.Errorf("Dist file should not be nil for the %s tag in %s", tagName, name))
			}

			dest := filepath.Base(path.String())

			if dist.Dest != nil {
				var err error
				if dest, err = validateSafePath(*dist.Dest); err != nil {
					// This was checked in ModuleBase.GenerateBuildActions
					panic(err)
				}
			}

			if dist.Suffix != nil {
				ext := filepath.Ext(dest)
				suffix := *dist.Suffix
				dest = strings.TrimSuffix(dest, ext) + suffix + ext
			}

			if dist.Dir != nil {
				var err error
				if dest, err = validateSafePath(*dist.Dir, dest); err != nil {
					// This was checked in ModuleBase.GenerateBuildActions
					panic(err)
				}
			}

			ret = append(
				ret,
				fmt.Sprintf("$(call dist-for-goals,%s,%s:%s)\n", goals, path.String(), dest))
		}
	}

	return ret
}

func (a *AndroidMkEntries) fillInEntries(config Config, bpPath string, mod blueprint.Module) {
	a.EntryMap = make(map[string][]string)
	amod := mod.(Module).base()
	name := amod.BaseModuleName()

	if a.Include == "" {
		a.Include = "$(BUILD_PREBUILT)"
	}
	a.Required = append(a.Required, amod.commonProperties.Required...)
	a.Host_required = append(a.Host_required, amod.commonProperties.Host_required...)
	a.Target_required = append(a.Target_required, amod.commonProperties.Target_required...)

	for _, distString := range a.GetDistForGoals(mod) {
		fmt.Fprintf(&a.header, distString)
	}

	fmt.Fprintln(&a.header, "\ninclude $(CLEAR_VARS)")

	// Collect make variable assignment entries.
	a.SetString("LOCAL_PATH", filepath.Dir(bpPath))
	a.SetString("LOCAL_MODULE", name+a.SubName)
	a.SetString("LOCAL_MODULE_CLASS", a.Class)
	a.SetString("LOCAL_PREBUILT_MODULE_FILE", a.OutputFile.String())
	a.AddStrings("LOCAL_REQUIRED_MODULES", a.Required...)
	a.AddStrings("LOCAL_HOST_REQUIRED_MODULES", a.Host_required...)
	a.AddStrings("LOCAL_TARGET_REQUIRED_MODULES", a.Target_required...)

	if am, ok := mod.(ApexModule); ok {
		a.SetBoolIfTrue("LOCAL_NOT_AVAILABLE_FOR_PLATFORM", am.NotAvailableForPlatform())
	}

	archStr := amod.Arch().ArchType.String()
	host := false
	switch amod.Os().Class {
	case Host:
		// Make cannot identify LOCAL_MODULE_HOST_ARCH:= common.
		if amod.Arch().ArchType != Common {
			a.SetString("LOCAL_MODULE_HOST_ARCH", archStr)
		}
		host = true
	case HostCross:
		// Make cannot identify LOCAL_MODULE_HOST_CROSS_ARCH:= common.
		if amod.Arch().ArchType != Common {
			a.SetString("LOCAL_MODULE_HOST_CROSS_ARCH", archStr)
		}
		host = true
	case Device:
		// Make cannot identify LOCAL_MODULE_TARGET_ARCH:= common.
		if amod.Arch().ArchType != Common {
			if amod.Target().NativeBridge {
				hostArchStr := amod.Target().NativeBridgeHostArchName
				if hostArchStr != "" {
					a.SetString("LOCAL_MODULE_TARGET_ARCH", hostArchStr)
				}
			} else {
				a.SetString("LOCAL_MODULE_TARGET_ARCH", archStr)
			}
		}

		a.AddStrings("LOCAL_INIT_RC", amod.commonProperties.Init_rc...)
		a.AddStrings("LOCAL_VINTF_FRAGMENTS", amod.commonProperties.Vintf_fragments...)
		a.SetBoolIfTrue("LOCAL_PROPRIETARY_MODULE", Bool(amod.commonProperties.Proprietary))
		if Bool(amod.commonProperties.Vendor) || Bool(amod.commonProperties.Soc_specific) {
			a.SetString("LOCAL_VENDOR_MODULE", "true")
		}
		a.SetBoolIfTrue("LOCAL_ODM_MODULE", Bool(amod.commonProperties.Device_specific))
		a.SetBoolIfTrue("LOCAL_PRODUCT_MODULE", Bool(amod.commonProperties.Product_specific))
		a.SetBoolIfTrue("LOCAL_SYSTEM_EXT_MODULE", Bool(amod.commonProperties.System_ext_specific))
		if amod.commonProperties.Owner != nil {
			a.SetString("LOCAL_MODULE_OWNER", *amod.commonProperties.Owner)
		}
	}

	if amod.noticeFile.Valid() {
		a.SetString("LOCAL_NOTICE_FILE", amod.noticeFile.String())
	}

	if host {
		makeOs := amod.Os().String()
		if amod.Os() == Linux || amod.Os() == LinuxBionic {
			makeOs = "linux"
		}
		a.SetString("LOCAL_MODULE_HOST_OS", makeOs)
		a.SetString("LOCAL_IS_HOST_MODULE", "true")
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

		if amod.Arch().ArchType != config.Targets[amod.Os()][0].Arch.ArchType {
			prefix = "2ND_" + prefix
		}
	}
	for _, extra := range a.ExtraEntries {
		extra(a)
	}

	// Write to footer.
	fmt.Fprintln(&a.footer, "include "+a.Include)
	blueprintDir := filepath.Dir(bpPath)
	for _, footerFunc := range a.ExtraFooters {
		footerFunc(&a.footer, name, prefix, blueprintDir, a)
	}
}

func (a *AndroidMkEntries) write(w io.Writer) {
	if a.Disabled {
		return
	}

	if !a.OutputFile.Valid() {
		return
	}

	w.Write(a.header.Bytes())
	for _, name := range a.entryOrder {
		fmt.Fprintln(w, name+" := "+strings.Join(a.EntryMap[name], " "))
	}
	w.Write(a.footer.Bytes())
}

func (a *AndroidMkEntries) FooterLinesForTests() []string {
	return strings.Split(string(a.footer.Bytes()), "\n")
}

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

	err := translateAndroidMk(ctx, absolutePath(transMk.String()), androidMkModulesList)
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
	if _, err := os.Stat(absolutePath(mkFile)); !os.IsNotExist(err) {
		if data, err := ioutil.ReadFile(absolutePath(mkFile)); err == nil {
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

	return ioutil.WriteFile(absolutePath(mkFile), buf.Bytes(), 0666)
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
	case AndroidMkEntriesProvider:
		return translateAndroidMkEntriesModule(ctx, w, mod, x)
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

func (data *AndroidMkData) fillInData(config Config, bpPath string, mod blueprint.Module) {
	// Get the preamble content through AndroidMkEntries logic.
	entries := AndroidMkEntries{
		Class:           data.Class,
		SubName:         data.SubName,
		DistFiles:       data.DistFiles,
		OutputFile:      data.OutputFile,
		Disabled:        data.Disabled,
		Include:         data.Include,
		Required:        data.Required,
		Host_required:   data.Host_required,
		Target_required: data.Target_required,
	}
	entries.fillInEntries(config, bpPath, mod)

	// preamble doesn't need the footer content.
	entries.footer = bytes.Buffer{}
	entries.write(&data.preamble)

	// copy entries back to data since it is used in Custom
	data.Required = entries.Required
	data.Host_required = entries.Host_required
	data.Target_required = entries.Target_required
}

func translateAndroidModule(ctx SingletonContext, w io.Writer, mod blueprint.Module,
	provider AndroidMkDataProvider) error {

	amod := mod.(Module).base()
	if shouldSkipAndroidMkProcessing(amod) {
		return nil
	}

	data := provider.AndroidMk()
	if data.Include == "" {
		data.Include = "$(BUILD_PREBUILT)"
	}

	data.fillInData(ctx.Config(), ctx.BlueprintFile(mod), mod)

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

	name := provider.BaseModuleName()
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

func translateAndroidMkEntriesModule(ctx SingletonContext, w io.Writer, mod blueprint.Module,
	provider AndroidMkEntriesProvider) error {
	if shouldSkipAndroidMkProcessing(mod.(Module).base()) {
		return nil
	}

	for _, entries := range provider.AndroidMkEntries() {
		entries.fillInEntries(ctx.Config(), ctx.BlueprintFile(mod), mod)
		entries.write(w)
	}

	return nil
}

func shouldSkipAndroidMkProcessing(module *ModuleBase) bool {
	if !module.commonProperties.NamespaceExportedToMake {
		// TODO(jeffrygaston) do we want to validate that there are no modules being
		// exported to Kati that depend on this module?
		return true
	}

	return !module.Enabled() ||
		module.commonProperties.SkipInstall ||
		// Make does not understand LinuxBionic
		module.Os() == LinuxBionic
}
