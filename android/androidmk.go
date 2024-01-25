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

// This file offers AndroidMkEntriesProvider, which individual modules implement to output
// Android.mk entries that contain information about the modules built through Soong. Kati reads
// and combines them with the legacy Make-based module definitions to produce the complete view of
// the source tree, which makes this a critical point of Make-Soong interoperability.
//
// Naturally, Soong-only builds do not rely on this mechanism.

package android

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/bootstrap"
	"github.com/google/blueprint/pathtools"
)

func init() {
	RegisterAndroidMkBuildComponents(InitRegistrationContext)
}

func RegisterAndroidMkBuildComponents(ctx RegistrationContext) {
	ctx.RegisterParallelSingletonType("androidmk", AndroidMkSingleton)
}

// Enable androidmk support.
// * Register the singleton
// * Configure that we are inside make
var PrepareForTestWithAndroidMk = GroupFixturePreparers(
	FixtureRegisterWithContext(RegisterAndroidMkBuildComponents),
	FixtureModifyConfig(SetKatiEnabledForTests),
)

// Deprecated: Use AndroidMkEntriesProvider instead, especially if you're not going to use the
// Custom function. It's easier to use and test.
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

	Entries AndroidMkEntries
}

type AndroidMkExtraFunc func(w io.Writer, outputFile Path)

// Interface for modules to declare their Android.mk outputs. Note that every module needs to
// implement this in order to be included in the final Android-<product_name>.mk output, even if
// they only need to output the common set of entries without any customizations.
type AndroidMkEntriesProvider interface {
	// Returns AndroidMkEntries objects that contain all basic info plus extra customization data
	// if needed. This is the core func to implement.
	// Note that one can return multiple objects. For example, java_library may return an additional
	// AndroidMkEntries object for its hostdex sub-module.
	AndroidMkEntries() []AndroidMkEntries
	// Modules don't need to implement this as it's already implemented by ModuleBase.
	// AndroidMkEntries uses BaseModuleName() instead of ModuleName() because certain modules
	// e.g. Prebuilts, override the Name() func and return modified names.
	// If a different name is preferred, use SubName or OverrideName in AndroidMkEntries.
	BaseModuleName() string
}

// The core data struct that modules use to provide their Android.mk data.
type AndroidMkEntries struct {
	// Android.mk class string, e.g EXECUTABLES, JAVA_LIBRARIES, ETC
	Class string
	// Optional suffix to append to the module name. Useful when a module wants to return multiple
	// AndroidMkEntries objects. For example, when a java_library returns an additional entry for
	// its hostdex sub-module, this SubName field is set to "-hostdex" so that it can have a
	// different name than the parent's.
	SubName string
	// If set, this value overrides the base module name. SubName is still appended.
	OverrideName string
	// Dist files to output
	DistFiles TaggedDistFiles
	// The output file for Kati to process and/or install. If absent, the module is skipped.
	OutputFile OptionalPath
	// If true, the module is skipped and does not appear on the final Android-<product name>.mk
	// file. Useful when a module needs to be skipped conditionally.
	Disabled bool
	// The postprocessing mk file to include, e.g. $(BUILD_SYSTEM)/soong_cc_rust_prebuilt.mk
	// If not set, $(BUILD_SYSTEM)/prebuilt.mk is used.
	Include string
	// Required modules that need to be built and included in the final build output when building
	// this module.
	Required []string
	// Required host modules that need to be built and included in the final build output when
	// building this module.
	Host_required []string
	// Required device modules that need to be built and included in the final build output when
	// building this module.
	Target_required []string

	header bytes.Buffer
	footer bytes.Buffer

	// Funcs to append additional Android.mk entries or modify the common ones. Multiple funcs are
	// accepted so that common logic can be factored out as a shared func.
	ExtraEntries []AndroidMkExtraEntriesFunc
	// Funcs to add extra lines to the module's Android.mk output. Unlike AndroidMkExtraEntriesFunc,
	// which simply sets Make variable values, this can be used for anything since it can write any
	// Make statements directly to the final Android-*.mk file.
	// Primarily used to call macros or declare/update Make targets.
	ExtraFooters []AndroidMkExtraFootersFunc

	// A map that holds the up-to-date Make variable values. Can be accessed from tests.
	EntryMap map[string][]string
	// A list of EntryMap keys in insertion order. This serves a few purposes:
	// 1. Prevents churns. Golang map doesn't provide consistent iteration order, so without this,
	// the outputted Android-*.mk file may change even though there have been no content changes.
	// 2. Allows modules to refer to other variables, like LOCAL_BAR_VAR := $(LOCAL_FOO_VAR),
	// without worrying about the variables being mixed up in the actual mk file.
	// 3. Makes troubleshooting and spotting errors easier.
	entryOrder []string

	// Provides data typically stored by Context objects that are commonly needed by
	//AndroidMkEntries objects.
	entryContext AndroidMkEntriesContext
}

