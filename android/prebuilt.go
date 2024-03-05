// Copyright 2016 Google Inc. All rights reserved.
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
	"fmt"
	"reflect"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

// This file implements common functionality for handling modules that may exist as prebuilts,
// source, or both.

func RegisterPrebuiltMutators(ctx RegistrationContext) {
	ctx.PreArchMutators(RegisterPrebuiltsPreArchMutators)
	ctx.PostDepsMutators(RegisterPrebuiltsPostDepsMutators)
}

// Marks a dependency tag as possibly preventing a reference to a source from being
// replaced with the prebuilt.
type ReplaceSourceWithPrebuilt interface {
	blueprint.DependencyTag

	// Return true if the dependency defined by this tag should be replaced with the
	// prebuilt.
	ReplaceSourceWithPrebuilt() bool
}

type prebuiltDependencyTag struct {
	blueprint.BaseDependencyTag
}

var PrebuiltDepTag prebuiltDependencyTag

// Mark this tag so dependencies that use it are excluded from visibility enforcement.
func (t prebuiltDependencyTag) ExcludeFromVisibilityEnforcement() {}

// Mark this tag so dependencies that use it are excluded from APEX contents.
func (t prebuiltDependencyTag) ExcludeFromApexContents() {}

var _ ExcludeFromVisibilityEnforcementTag = PrebuiltDepTag
var _ ExcludeFromApexContentsTag = PrebuiltDepTag

// UserSuppliedPrebuiltProperties contains the prebuilt properties that can be specified in an
// Android.bp file.
type UserSuppliedPrebuiltProperties struct {
	// When prefer is set to true the prebuilt will be used instead of any source module with
	// a matching name.
	Prefer *bool `android:"arch_variant"`

	// When specified this names a Soong config variable that controls the prefer property.
	//
	// If the value of the named Soong config variable is true then prefer is set to false and vice
	// versa. If the Soong config variable is not set then it defaults to false, so prefer defaults
	// to true.
	//
	// If specified then the prefer property is ignored in favor of the value of the Soong config
	// variable.
	Use_source_config_var *ConfigVarProperties
}

// CopyUserSuppliedPropertiesFromPrebuilt copies the user supplied prebuilt properties from the
// prebuilt properties.
func (u *UserSuppliedPrebuiltProperties) CopyUserSuppliedPropertiesFromPrebuilt(p *Prebuilt) {
	*u = p.properties.UserSuppliedPrebuiltProperties
}

type PrebuiltProperties struct {
	UserSuppliedPrebuiltProperties

	SourceExists bool `blueprint:"mutated"`
	UsePrebuilt  bool `blueprint:"mutated"`

	// Set if the module has been renamed to remove the "prebuilt_" prefix.
	PrebuiltRenamedToSource bool `blueprint:"mutated"`
}

// Properties that can be used to select a Soong config variable.
type ConfigVarProperties struct {
	// Allow instances of this struct to be used as a property value in a BpPropertySet.
	BpPrintableBase

	// The name of the configuration namespace.
	//
	// As passed to add_soong_config_namespace in Make.
	Config_namespace *string

	// The name of the configuration variable.
	//
	// As passed to add_soong_config_var_value in Make.
	Var_name *string
}

type Prebuilt struct {
	properties PrebuiltProperties

	// nil if the prebuilt has no srcs property at all. See InitPrebuiltModuleWithoutSrcs.
	srcsSupplier PrebuiltSrcsSupplier

	// "-" if the prebuilt has no srcs property at all. See InitPrebuiltModuleWithoutSrcs.
	srcsPropertyName string
}

// RemoveOptionalPrebuiltPrefix returns the result of removing the "prebuilt_" prefix from the
// supplied name if it has one, or returns the name unmodified if it does not.
func RemoveOptionalPrebuiltPrefix(name string) string {
	return strings.TrimPrefix(name, "prebuilt_")
}

// RemoveOptionalPrebuiltPrefixFromBazelLabel removes the "prebuilt_" prefix from the *target name* of a Bazel label.
// This differs from RemoveOptionalPrebuiltPrefix in that it does not remove it from the start of the string, but
// instead removes it from the target name itself.
func RemoveOptionalPrebuiltPrefixFromBazelLabel(label string) string {
	splitLabel := strings.Split(label, ":")
	bazelModuleNameNoPrebuilt := RemoveOptionalPrebuiltPrefix(splitLabel[1])
	return strings.Join([]string{
		splitLabel[0],
		bazelModuleNameNoPrebuilt,
	}, ":")
}

func (p *Prebuilt) Name(name string) string {
	return PrebuiltNameFromSource(name)
}

// PrebuiltNameFromSource returns the result of prepending the "prebuilt_" prefix to the supplied
// name.
func PrebuiltNameFromSource(name string) string {
	return "prebuilt_" + name
}

func (p *Prebuilt) ForcePrefer() {
	p.properties.Prefer = proptools.BoolPtr(true)
}

func (p *Prebuilt) Prefer() bool {
	return proptools.Bool(p.properties.Prefer)
}

// SingleSourcePathFromSupplier invokes the supplied supplier for the current module in the
// supplied context to retrieve a list of file paths, ensures that the returned list of file paths
// contains a single value and then assumes that is a module relative file path and converts it to
// a Path accordingly.
//
// Any issues, such as nil supplier or not exactly one file path will be reported as errors on the
// supplied context and this will return nil.
func SingleSourcePathFromSupplier(ctx ModuleContext, srcsSupplier PrebuiltSrcsSupplier, srcsPropertyName string) Path {
	if srcsSupplier != nil {
		srcs := srcsSupplier(ctx, ctx.Module())

		if len(srcs) == 0 {
			ctx.PropertyErrorf(srcsPropertyName, "missing prebuilt source file")
			return nil
		}

		if len(srcs) > 1 {
			ctx.PropertyErrorf(srcsPropertyName, "multiple prebuilt source files")
			return nil
		}

		// Return the singleton source after expanding any filegroup in the
		// sources.
		src := srcs[0]
		return PathForModuleSrc(ctx, src)
	} else {
		ctx.ModuleErrorf("prebuilt source was not set")
		return nil
	}
}

// The below source-related functions and the srcs, src fields are based on an assumption that
// prebuilt modules have a static source property at the moment. Currently there is only one
// exception, android_app_import, which chooses a source file depending on the product's DPI
// preference configs. We'll want to add native support for dynamic source cases if we end up having
// more modules like this.
func (p *Prebuilt) SingleSourcePath(ctx ModuleContext) Path {
	return SingleSourcePathFromSupplier(ctx, p.srcsSupplier, p.srcsPropertyName)
}

func (p *Prebuilt) UsePrebuilt() bool {
	return p.properties.UsePrebuilt
}

// Called to provide the srcs value for the prebuilt module.
//
// This can be called with a context for any module not just the prebuilt one itself. It can also be
// called concurrently.
//
// Return the src value or nil if it is not available.
type PrebuiltSrcsSupplier func(ctx BaseModuleContext, prebuilt Module) []string

func initPrebuiltModuleCommon(module PrebuiltInterface) *Prebuilt {
	p := module.Prebuilt()
	module.AddProperties(&p.properties)
	return p
}

// Initialize the module as a prebuilt module that has no dedicated property that lists its
// sources. SingleSourcePathFromSupplier should not be called for this module.
//
// This is the case e.g. for header modules, which provides the headers in source form
// regardless whether they are prebuilt or not.
func InitPrebuiltModuleWithoutSrcs(module PrebuiltInterface) {
	p := initPrebuiltModuleCommon(module)
	p.srcsPropertyName = "-"
}

// Initialize the module as a prebuilt module that uses the provided supplier to access the
// prebuilt sources of the module.
//
// The supplier will be called multiple times and must return the same values each time it
// is called. If it returns an empty array (or nil) then the prebuilt module will not be used
// as a replacement for a source module with the same name even if prefer = true.
//
// If the Prebuilt.SingleSourcePath() is called on the module then this must return an array
// containing exactly one source file.
//
// The provided property name is used to provide helpful error messages in the event that
// a problem arises, e.g. calling SingleSourcePath() when more than one source is provided.
func InitPrebuiltModuleWithSrcSupplier(module PrebuiltInterface, srcsSupplier PrebuiltSrcsSupplier, srcsPropertyName string) {
	if srcsSupplier == nil {
		panic(fmt.Errorf("srcsSupplier must not be nil"))
	}
	if srcsPropertyName == "" {
		panic(fmt.Errorf("srcsPropertyName must not be empty"))
	}

	p := initPrebuiltModuleCommon(module)
	p.srcsSupplier = srcsSupplier
	p.srcsPropertyName = srcsPropertyName
}