type AndroidMkEntriesContext interface {
	Config() Config
}

type AndroidMkExtraEntriesContext interface {
	Provider(provider blueprint.AnyProviderKey) (any, bool)
}

type androidMkExtraEntriesContext struct {
	ctx fillInEntriesContext
	mod blueprint.Module
}

func (a *androidMkExtraEntriesContext) Provider(provider blueprint.AnyProviderKey) (any, bool) {
	return a.ctx.moduleProvider(a.mod, provider)
}

type AndroidMkExtraEntriesFunc func(ctx AndroidMkExtraEntriesContext, entries *AndroidMkEntries)
type AndroidMkExtraFootersFunc func(w io.Writer, name, prefix, moduleDir string)

// Utility funcs to manipulate Android.mk variable entries.

// SetString sets a Make variable with the given name to the given value.
func (a *AndroidMkEntries) SetString(name, value string) {
	if _, ok := a.EntryMap[name]; !ok {
		a.entryOrder = append(a.entryOrder, name)
	}
	a.EntryMap[name] = []string{value}
}

// SetPath sets a Make variable with the given name to the given path string.
func (a *AndroidMkEntries) SetPath(name string, path Path) {
	if _, ok := a.EntryMap[name]; !ok {
		a.entryOrder = append(a.entryOrder, name)
	}
	a.EntryMap[name] = []string{path.String()}
}

// SetOptionalPath sets a Make variable with the given name to the given path string if it is valid.
// It is a no-op if the given path is invalid.
func (a *AndroidMkEntries) SetOptionalPath(name string, path OptionalPath) {
	if path.Valid() {
		a.SetPath(name, path.Path())
	}
}

// AddPath appends the given path string to a Make variable with the given name.
func (a *AndroidMkEntries) AddPath(name string, path Path) {
	if _, ok := a.EntryMap[name]; !ok {
		a.entryOrder = append(a.entryOrder, name)
	}
	a.EntryMap[name] = append(a.EntryMap[name], path.String())
}

// AddOptionalPath appends the given path string to a Make variable with the given name if it is
// valid. It is a no-op if the given path is invalid.
func (a *AndroidMkEntries) AddOptionalPath(name string, path OptionalPath) {
	if path.Valid() {
		a.AddPath(name, path.Path())
	}
}

// SetPaths sets a Make variable with the given name to a slice of the given path strings.
func (a *AndroidMkEntries) SetPaths(name string, paths Paths) {
	if _, ok := a.EntryMap[name]; !ok {
		a.entryOrder = append(a.entryOrder, name)
	}
	a.EntryMap[name] = paths.Strings()
}

// SetOptionalPaths sets a Make variable with the given name to a slice of the given path strings
// only if there are a non-zero amount of paths.
func (a *AndroidMkEntries) SetOptionalPaths(name string, paths Paths) {
	if len(paths) > 0 {
		a.SetPaths(name, paths)
	}
}

// AddPaths appends the given path strings to a Make variable with the given name.
func (a *AndroidMkEntries) AddPaths(name string, paths Paths) {
	if _, ok := a.EntryMap[name]; !ok {
		a.entryOrder = append(a.entryOrder, name)
	}
	a.EntryMap[name] = append(a.EntryMap[name], paths.Strings()...)
}

// SetBoolIfTrue sets a Make variable with the given name to true if the given flag is true.
// It is a no-op if the given flag is false.
func (a *AndroidMkEntries) SetBoolIfTrue(name string, flag bool) {
	if flag {
		if _, ok := a.EntryMap[name]; !ok {
			a.entryOrder = append(a.entryOrder, name)
		}
		a.EntryMap[name] = []string{"true"}
	}
}

// SetBool sets a Make variable with the given name to if the given bool flag value.
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

// AddStrings appends the given strings to a Make variable with the given name.
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
// for partial MTS and MCTS test suites.
func (a *AndroidMkEntries) AddCompatibilityTestSuites(suites ...string) {
	// M(C)TS supports a full test suite and partial per-module MTS test suites, with naming mts-${MODULE}.
	// To reduce repetition, if we find a partial M(C)TS test suite without an full M(C)TS test suite,
	// we add the full test suite to our list.
	if PrefixInList(suites, "mts-") && !InList("mts", suites) {
		suites = append(suites, "mts")
	}
	if PrefixInList(suites, "mcts-") && !InList("mcts", suites) {
		suites = append(suites, "mcts")
	}
	a.AddStrings("LOCAL_COMPATIBILITY_SUITE", suites...)
}

// The contributions to the dist.
type distContributions struct {
	// Path to license metadata file.
	licenseMetadataFile Path
	// List of goals and the dist copy instructions.
	copiesForGoals []*copiesForGoals
}

// getCopiesForGoals returns a copiesForGoals into which copy instructions that
// must be processed when building one or more of those goals can be added.
func (d *distContributions) getCopiesForGoals(goals string) *copiesForGoals {
	copiesForGoals := &copiesForGoals{goals: goals}
	d.copiesForGoals = append(d.copiesForGoals, copiesForGoals)
	return copiesForGoals
}

// Associates a list of dist copy instructions with a set of goals for which they
// should be run.
type copiesForGoals struct {
	// goals are a space separated list of build targets that will trigger the
	// copy instructions.
	goals string

	// A list of instructions to copy a module's output files to somewhere in the
	// dist directory.
	copies []distCopy
}

// Adds a copy instruction.
func (d *copiesForGoals) addCopyInstruction(from Path, dest string) {
	d.copies = append(d.copies, distCopy{from, dest})
}

// Instruction on a path that must be copied into the dist.
type distCopy struct {
	// The path to copy from.
	from Path

	// The destination within the dist directory to copy to.
	dest string
}

// Compute the contributions that the module makes to the dist.
func (a *AndroidMkEntries) getDistContributions(mod blueprint.Module) *distContributions {
	amod := mod.(Module).base()
	name := amod.BaseModuleName()

	// Collate the set of associated tag/paths available for copying to the dist.
	// Start with an empty (nil) set.
	var availableTaggedDists TaggedDistFiles

	// Then merge in any that are provided explicitly by the module.
	if a.DistFiles != nil {
		// Merge the DistFiles into the set.
		availableTaggedDists = availableTaggedDists.merge(a.DistFiles)
	}

	// If no paths have been provided for the DefaultDistTag and the output file is
	// valid then add that as the default dist path.
	if _, ok := availableTaggedDists[DefaultDistTag]; !ok && a.OutputFile.Valid() {
		availableTaggedDists = availableTaggedDists.addPathsForTag(DefaultDistTag, a.OutputFile.Path())
	}

	// If the distFiles created by GenerateTaggedDistFiles contains paths for the
	// DefaultDistTag then that takes priority so delete any existing paths.
	if _, ok := amod.distFiles[DefaultDistTag]; ok {
		delete(availableTaggedDists, DefaultDistTag)
	}

	// Finally, merge the distFiles created by GenerateTaggedDistFiles.
	availableTaggedDists = availableTaggedDists.merge(amod.distFiles)

	if len(availableTaggedDists) == 0 {
		// Nothing dist-able for this module.
		return nil
	}

	// Collate the contributions this module makes to the dist.
	distContributions := &distContributions{}

	if !exemptFromRequiredApplicableLicensesProperty(mod.(Module)) {
		distContributions.licenseMetadataFile = amod.licenseMetadataFile
	}

	// Iterate over this module's dist structs, merged from the dist and dists properties.
	for _, dist := range amod.Dists() {
		// Get the list of goals this dist should be enabled for. e.g. sdk, droidcore
		goals := strings.Join(dist.Targets, " ")

		// Get the tag representing the output files to be dist'd. e.g. ".jar", ".proguard_map"
		var tag string
		if dist.Tag == nil {
			// If the dist struct does not specify a tag, use the default output files tag.
			tag = DefaultDistTag
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
			errorMessage := "%s: Cannot apply dest/suffix for more than one dist " +
				"file for %q goals tag %q in module %s. The list of dist files, " +
				"which should have a single element, is:\n%s"
			panic(fmt.Errorf(errorMessage, mod, goals, tag, name, tagPaths))
		}

		copiesForGoals := distContributions.getCopiesForGoals(goals)

		// Iterate over each path adding a copy instruction to copiesForGoals
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

			ext := filepath.Ext(dest)
			suffix := ""
			if dist.Suffix != nil {
				suffix = *dist.Suffix
			}

			productString := ""
			if dist.Append_artifact_with_product != nil && *dist.Append_artifact_with_product {
				productString = fmt.Sprintf("_%s", a.entryContext.Config().DeviceProduct())
			}

			if suffix != "" || productString != "" {
				dest = strings.TrimSuffix(dest, ext) + suffix + productString + ext
			}

			if dist.Dir != nil {
				var err error
				if dest, err = validateSafePath(*dist.Dir, dest); err != nil {
					// This was checked in ModuleBase.GenerateBuildActions
					panic(err)
				}
			}

			copiesForGoals.addCopyInstruction(path, dest)
		}
	}

	return distContributions
}