func InitPrebuiltModule(module PrebuiltInterface, srcs *[]string) {
	if srcs == nil {
		panic(fmt.Errorf("srcs must not be nil"))
	}

	srcsSupplier := func(ctx BaseModuleContext, _ Module) []string {
		return *srcs
	}

	InitPrebuiltModuleWithSrcSupplier(module, srcsSupplier, "srcs")
}

func InitSingleSourcePrebuiltModule(module PrebuiltInterface, srcProps interface{}, srcField string) {
	srcPropsValue := reflect.ValueOf(srcProps).Elem()
	srcStructField, _ := srcPropsValue.Type().FieldByName(srcField)
	if !srcPropsValue.IsValid() || srcStructField.Name == "" {
		panic(fmt.Errorf("invalid single source prebuilt %+v", module))
	}

	if srcPropsValue.Kind() != reflect.Struct && srcPropsValue.Kind() != reflect.Interface {
		panic(fmt.Errorf("invalid single source prebuilt %+v", srcProps))
	}

	srcFieldIndex := srcStructField.Index
	srcPropertyName := proptools.PropertyNameForField(srcField)

	srcsSupplier := func(ctx BaseModuleContext, _ Module) []string {
		if !module.Enabled() {
			return nil
		}
		value := srcPropsValue.FieldByIndex(srcFieldIndex)
		if value.Kind() == reflect.Ptr {
			if value.IsNil() {
				return nil
			}
			value = value.Elem()
		}
		if value.Kind() != reflect.String {
			panic(fmt.Errorf("prebuilt src field %q in %T in module %s should be a string or a pointer to one but was %v", srcField, srcProps, module, value))
		}
		src := value.String()
		if src == "" {
			return nil
		}
		return []string{src}
	}

	InitPrebuiltModuleWithSrcSupplier(module, srcsSupplier, srcPropertyName)
}

type PrebuiltInterface interface {
	Module
	Prebuilt() *Prebuilt
}

// IsModulePreferred returns true if the given module is preferred.
//
// A source module is preferred if there is no corresponding prebuilt module or the prebuilt module
// does not have "prefer: true".
//
// A prebuilt module is preferred if there is no corresponding source module or the prebuilt module
// has "prefer: true".
func IsModulePreferred(module Module) bool {
	if module.IsReplacedByPrebuilt() {
		// A source module that has been replaced by a prebuilt counterpart.
		return false
	}
	if p := GetEmbeddedPrebuilt(module); p != nil {
		return p.UsePrebuilt()
	}
	return true
}

// IsModulePrebuilt returns true if the module implements PrebuiltInterface and
// has been initialized as a prebuilt and so returns a non-nil value from the
// PrebuiltInterface.Prebuilt() method.
func IsModulePrebuilt(module Module) bool {
	return GetEmbeddedPrebuilt(module) != nil
}

// GetEmbeddedPrebuilt returns a pointer to the embedded Prebuilt structure or
// nil if the module does not implement PrebuiltInterface or has not been
// initialized as a prebuilt module.
func GetEmbeddedPrebuilt(module Module) *Prebuilt {
	if p, ok := module.(PrebuiltInterface); ok {
		return p.Prebuilt()
	}

	return nil
}