// generateDistContributionsForMake generates make rules that will generate the
// dist according to the instructions in the supplied distContribution.
func generateDistContributionsForMake(distContributions *distContributions) []string {
	var ret []string
	for _, d := range distContributions.copiesForGoals {
		ret = append(ret, fmt.Sprintf(".PHONY: %s\n", d.goals))
		// Create dist-for-goals calls for each of the copy instructions.
		for _, c := range d.copies {
			if distContributions.licenseMetadataFile != nil {
				ret = append(
					ret,
					fmt.Sprintf("$(if $(strip $(ALL_TARGETS.%s.META_LIC)),,$(eval ALL_TARGETS.%s.META_LIC := %s))\n",
						c.from.String(), c.from.String(), distContributions.licenseMetadataFile.String()))
			}
			ret = append(
				ret,
				fmt.Sprintf("$(call dist-for-goals,%s,%s:%s)\n", d.goals, c.from.String(), c.dest))
		}
	}

	return ret
}

// Compute the list of Make strings to declare phony goals and dist-for-goals
// calls from the module's dist and dists properties.
func (a *AndroidMkEntries) GetDistForGoals(mod blueprint.Module) []string {
	distContributions := a.getDistContributions(mod)
	if distContributions == nil {
		return nil
	}

	return generateDistContributionsForMake(distContributions)
}

// fillInEntries goes through the common variable processing and calls the extra data funcs to
// generate and fill in AndroidMkEntries's in-struct data, ready to be flushed to a file.
type fillInEntriesContext interface {
	ModuleDir(module blueprint.Module) string
	ModuleSubDir(module blueprint.Module) string
	Config() Config
	moduleProvider(module blueprint.Module, provider blueprint.AnyProviderKey) (any, bool)
	ModuleType(module blueprint.Module) string
}