// PrebuiltGetPreferred returns the module that is preferred for the given
// module. That is either the module itself or the prebuilt counterpart that has
// taken its place. The given module must be a direct dependency of the current
// context module, and it must be the source module if both source and prebuilt
// exist.
//
// This function is for use on dependencies after PrebuiltPostDepsMutator has
// run - any dependency that is registered before that will already reference
// the right module. This function is only safe to call after all mutators that
// may call CreateVariations, e.g. in GenerateAndroidBuildActions.
func PrebuiltGetPreferred(ctx BaseModuleContext, module Module) Module {
	if !module.IsReplacedByPrebuilt() {
		return module
	}
	if IsModulePrebuilt(module) {
		// If we're given a prebuilt then assume there's no source module around.
		return module
	}

	sourceModDepFound := false
	var prebuiltMod Module

	ctx.WalkDeps(func(child, parent Module) bool {
		if prebuiltMod != nil {
			return false
		}
		if parent == ctx.Module() {
			// First level: Only recurse if the module is found as a direct dependency.
			sourceModDepFound = child == module
			return sourceModDepFound
		}
		// Second level: Follow PrebuiltDepTag to the prebuilt.
		if t := ctx.OtherModuleDependencyTag(child); t == PrebuiltDepTag {
			prebuiltMod = child
		}
		return false
	})

	if prebuiltMod == nil {
		if !sourceModDepFound {
			panic(fmt.Errorf("Failed to find source module as a direct dependency: %s", module))
		} else {
			panic(fmt.Errorf("Failed to find prebuilt for source module: %s", module))
		}
	}
	return prebuiltMod
}

func RegisterPrebuiltsPreArchMutators(ctx RegisterMutatorsContext) {
	ctx.BottomUp("prebuilt_rename", PrebuiltRenameMutator).Parallel()
}

func RegisterPrebuiltsPostDepsMutators(ctx RegisterMutatorsContext) {
	ctx.BottomUp("prebuilt_source", PrebuiltSourceDepsMutator).Parallel()
	ctx.BottomUp("prebuilt_select", PrebuiltSelectModuleMutator).Parallel()
	ctx.BottomUp("prebuilt_postdeps", PrebuiltPostDepsMutator).Parallel()
}

// Returns the name of the source module corresponding to a prebuilt module
// For source modules, it returns its own name
type baseModuleName interface {
	BaseModuleName() string
}

// PrebuiltRenameMutator ensures that there always is a module with an
// undecorated name.
func PrebuiltRenameMutator(ctx BottomUpMutatorContext) {
	m := ctx.Module()
	if p := GetEmbeddedPrebuilt(m); p != nil {
		bmn, _ := m.(baseModuleName)
		name := bmn.BaseModuleName()
		if !ctx.OtherModuleExists(name) {
			ctx.Rename(name)
			p.properties.PrebuiltRenamedToSource = true
		}
	}
}

// PrebuiltSourceDepsMutator adds dependencies to the prebuilt module from the
// corresponding source module, if one exists for the same variant.
// Add a dependency from the prebuilt to `all_apex_contributions`
// The metadata will be used for source vs prebuilts selection
func PrebuiltSourceDepsMutator(ctx BottomUpMutatorContext) {
	m := ctx.Module()
	// If this module is a prebuilt, is enabled and has not been renamed to source then add a
	// dependency onto the source if it is present.
	if p := GetEmbeddedPrebuilt(m); p != nil && m.Enabled() && !p.properties.PrebuiltRenamedToSource {
		bmn, _ := m.(baseModuleName)
		name := bmn.BaseModuleName()
		if ctx.OtherModuleReverseDependencyVariantExists(name) {
			ctx.AddReverseDependency(ctx.Module(), PrebuiltDepTag, name)
			p.properties.SourceExists = true
		}
		// Add a dependency from the prebuilt to the `all_apex_contributions`
		// metadata module
		// TODO: When all branches contain this singleton module, make this strict
		// TODO: Add this dependency only for mainline prebuilts and not every prebuilt module
		if ctx.OtherModuleExists("all_apex_contributions") {
			ctx.AddDependency(m, acDepTag, "all_apex_contributions")
		}

	}
}

// checkInvariantsForSourceAndPrebuilt checks if invariants are kept when replacing
// source with prebuilt. Note that the current module for the context is the source module.
func checkInvariantsForSourceAndPrebuilt(ctx BaseModuleContext, s, p Module) {
	if _, ok := s.(OverrideModule); ok {
		// skip the check when the source module is `override_X` because it's only a placeholder
		// for the actual source module. The check will be invoked for the actual module.
		return
	}
	if sourcePartition, prebuiltPartition := s.PartitionTag(ctx.DeviceConfig()), p.PartitionTag(ctx.DeviceConfig()); sourcePartition != prebuiltPartition {
		ctx.OtherModuleErrorf(p, "partition is different: %s(%s) != %s(%s)",
			sourcePartition, ctx.ModuleName(), prebuiltPartition, ctx.OtherModuleName(p))
	}
}