func (a *AndroidMkEntries) fillInEntries(ctx fillInEntriesContext, mod blueprint.Module) {
	a.entryContext = ctx
	a.EntryMap = make(map[string][]string)
	amod := mod.(Module)
	base := amod.base()
	name := base.BaseModuleName()
	if a.OverrideName != "" {
		name = a.OverrideName
	}

	if a.Include == "" {
		a.Include = "$(BUILD_PREBUILT)"
	}
	a.Required = append(a.Required, amod.RequiredModuleNames()...)
	a.Host_required = append(a.Host_required, amod.HostRequiredModuleNames()...)
	a.Target_required = append(a.Target_required, amod.TargetRequiredModuleNames()...)

	for _, distString := range a.GetDistForGoals(mod) {
		fmt.Fprintf(&a.header, distString)
	}

	fmt.Fprintf(&a.header, "\ninclude $(CLEAR_VARS)  # type: %s, name: %s, variant: %s\n", ctx.ModuleType(mod), base.BaseModuleName(), ctx.ModuleSubDir(mod))

	// Collect make variable assignment entries.
	a.SetString("LOCAL_PATH", ctx.ModuleDir(mod))
	a.SetString("LOCAL_MODULE", name+a.SubName)
	a.SetString("LOCAL_MODULE_CLASS", a.Class)
	a.SetString("LOCAL_PREBUILT_MODULE_FILE", a.OutputFile.String())
	a.AddStrings("LOCAL_REQUIRED_MODULES", a.Required...)
	a.AddStrings("LOCAL_HOST_REQUIRED_MODULES", a.Host_required...)
	a.AddStrings("LOCAL_TARGET_REQUIRED_MODULES", a.Target_required...)
	a.AddStrings("LOCAL_SOONG_MODULE_TYPE", ctx.ModuleType(amod))

	// If the install rule was generated by Soong tell Make about it.
	if len(base.katiInstalls) > 0 {
		// Assume the primary install file is last since it probably needs to depend on any other
		// installed files.  If that is not the case we can add a method to specify the primary
		// installed file.
		a.SetPath("LOCAL_SOONG_INSTALLED_MODULE", base.katiInstalls[len(base.katiInstalls)-1].to)
		a.SetString("LOCAL_SOONG_INSTALL_PAIRS", base.katiInstalls.BuiltInstalled())
		a.SetPaths("LOCAL_SOONG_INSTALL_SYMLINKS", base.katiSymlinks.InstallPaths().Paths())
	}

	if len(base.testData) > 0 {
		a.AddStrings("LOCAL_TEST_DATA", androidMkDataPaths(base.testData)...)
	}

	if am, ok := mod.(ApexModule); ok {
		a.SetBoolIfTrue("LOCAL_NOT_AVAILABLE_FOR_PLATFORM", am.NotAvailableForPlatform())
	}

	archStr := base.Arch().ArchType.String()
	host := false
	switch base.Os().Class {
	case Host:
		if base.Target().HostCross {
			// Make cannot identify LOCAL_MODULE_HOST_CROSS_ARCH:= common.
			if base.Arch().ArchType != Common {
				a.SetString("LOCAL_MODULE_HOST_CROSS_ARCH", archStr)
			}
		} else {
			// Make cannot identify LOCAL_MODULE_HOST_ARCH:= common.
			if base.Arch().ArchType != Common {
				a.SetString("LOCAL_MODULE_HOST_ARCH", archStr)
			}
		}
		host = true
	case Device:
		// Make cannot identify LOCAL_MODULE_TARGET_ARCH:= common.
		if base.Arch().ArchType != Common {
			if base.Target().NativeBridge {
				hostArchStr := base.Target().NativeBridgeHostArchName
				if hostArchStr != "" {
					a.SetString("LOCAL_MODULE_TARGET_ARCH", hostArchStr)
				}
			} else {
				a.SetString("LOCAL_MODULE_TARGET_ARCH", archStr)
			}
		}

		if !base.InVendorRamdisk() {
			a.AddPaths("LOCAL_FULL_INIT_RC", base.initRcPaths)
		}
		if len(base.vintfFragmentsPaths) > 0 {
			a.AddPaths("LOCAL_FULL_VINTF_FRAGMENTS", base.vintfFragmentsPaths)
		}
		a.SetBoolIfTrue("LOCAL_PROPRIETARY_MODULE", Bool(base.commonProperties.Proprietary))
		if Bool(base.commonProperties.Vendor) || Bool(base.commonProperties.Soc_specific) {
			a.SetString("LOCAL_VENDOR_MODULE", "true")
		}
		a.SetBoolIfTrue("LOCAL_ODM_MODULE", Bool(base.commonProperties.Device_specific))
		a.SetBoolIfTrue("LOCAL_PRODUCT_MODULE", Bool(base.commonProperties.Product_specific))
		a.SetBoolIfTrue("LOCAL_SYSTEM_EXT_MODULE", Bool(base.commonProperties.System_ext_specific))
		if base.commonProperties.Owner != nil {
			a.SetString("LOCAL_MODULE_OWNER", *base.commonProperties.Owner)
		}
	}

	if host {
		makeOs := base.Os().String()
		if base.Os() == Linux || base.Os() == LinuxBionic || base.Os() == LinuxMusl {
			makeOs = "linux"
		}
		a.SetString("LOCAL_MODULE_HOST_OS", makeOs)
		a.SetString("LOCAL_IS_HOST_MODULE", "true")
	}

	prefix := ""
	if base.ArchSpecific() {
		switch base.Os().Class {
		case Host:
			if base.Target().HostCross {
				prefix = "HOST_CROSS_"
			} else {
				prefix = "HOST_"
			}
		case Device:
			prefix = "TARGET_"

		}

		if base.Arch().ArchType != ctx.Config().Targets[base.Os()][0].Arch.ArchType {
			prefix = "2ND_" + prefix
		}
	}

	if licenseMetadata, ok := SingletonModuleProvider(ctx, mod, LicenseMetadataProvider); ok {
		a.SetPath("LOCAL_SOONG_LICENSE_METADATA", licenseMetadata.LicenseMetadataPath)
	}

	if _, ok := SingletonModuleProvider(ctx, mod, ModuleInfoJSONProvider); ok {
		a.SetBool("LOCAL_SOONG_MODULE_INFO_JSON", true)
	}

	extraCtx := &androidMkExtraEntriesContext{
		ctx: ctx,
		mod: mod,
	}

	for _, extra := range a.ExtraEntries {
		extra(extraCtx, a)
	}

	// Write to footer.
	fmt.Fprintln(&a.footer, "include "+a.Include)
	blueprintDir := ctx.ModuleDir(mod)
	for _, footerFunc := range a.ExtraFooters {
		footerFunc(&a.footer, name, prefix, blueprintDir)
	}
}

func (a *AndroidMkEntries) disabled() bool {
	return a.Disabled || !a.OutputFile.Valid()
}