// PrebuiltSelectModuleMutator marks prebuilts that are used, either overriding source modules or
// because the source module doesn't exist.  It also disables installing overridden source modules.
//
// If the visited module is the metadata module `all_apex_contributions`, it sets a
// provider containing metadata about whether source or prebuilt of mainline modules should be used.
// This logic was added here to prevent the overhead of creating a new mutator.
func PrebuiltSelectModuleMutator(ctx BottomUpMutatorContext) {
	m := ctx.Module()
	if p := GetEmbeddedPrebuilt(m); p != nil {
		if p.srcsSupplier == nil && p.srcsPropertyName == "" {
			panic(fmt.Errorf("prebuilt module did not have InitPrebuiltModule called on it"))
		}
		if !p.properties.SourceExists {
			p.properties.UsePrebuilt = p.usePrebuilt(ctx, nil, m)
		}
		// Propagate the provider received from `all_apex_contributions`
		// to the source module
		ctx.VisitDirectDepsWithTag(acDepTag, func(am Module) {
			psi, _ := OtherModuleProvider(ctx, am, PrebuiltSelectionInfoProvider)
			SetProvider(ctx, PrebuiltSelectionInfoProvider, psi)
		})

	} else if s, ok := ctx.Module().(Module); ok {
		// Use `all_apex_contributions` for source vs prebuilt selection.
		psi := PrebuiltSelectionInfoMap{}
		ctx.VisitDirectDepsWithTag(PrebuiltDepTag, func(am Module) {
			// The value of psi gets overwritten with the provider from the last visited prebuilt.
			// But all prebuilts have the same value of the provider, so this should be idempontent.
			psi, _ = OtherModuleProvider(ctx, am, PrebuiltSelectionInfoProvider)
		})
		ctx.VisitDirectDepsWithTag(PrebuiltDepTag, func(prebuiltModule Module) {
			p := GetEmbeddedPrebuilt(prebuiltModule)
			if p.usePrebuilt(ctx, s, prebuiltModule) {
				checkInvariantsForSourceAndPrebuilt(ctx, s, prebuiltModule)

				p.properties.UsePrebuilt = true
				s.ReplacedByPrebuilt()
			}
		})

		// If any module in this mainline module family has been flagged using apex_contributions, disable every other module in that family
		// Add source
		allModules := []Module{s}
		// Add each prebuilt
		ctx.VisitDirectDepsWithTag(PrebuiltDepTag, func(prebuiltModule Module) {
			allModules = append(allModules, prebuiltModule)
		})
		hideUnflaggedModules(ctx, psi, allModules)

	}

	// If this is `all_apex_contributions`, set a provider containing
	// metadata about source vs prebuilts selection
	if am, ok := m.(*allApexContributions); ok {
		am.SetPrebuiltSelectionInfoProvider(ctx)
	}
}