// write  flushes the AndroidMkEntries's in-struct data populated by AndroidMkEntries into the
// given Writer object.
func (a *AndroidMkEntries) write(w io.Writer) {
	if a.disabled() {
		return
	}

	w.Write(a.header.Bytes())
	for _, name := range a.entryOrder {
		AndroidMkEmitAssignList(w, name, a.EntryMap[name])
	}
	w.Write(a.footer.Bytes())
}

func (a *AndroidMkEntries) FooterLinesForTests() []string {
	return strings.Split(string(a.footer.Bytes()), "\n")
}

// AndroidMkSingleton is a singleton to collect Android.mk data from all modules and dump them into
// the final Android-<product_name>.mk file output.
func AndroidMkSingleton() Singleton {
	return &androidMkSingleton{}
}

type androidMkSingleton struct{}

func (c *androidMkSingleton) GenerateBuildActions(ctx SingletonContext) {
	// Skip if Soong wasn't invoked from Make.
	if !ctx.Config().KatiEnabled() {
		return
	}

	var androidMkModulesList []blueprint.Module

	ctx.VisitAllModulesBlueprint(func(module blueprint.Module) {
		androidMkModulesList = append(androidMkModulesList, module)
	})

	// Sort the module list by the module names to eliminate random churns, which may erroneously
	// invoke additional build processes.
	sort.SliceStable(androidMkModulesList, func(i, j int) bool {
		return ctx.ModuleName(androidMkModulesList[i]) < ctx.ModuleName(androidMkModulesList[j])
	})

	transMk := PathForOutput(ctx, "Android"+String(ctx.Config().productVariables.Make_suffix)+".mk")
	if ctx.Failed() {
		return
	}

	moduleInfoJSON := PathForOutput(ctx, "module-info"+String(ctx.Config().productVariables.Make_suffix)+".json")

	err := translateAndroidMk(ctx, absolutePath(transMk.String()), moduleInfoJSON, androidMkModulesList)
	if err != nil {
		ctx.Errorf(err.Error())
	}

	ctx.Build(pctx, BuildParams{
		Rule:   blueprint.Phony,
		Output: transMk,
	})
}

func translateAndroidMk(ctx SingletonContext, absMkFile string, moduleInfoJSONPath WritablePath, mods []blueprint.Module) error {
	buf := &bytes.Buffer{}

	var moduleInfoJSONs []*ModuleInfoJSON

	fmt.Fprintln(buf, "LOCAL_MODULE_MAKEFILE := $(lastword $(MAKEFILE_LIST))")

	typeStats := make(map[string]int)
	for _, mod := range mods {
		err := translateAndroidMkModule(ctx, buf, &moduleInfoJSONs, mod)
		if err != nil {
			os.Remove(absMkFile)
			return err
		}

		if amod, ok := mod.(Module); ok && ctx.PrimaryModule(amod) == amod {
			typeStats[ctx.ModuleType(amod)] += 1
		}
	}

	keys := []string{}
	fmt.Fprintln(buf, "\nSTATS.SOONG_MODULE_TYPE :=")
	for k := range typeStats {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, mod_type := range keys {
		fmt.Fprintln(buf, "STATS.SOONG_MODULE_TYPE +=", mod_type)
		fmt.Fprintf(buf, "STATS.SOONG_MODULE_TYPE.%s := %d\n", mod_type, typeStats[mod_type])
	}

	err := pathtools.WriteFileIfChanged(absMkFile, buf.Bytes(), 0666)
	if err != nil {
		return err
	}

	return writeModuleInfoJSON(ctx, moduleInfoJSONs, moduleInfoJSONPath)
}

func writeModuleInfoJSON(ctx SingletonContext, moduleInfoJSONs []*ModuleInfoJSON, moduleInfoJSONPath WritablePath) error {
	moduleInfoJSONBuf := &strings.Builder{}
	moduleInfoJSONBuf.WriteString("[")
	for i, moduleInfoJSON := range moduleInfoJSONs {
		if i != 0 {
			moduleInfoJSONBuf.WriteString(",\n")
		}
		moduleInfoJSONBuf.WriteString("{")
		moduleInfoJSONBuf.WriteString(strconv.Quote(moduleInfoJSON.core.RegisterName))
		moduleInfoJSONBuf.WriteString(":")
		err := encodeModuleInfoJSON(moduleInfoJSONBuf, moduleInfoJSON)
		moduleInfoJSONBuf.WriteString("}")
		if err != nil {
			return err
		}
	}
	moduleInfoJSONBuf.WriteString("]")
	WriteFileRule(ctx, moduleInfoJSONPath, moduleInfoJSONBuf.String())
	return nil
}

func translateAndroidMkModule(ctx SingletonContext, w io.Writer, moduleInfoJSONs *[]*ModuleInfoJSON, mod blueprint.Module) error {
	defer func() {
		if r := recover(); r != nil {
			panic(fmt.Errorf("%s in translateAndroidMkModule for module %s variant %s",
				r, ctx.ModuleName(mod), ctx.ModuleSubDir(mod)))
		}
	}()

	// Additional cases here require review for correct license propagation to make.
	var err error
	switch x := mod.(type) {
	case AndroidMkDataProvider:
		err = translateAndroidModule(ctx, w, moduleInfoJSONs, mod, x)
	case bootstrap.GoBinaryTool:
		err = translateGoBinaryModule(ctx, w, mod, x)
	case AndroidMkEntriesProvider:
		err = translateAndroidMkEntriesModule(ctx, w, moduleInfoJSONs, mod, x)
	default:
		// Not exported to make so no make variables to set.
	}

	if err != nil {
		return err
	}

	return err
}

// A simple, special Android.mk entry output func to make it possible to build blueprint tools using
// m by making them phony targets.
func translateGoBinaryModule(ctx SingletonContext, w io.Writer, mod blueprint.Module,
	goBinary bootstrap.GoBinaryTool) error {

	name := ctx.ModuleName(mod)
	fmt.Fprintln(w, ".PHONY:", name)
	fmt.Fprintln(w, name+":", goBinary.InstallPath())
	fmt.Fprintln(w, "")
	// Assuming no rules in make include go binaries in distributables.
	// If the assumption is wrong, make will fail to build without the necessary .meta_lic and .meta_module files.
	// In that case, add the targets and rules here to build a .meta_lic file for `name` and a .meta_module for
	// `goBinary.InstallPath()` pointing to the `name`.meta_lic file.

	return nil
}

func (data *AndroidMkData) fillInData(ctx fillInEntriesContext, mod blueprint.Module) {
	// Get the preamble content through AndroidMkEntries logic.
	data.Entries = AndroidMkEntries{
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
	data.Entries.fillInEntries(ctx, mod)

	// copy entries back to data since it is used in Custom
	data.Required = data.Entries.Required
	data.Host_required = data.Entries.Host_required
	data.Target_required = data.Entries.Target_required
}

// A support func for the deprecated AndroidMkDataProvider interface. Use AndroidMkEntryProvider
// instead.
func translateAndroidModule(ctx SingletonContext, w io.Writer, moduleInfoJSONs *[]*ModuleInfoJSON,
	mod blueprint.Module, provider AndroidMkDataProvider) error {

	amod := mod.(Module).base()
	if shouldSkipAndroidMkProcessing(amod) {
		return nil
	}

	data := provider.AndroidMk()
	if data.Include == "" {
		data.Include = "$(BUILD_PREBUILT)"
	}

	data.fillInData(ctx, mod)
	aconfigUpdateAndroidMkData(ctx, mod.(Module), &data)

	prefix := ""
	if amod.ArchSpecific() {
		switch amod.Os().Class {
		case Host:
			if amod.Target().HostCross {
				prefix = "HOST_CROSS_"
			} else {
				prefix = "HOST_"
			}
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
		// List of module types allowed to use .Custom(...)
		// Additions to the list require careful review for proper license handling.
		switch reflect.TypeOf(mod).String() { // ctx.ModuleType(mod) doesn't work: aidl_interface creates phony without type
		case "*aidl.aidlApi": // writes non-custom before adding .phony
		case "*aidl.aidlMapping": // writes non-custom before adding .phony
		case "*android.customModule": // appears in tests only
		case "*android_sdk.sdkRepoHost": // doesn't go through base_rules
		case "*apex.apexBundle": // license properties written
		case "*bpf.bpf": // license properties written (both for module and objs)
		case "*genrule.Module": // writes non-custom before adding .phony
		case "*java.SystemModules": // doesn't go through base_rules
		case "*java.systemModulesImport": // doesn't go through base_rules
		case "*phony.phony": // license properties written
		case "*phony.PhonyRule": // writes phony deps and acts like `.PHONY`
		case "*selinux.selinuxContextsModule": // license properties written
		case "*sysprop.syspropLibrary": // license properties written
		default:
			if !ctx.Config().IsEnvFalse("ANDROID_REQUIRE_LICENSES") {
				return fmt.Errorf("custom make rules not allowed for %q (%q) module %q", ctx.ModuleType(mod), reflect.TypeOf(mod), ctx.ModuleName(mod))
			}
		}
		data.Custom(w, name, prefix, blueprintDir, data)
	} else {
		WriteAndroidMkData(w, data)
	}

	if !data.Entries.disabled() {
		if moduleInfoJSON, ok := SingletonModuleProvider(ctx, mod, ModuleInfoJSONProvider); ok {
			*moduleInfoJSONs = append(*moduleInfoJSONs, moduleInfoJSON)
		}
	}

	return nil
}

// A support func for the deprecated AndroidMkDataProvider interface. Use AndroidMkEntryProvider
// instead.
func WriteAndroidMkData(w io.Writer, data AndroidMkData) {
	if data.Entries.disabled() {
		return
	}

	// write preamble via Entries
	data.Entries.footer = bytes.Buffer{}
	data.Entries.write(w)

	for _, extra := range data.Extra {
		extra(w, data.OutputFile.Path())
	}

	fmt.Fprintln(w, "include "+data.Include)
}

func translateAndroidMkEntriesModule(ctx SingletonContext, w io.Writer, moduleInfoJSONs *[]*ModuleInfoJSON,
	mod blueprint.Module, provider AndroidMkEntriesProvider) error {
	if shouldSkipAndroidMkProcessing(mod.(Module).base()) {
		return nil
	}

	entriesList := provider.AndroidMkEntries()
	aconfigUpdateAndroidMkEntries(ctx, mod.(Module), &entriesList)

	// Any new or special cases here need review to verify correct propagation of license information.
	for _, entries := range entriesList {
		entries.fillInEntries(ctx, mod)
		entries.write(w)
	}

	if len(entriesList) > 0 && !entriesList[0].disabled() {
		if moduleInfoJSON, ok := SingletonModuleProvider(ctx, mod, ModuleInfoJSONProvider); ok {
			*moduleInfoJSONs = append(*moduleInfoJSONs, moduleInfoJSON)
		}
	}

	return nil
}

func ShouldSkipAndroidMkProcessing(module Module) bool {
	return shouldSkipAndroidMkProcessing(module.base())
}

func shouldSkipAndroidMkProcessing(module *ModuleBase) bool {
	if !module.commonProperties.NamespaceExportedToMake {
		// TODO(jeffrygaston) do we want to validate that there are no modules being
		// exported to Kati that depend on this module?
		return true
	}

	// On Mac, only expose host darwin modules to Make, as that's all we claim to support.
	// In reality, some of them depend on device-built (Java) modules, so we can't disable all
	// device modules in Soong, but we can hide them from Make (and thus the build user interface)
	if runtime.GOOS == "darwin" && module.Os() != Darwin {
		return true
	}

	// Only expose the primary Darwin target, as Make does not understand Darwin+Arm64
	if module.Os() == Darwin && module.Target().HostCross {
		return true
	}

	return !module.Enabled() ||
		module.commonProperties.HideFromMake ||
		// Make does not understand LinuxBionic
		module.Os() == LinuxBionic ||
		// Make does not understand LinuxMusl, except when we are building with USE_HOST_MUSL=true
		// and all host binaries are LinuxMusl
		(module.Os() == LinuxMusl && module.Target().HostCross)
}

// A utility func to format LOCAL_TEST_DATA outputs. See the comments on DataPath to understand how
// to use this func.
func androidMkDataPaths(data []DataPath) []string {
	var testFiles []string
	for _, d := range data {
		rel := d.SrcPath.Rel()
		if d.WithoutRel {
			rel = d.SrcPath.Base()
		}
		path := d.SrcPath.String()
		// LOCAL_TEST_DATA requires the rel portion of the path to be removed from the path.
		if !strings.HasSuffix(path, rel) {
			panic(fmt.Errorf("path %q does not end with %q", path, rel))
		}
		path = strings.TrimSuffix(path, rel)
		testFileString := path + ":" + rel
		if len(d.RelativeInstallPath) > 0 {
			testFileString += ":" + d.RelativeInstallPath
		}
		testFiles = append(testFiles, testFileString)
	}
	return testFiles
}

// AndroidMkEmitAssignList emits the line
//
//	VAR := ITEM ...
//
// Items are the elements to the given set of lists
// If all the passed lists are empty, no line will be emitted
func AndroidMkEmitAssignList(w io.Writer, varName string, lists ...[]string) {
	doPrint := false
	for _, l := range lists {
		if doPrint = len(l) > 0; doPrint {
			break
		}
	}
	if !doPrint {
		return
	}
	fmt.Fprint(w, varName, " :=")
	for _, l := range lists {
		for _, item := range l {
			fmt.Fprint(w, " ", item)
		}
	}
	fmt.Fprintln(w)
}