// If any module in this mainline module family has been flagged using apex_contributions, disable every other module in that family
func hideUnflaggedModules(ctx BottomUpMutatorContext, psi PrebuiltSelectionInfoMap, allModulesInFamily []Module) {
	var selectedModuleInFamily Module
	// query all_apex_contributions to see if any module in this family has been selected
	for _, moduleInFamily := range allModulesInFamily {
		// validate that are no duplicates
		if isSelected(psi, moduleInFamily) {
			if selectedModuleInFamily == nil {
				// Store this so we can validate that there are no duplicates
				selectedModuleInFamily = moduleInFamily
			} else {
				// There are duplicate modules from the same mainline module family
				ctx.ModuleErrorf("Found duplicate variations of the same module in apex_contributions: %s and %s. Please remove one of these.\n", selectedModuleInFamily.Name(), moduleInFamily.Name())
			}
		}
	}

	// If a module has been selected, hide all other modules
	if selectedModuleInFamily != nil {
		for _, moduleInFamily := range allModulesInFamily {
			if moduleInFamily.Name() != selectedModuleInFamily.Name() {
				moduleInFamily.HideFromMake()
				// If this is a prebuilt module, unset properties.UsePrebuilt
				// properties.UsePrebuilt might evaluate to true via soong config var fallback mechanism
				// Set it to false explicitly so that the following mutator does not replace rdeps to this unselected prebuilt
				if p := GetEmbeddedPrebuilt(moduleInFamily); p != nil {
					p.properties.UsePrebuilt = false
				}
			}
		}
	}
	// Do a validation pass to make sure that multiple prebuilts of a specific module are not selected.
	// This might happen if the prebuilts share the same soong config var namespace.
	// This should be an error, unless one of the prebuilts has been explicitly declared in apex_contributions
	var selectedPrebuilt Module
	for _, moduleInFamily := range allModulesInFamily {
		// Skip if this module is in a different namespace
		if !moduleInFamily.ExportedToMake() {
			continue
		}
		// Skip for the top-level java_sdk_library_(_import). This has some special cases that need to be addressed first.
		// This does not run into non-determinism because PrebuiltPostDepsMutator also has the special case
		if sdkLibrary, ok := moduleInFamily.(interface{ SdkLibraryName() *string }); ok && sdkLibrary.SdkLibraryName() != nil {
			continue
		}
		if p := GetEmbeddedPrebuilt(moduleInFamily); p != nil && p.properties.UsePrebuilt {
			if selectedPrebuilt == nil {
				selectedPrebuilt = moduleInFamily
			} else {
				ctx.ModuleErrorf("Multiple prebuilt modules %v and %v have been marked as preferred for this source module. "+
					"Please add the appropriate prebuilt module to apex_contributions for this release config.", selectedPrebuilt.Name(), moduleInFamily.Name())
			}
		}
	}
}

// PrebuiltPostDepsMutator replaces dependencies on the source module with dependencies on the
// prebuilt when both modules exist and the prebuilt should be used.  When the prebuilt should not
// be used, disable installing it.
func PrebuiltPostDepsMutator(ctx BottomUpMutatorContext) {
	m := ctx.Module()
	if p := GetEmbeddedPrebuilt(m); p != nil {
		bmn, _ := m.(baseModuleName)
		name := bmn.BaseModuleName()
		psi := PrebuiltSelectionInfoMap{}
		ctx.VisitDirectDepsWithTag(acDepTag, func(am Module) {
			psi, _ = OtherModuleProvider(ctx, am, PrebuiltSelectionInfoProvider)
		})

		if p.properties.UsePrebuilt {
			if p.properties.SourceExists {
				ctx.ReplaceDependenciesIf(name, func(from blueprint.Module, tag blueprint.DependencyTag, to blueprint.Module) bool {
					if sdkLibrary, ok := m.(interface{ SdkLibraryName() *string }); ok && sdkLibrary.SdkLibraryName() != nil {
						// Do not replace deps to the top-level prebuilt java_sdk_library hook.
						// This hook has been special-cased in #isSelected to be _always_ active, even in next builds
						// for dexpreopt and hiddenapi processing.
						// If we do not special-case this here, rdeps referring to a java_sdk_library in next builds via libs
						// will get prebuilt stubs
						// TODO (b/308187268): Remove this after the apexes have been added to apex_contributions
						if psi.IsSelected(name) {
							return false
						}
					}

					if t, ok := tag.(ReplaceSourceWithPrebuilt); ok {
						return t.ReplaceSourceWithPrebuilt()
					}
					return true
				})
			}
		} else {
			m.HideFromMake()
		}
	}
}

// A wrapper around PrebuiltSelectionInfoMap.IsSelected with special handling for java_sdk_library
// java_sdk_library is a macro that creates
// 1. top-level impl library
// 2. stub libraries (suffixed with .stubs...)
//
// java_sdk_library_import is a macro that creates
// 1. top-level "impl" library
// 2. stub libraries (suffixed with .stubs...)
//
// the impl of java_sdk_library_import is a "hook" for hiddenapi and dexpreopt processing. It does not have an impl jar, but acts as a shim
// to provide the jar deapxed from the prebuilt apex
//
// isSelected uses `all_apex_contributions` to supersede source vs prebuilts selection of the stub libraries. It does not supersede the
// selection of the top-level "impl" library so that this hook can work
//
// TODO (b/308174306) - Fix this when we need to support multiple prebuilts in main
func isSelected(psi PrebuiltSelectionInfoMap, m Module) bool {
	if sdkLibrary, ok := m.(interface{ SdkLibraryName() *string }); ok && sdkLibrary.SdkLibraryName() != nil {
		sln := proptools.String(sdkLibrary.SdkLibraryName())

		// This is the top-level library
		// Do not supersede the existing prebuilts vs source selection mechanisms
		// TODO (b/308187268): Remove this after the apexes have been added to apex_contributions
		if bmn, ok := m.(baseModuleName); ok && sln == bmn.BaseModuleName() {
			return false
		}

		// Stub library created by java_sdk_library_import
		// java_sdk_library creates several child modules (java_import + prebuilt_stubs_sources) dynamically.
		// This code block ensures that these child modules are selected if the top-level java_sdk_library_import is listed
		// in the selected apex_contributions.
		if javaImport, ok := m.(createdByJavaSdkLibraryName); ok && javaImport.CreatedByJavaSdkLibraryName() != nil {
			return psi.IsSelected(PrebuiltNameFromSource(proptools.String(javaImport.CreatedByJavaSdkLibraryName())))
		}

		// Stub library created by java_sdk_library
		return psi.IsSelected(sln)
	}
	return psi.IsSelected(m.Name())
}

// implemented by child modules of java_sdk_library_import
type createdByJavaSdkLibraryName interface {
	CreatedByJavaSdkLibraryName() *string
}

// Returns true if the prebuilt variant is disabled
// e.g. for a cc_prebuilt_library_shared, this will return
// - true for the static variant of the module
// - false for the shared variant of the module
//
// Even though this is a cc_prebuilt_library_shared, we create both the variants today
// https://source.corp.google.com/h/googleplex-android/platform/build/soong/+/e08e32b45a18a77bc3c3e751f730539b1b374f1b:cc/library.go;l=2113-2116;drc=2c4a9779cd1921d0397a12b3d3521f4c9b30d747;bpv=1;bpt=0
func (p *Prebuilt) variantIsDisabled(ctx BaseMutatorContext, prebuilt Module) bool {
	return p.srcsSupplier != nil && len(p.srcsSupplier(ctx, prebuilt)) == 0
}

// usePrebuilt returns true if a prebuilt should be used instead of the source module.  The prebuilt
// will be used if it is marked "prefer" or if the source module is disabled.
func (p *Prebuilt) usePrebuilt(ctx BaseMutatorContext, source Module, prebuilt Module) bool {
	// Use `all_apex_contributions` for source vs prebuilt selection.
	psi := PrebuiltSelectionInfoMap{}
	ctx.VisitDirectDepsWithTag(PrebuiltDepTag, func(am Module) {
		psi, _ = OtherModuleProvider(ctx, am, PrebuiltSelectionInfoProvider)
	})

	// If the source module is explicitly listed in the metadata module, use that
	if source != nil && isSelected(psi, source) {
		return false
	}
	// If the prebuilt module is explicitly listed in the metadata module, use that
	if isSelected(psi, prebuilt) && !p.variantIsDisabled(ctx, prebuilt) {
		return true
	}

	// If the baseModuleName could not be found in the metadata module,
	// fall back to the existing source vs prebuilt selection.
	// TODO: Drop the fallback mechanisms

	if p.variantIsDisabled(ctx, prebuilt) {
		return false
	}

	// Skip prebuilt modules under unexported namespaces so that we won't
	// end up shadowing non-prebuilt module when prebuilt module under same
	// name happens to have a `Prefer` property set to true.
	if ctx.Config().KatiEnabled() && !prebuilt.ExportedToMake() {
		return false
	}

	// If source is not available or is disabled then always use the prebuilt.
	if source == nil || !source.Enabled() {
		return true
	}

	// If the use_source_config_var property is set then it overrides the prefer property setting.
	if configVar := p.properties.Use_source_config_var; configVar != nil {
		return !ctx.Config().VendorConfig(proptools.String(configVar.Config_namespace)).Bool(proptools.String(configVar.Var_name))
	}

	// TODO: use p.Properties.Name and ctx.ModuleDir to override preference
	return Bool(p.properties.Prefer)
}

func (p *Prebuilt) SourceExists() bool {
	return p.properties.SourceExists
}
