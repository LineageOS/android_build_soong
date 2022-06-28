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
	"reflect"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

type defaultsDependencyTag struct {
	blueprint.BaseDependencyTag
}

var DefaultsDepTag defaultsDependencyTag

type defaultsProperties struct {
	Defaults []string
}

type DefaultableModuleBase struct {
	defaultsProperties            defaultsProperties
	defaultableProperties         []interface{}
	defaultableVariableProperties interface{}

	// The optional hook to call after any defaults have been applied.
	hook DefaultableHook
}

func (d *DefaultableModuleBase) defaults() *defaultsProperties {
	return &d.defaultsProperties
}

func (d *DefaultableModuleBase) setProperties(props []interface{}, variableProperties interface{}) {
	d.defaultableProperties = props
	d.defaultableVariableProperties = variableProperties
}

func (d *DefaultableModuleBase) SetDefaultableHook(hook DefaultableHook) {
	d.hook = hook
}

func (d *DefaultableModuleBase) callHookIfAvailable(ctx DefaultableHookContext) {
	if d.hook != nil {
		d.hook(ctx)
	}
}

// Interface that must be supported by any module to which defaults can be applied.
type Defaultable interface {
	// Get a pointer to the struct containing the Defaults property.
	defaults() *defaultsProperties

	// Set the property structures into which defaults will be added.
	setProperties(props []interface{}, variableProperties interface{})

	// Apply defaults from the supplied DefaultsModule to the property structures supplied to
	// setProperties(...).
	applyDefaults(TopDownMutatorContext, []DefaultsModule)

	applySingleDefaultsWithTracker(EarlyModuleContext, DefaultsModule, defaultsTrackerFunc)

	// Set the hook to be called after any defaults have been applied.
	//
	// Should be used in preference to a AddLoadHook when the behavior of the load
	// hook is dependent on properties supplied in the Android.bp file.
	SetDefaultableHook(hook DefaultableHook)

	// Call the hook if specified.
	callHookIfAvailable(context DefaultableHookContext)
}

type DefaultableModule interface {
	Module
	Defaultable
}

var _ Defaultable = (*DefaultableModuleBase)(nil)

func InitDefaultableModule(module DefaultableModule) {
	if module.base().module == nil {
		panic("InitAndroidModule must be called before InitDefaultableModule")
	}

	module.setProperties(module.GetProperties(), module.base().variableProperties)

	module.AddProperties(module.defaults())
}

// A restricted subset of context methods, similar to LoadHookContext.
type DefaultableHookContext interface {
	EarlyModuleContext

	CreateModule(ModuleFactory, ...interface{}) Module
	AddMissingDependencies(missingDeps []string)
}

type DefaultableHook func(ctx DefaultableHookContext)

// The Defaults_visibility property.
type DefaultsVisibilityProperties struct {

	// Controls the visibility of the defaults module itself.
	Defaults_visibility []string
}

// AdditionalDefaultsProperties contains properties of defaults modules which
// can have other defaults applied.
type AdditionalDefaultsProperties struct {

	// The list of properties set by the default whose values must not be changed by any module that
	// applies these defaults. It is an error if a property is not supported by the defaults module or
	// has not been set to a non-zero value. If this contains "*" then that must be the only entry in
	// which case all properties that are set on this defaults will be protected (except the
	// protected_properties and visibility.
	Protected_properties []string
}

type DefaultsModuleBase struct {
	DefaultableModuleBase

	defaultsProperties AdditionalDefaultsProperties

	// Included to support setting bazel_module.label for multiple Soong modules to the same Bazel
	// target. This is primarily useful for modules that were architecture specific and instead are
	// handled in Bazel as a select().
	BazelModuleBase
}

// The common pattern for defaults modules is to register separate instances of
// the xxxProperties structs in the AddProperties calls, rather than reusing the
// ones inherited from Module.
//
// The effect is that e.g. myDefaultsModuleInstance.base().xxxProperties won't
// contain the values that have been set for the defaults module. Rather, to
// retrieve the values it is necessary to iterate over properties(). E.g. to get
// the commonProperties instance that have the real values:
//
//   d := myModule.(Defaults)
//   for _, props := range d.properties() {
//     if cp, ok := props.(*commonProperties); ok {
//       ... access property values in cp ...
//     }
//   }
//
// The rationale is that the properties on a defaults module apply to the
// defaultable modules using it, not to the defaults module itself. E.g. setting
// the "enabled" property false makes inheriting modules disabled by default,
// rather than disabling the defaults module itself.
type Defaults interface {
	Defaultable

	// Although this function is unused it is actually needed to ensure that only modules that embed
	// DefaultsModuleBase will type-assert to the Defaults interface.
	isDefaults() bool

	// additionalDefaultableProperties returns additional properties provided by the defaults which
	// can themselves have defaults applied.
	additionalDefaultableProperties() []interface{}

	// protectedProperties returns the names of the properties whose values cannot be changed by a
	// module that applies these defaults.
	protectedProperties() []string

	// setProtectedProperties sets the names of the properties whose values cannot be changed by a
	// module that applies these defaults.
	setProtectedProperties(protectedProperties []string)

	// Get the structures containing the properties for which defaults can be provided.
	properties() []interface{}

	productVariableProperties() interface{}
}

func (d *DefaultsModuleBase) isDefaults() bool {
	return true
}

type DefaultsModule interface {
	Module
	Defaults
	Bazelable
}

func (d *DefaultsModuleBase) additionalDefaultableProperties() []interface{} {
	return []interface{}{&d.defaultsProperties}
}

func (d *DefaultsModuleBase) protectedProperties() []string {
	return d.defaultsProperties.Protected_properties
}

func (d *DefaultsModuleBase) setProtectedProperties(protectedProperties []string) {
	d.defaultsProperties.Protected_properties = protectedProperties
}

func (d *DefaultsModuleBase) properties() []interface{} {
	return d.defaultableProperties
}

func (d *DefaultsModuleBase) productVariableProperties() interface{} {
	return d.defaultableVariableProperties
}

func (d *DefaultsModuleBase) GenerateAndroidBuildActions(ctx ModuleContext) {}

// ConvertWithBp2build to fulfill Bazelable interface; however, at this time defaults module are
// *NOT* converted with bp2build
func (defaultable *DefaultsModuleBase) ConvertWithBp2build(ctx TopDownMutatorContext) {}

func InitDefaultsModule(module DefaultsModule) {
	commonProperties := &commonProperties{}

	module.AddProperties(
		&hostAndDeviceProperties{},
		commonProperties,
		&ApexProperties{},
		&distProperties{})

	// Additional properties of defaults modules that can themselves have
	// defaults applied.
	module.AddProperties(module.additionalDefaultableProperties()...)

	// Bazel module must be initialized _before_ Defaults to be included in cc_defaults module.
	InitBazelModule(module)
	initAndroidModuleBase(module)
	initProductVariableModule(module)
	initArchModule(module)
	InitDefaultableModule(module)

	// Add properties that will not have defaults applied to them.
	base := module.base()
	defaultsVisibility := &DefaultsVisibilityProperties{}
	module.AddProperties(&base.nameProperties, defaultsVisibility)

	// Unlike non-defaults modules the visibility property is not stored in m.base().commonProperties.
	// Instead it is stored in a separate instance of commonProperties created above so clear the
	// existing list of properties.
	clearVisibilityProperties(module)

	// The defaults_visibility property controls the visibility of a defaults module so it must be
	// set as the primary property, which also adds it to the list.
	setPrimaryVisibilityProperty(module, "defaults_visibility", &defaultsVisibility.Defaults_visibility)

	// The visibility property needs to be checked (but not parsed) by the visibility module during
	// its checking phase and parsing phase so add it to the list as a normal property.
	AddVisibilityProperty(module, "visibility", &commonProperties.Visibility)

	// The applicable licenses property for defaults is 'licenses'.
	setPrimaryLicensesProperty(module, "licenses", &commonProperties.Licenses)

	AddLoadHook(module, func(ctx LoadHookContext) {

		protectedProperties := module.protectedProperties()
		if len(protectedProperties) == 0 {
			return
		}

		propertiesAvailable := map[string]struct{}{}
		propertiesSet := map[string]struct{}{}

		// A defaults tracker which will keep track of which properties have been set on this module.
		collector := func(defaults DefaultsModule, property string, dstValue interface{}, srcValue interface{}) bool {
			value := reflect.ValueOf(dstValue)
			propertiesAvailable[property] = struct{}{}
			if !value.IsZero() {
				propertiesSet[property] = struct{}{}
			}
			// Skip all the properties so that there are no changes to the defaults.
			return false
		}

		// Try and apply this module's defaults to itself, so that the properties can be collected but
		// skip all the properties so it doesn't actually do anything.
		module.applySingleDefaultsWithTracker(ctx, module, collector)

		if InList("*", protectedProperties) {
			if len(protectedProperties) != 1 {
				ctx.PropertyErrorf("protected_properties", `if specified then "*" must be the only property listed`)
				return
			}

			// Do not automatically protect the protected_properties property.
			delete(propertiesSet, "protected_properties")

			// Or the visibility property.
			delete(propertiesSet, "visibility")

			// Replace the "*" with the names of all the properties that have been set.
			protectedProperties = SortedStringKeys(propertiesSet)
			module.setProtectedProperties(protectedProperties)
		} else {
			for _, property := range protectedProperties {
				if _, ok := propertiesAvailable[property]; !ok {
					ctx.PropertyErrorf(property, "property is not supported by this module type %q",
						ctx.ModuleType())
				} else if _, ok := propertiesSet[property]; !ok {
					ctx.PropertyErrorf(property, "is not set; protected properties must be explicitly set")
				}
			}
		}
	})
}

var _ Defaults = (*DefaultsModuleBase)(nil)

// applyNamespacedVariableDefaults only runs in bp2build mode for
// defaultable/defaults modules. Its purpose is to merge namespaced product
// variable props from defaults deps, even if those defaults are custom module
// types created from soong_config_module_type, e.g. one that's wrapping a
// cc_defaults or java_defaults.
func applyNamespacedVariableDefaults(defaultDep Defaults, ctx TopDownMutatorContext) {
	var dep, b Bazelable

	dep, ok := defaultDep.(Bazelable)
	if !ok {
		if depMod, ok := defaultDep.(Module); ok {
			// Track that this dependency hasn't been converted to bp2build yet.
			ctx.AddUnconvertedBp2buildDep(depMod.Name())
			return
		} else {
			panic("Expected default dep to be a Module.")
		}
	}

	b, ok = ctx.Module().(Bazelable)
	if !ok {
		return
	}

	// namespacedVariableProps is a map from namespaces (e.g. acme, android,
	// vendor_foo) to a slice of soong_config_variable struct pointers,
	// containing properties for that particular module.
	src := dep.namespacedVariableProps()
	dst := b.namespacedVariableProps()
	if dst == nil {
		dst = make(namespacedVariableProperties)
	}

	// Propagate all soong_config_variable structs from the dep. We'll merge the
	// actual property values later in variable.go.
	for namespace := range src {
		if dst[namespace] == nil {
			dst[namespace] = []interface{}{}
		}
		for _, i := range src[namespace] {
			dst[namespace] = append(dst[namespace], i)
		}
	}

	b.setNamespacedVariableProps(dst)
}

// defaultValueInfo contains information about each default value that applies to a protected
// property.
type defaultValueInfo struct {
	// The DefaultsModule providing the value, which may be defined on that module or applied as a
	// default from other modules.
	module Module

	// The default value, as returned by getComparableValue
	defaultValue reflect.Value
}

// protectedPropertyInfo contains information about each property that has to be protected when
// applying defaults.
type protectedPropertyInfo struct {
	// True if the property was set on the module to which defaults are applied, this is an error.
	propertySet bool

	// The original value of the property on the module, as returned by getComparableValue.
	originalValue reflect.Value

	// A list of defaults for the property that are being applied.
	defaultValues []defaultValueInfo
}

// getComparableValue takes a reflect.Value that may be a pointer to another value and returns a
// reflect.Value to the underlying data or the original if was not a pointer or was nil. The
// returned values can then be compared for equality.
func getComparableValue(value reflect.Value) reflect.Value {
	if value.IsZero() {
		return value
	}
	for value.Kind() == reflect.Ptr {
		value = value.Elem()
	}
	return value
}

func (defaultable *DefaultableModuleBase) applyDefaults(ctx TopDownMutatorContext,
	defaultsList []DefaultsModule) {

	// Collate information on all the properties protected by each of the default modules applied
	// to this module.
	allProtectedProperties := map[string]*protectedPropertyInfo{}
	for _, defaults := range defaultsList {
		for _, property := range defaults.protectedProperties() {
			info := allProtectedProperties[property]
			if info == nil {
				info = &protectedPropertyInfo{}
				allProtectedProperties[property] = info
			}
		}
	}

	// If there are any protected properties then collate information about attempts to change them.
	var protectedPropertyInfoCollector defaultsTrackerFunc
	if len(allProtectedProperties) > 0 {
		protectedPropertyInfoCollector = func(defaults DefaultsModule, property string,
			dstValue interface{}, srcValue interface{}) bool {

			// If the property is not protected then return immediately.
			info := allProtectedProperties[property]
			if info == nil {
				return true
			}

			currentValue := reflect.ValueOf(dstValue)
			if info.defaultValues == nil {
				info.propertySet = !currentValue.IsZero()
				info.originalValue = getComparableValue(currentValue)
			}

			defaultValue := reflect.ValueOf(srcValue)
			if !defaultValue.IsZero() {
				info.defaultValues = append(info.defaultValues,
					defaultValueInfo{defaults, getComparableValue(defaultValue)})
			}

			return true
		}
	}

	for _, defaults := range defaultsList {
		if ctx.Config().runningAsBp2Build {
			applyNamespacedVariableDefaults(defaults, ctx)
		}

		defaultable.applySingleDefaultsWithTracker(ctx, defaults, protectedPropertyInfoCollector)
	}

	// Check the status of any protected properties.
	for property, info := range allProtectedProperties {
		if len(info.defaultValues) == 0 {
			// No defaults were applied to the protected properties. Possibly because this module type
			// does not support any of them.
			continue
		}

		// Check to make sure that there are no conflicts between the defaults.
		conflictingDefaults := false
		previousDefaultValue := reflect.ValueOf(false)
		for _, defaultInfo := range info.defaultValues {
			defaultValue := defaultInfo.defaultValue
			if previousDefaultValue.IsZero() {
				previousDefaultValue = defaultValue
			} else if !reflect.DeepEqual(previousDefaultValue.Interface(), defaultValue.Interface()) {
				conflictingDefaults = true
				break
			}
		}

		if conflictingDefaults {
			var buf bytes.Buffer
			for _, defaultInfo := range info.defaultValues {
				buf.WriteString(fmt.Sprintf("\n    defaults module %q provides value %#v",
					ctx.OtherModuleName(defaultInfo.module), defaultInfo.defaultValue))
			}
			result := buf.String()
			ctx.ModuleErrorf("has conflicting default values for protected property %q:%s", property, result)
			continue
		}

		// Now check to see whether there the current module tried to override/append to the defaults.
		if info.propertySet {
			originalValue := info.originalValue
			// Just compare against the first defaults.
			defaultValue := info.defaultValues[0].defaultValue
			defaults := info.defaultValues[0].module

			if originalValue.Kind() == reflect.Slice {
				ctx.ModuleErrorf("attempts to append %q to protected property %q's value of %q defined in module %q",
					originalValue,
					property,
					defaultValue,
					ctx.OtherModuleName(defaults))
			} else {
				same := reflect.DeepEqual(originalValue.Interface(), defaultValue.Interface())
				message := ""
				if same {
					message = fmt.Sprintf(" with a matching value (%#v) so this property can simply be removed.", originalValue)
				} else {
					message = fmt.Sprintf(" with a different value (override %#v with %#v) so removing the property may necessitate other changes.", defaultValue, originalValue)
				}
				ctx.ModuleErrorf("attempts to override protected property %q defined in module %q%s",
					property,
					ctx.OtherModuleName(defaults), message)
			}
		}
	}
}

func (defaultable *DefaultableModuleBase) applySingleDefaultsWithTracker(ctx EarlyModuleContext, defaults DefaultsModule, tracker defaultsTrackerFunc) {
	for _, prop := range defaultable.defaultableProperties {
		var err error
		if prop == defaultable.defaultableVariableProperties {
			err = defaultable.applyDefaultVariableProperties(defaults, prop, tracker)
		} else {
			err = defaultable.applyDefaultProperties(defaults, prop, tracker)
		}
		if err != nil {
			if propertyErr, ok := err.(*proptools.ExtendPropertyError); ok {
				ctx.PropertyErrorf(propertyErr.Property, "%s", propertyErr.Err.Error())
			} else {
				panic(err)
			}
		}
	}
}

// defaultsTrackerFunc is the type of a function that can be used to track how defaults are applied.
type defaultsTrackerFunc func(defaults DefaultsModule, property string,
	dstValue interface{}, srcValue interface{}) bool

// filterForTracker wraps a defaultsTrackerFunc in a proptools.ExtendPropertyFilterFunc
func filterForTracker(defaults DefaultsModule, tracker defaultsTrackerFunc) proptools.ExtendPropertyFilterFunc {
	if tracker == nil {
		return nil
	}
	return func(property string,
		dstField, srcField reflect.StructField,
		dstValue, srcValue interface{}) (bool, error) {

		apply := tracker(defaults, property, dstValue, srcValue)
		return apply, nil
	}
}

// Product variable properties need special handling, the type of the filtered product variable
// property struct may not be identical between the defaults module and the defaultable module.
// Use PrependMatchingProperties to apply whichever properties match.
func (defaultable *DefaultableModuleBase) applyDefaultVariableProperties(defaults DefaultsModule,
	defaultableProp interface{}, tracker defaultsTrackerFunc) error {
	if defaultableProp == nil {
		return nil
	}

	defaultsProp := defaults.productVariableProperties()
	if defaultsProp == nil {
		return nil
	}

	dst := []interface{}{
		defaultableProp,
		// Put an empty copy of the src properties into dst so that properties in src that are not in dst
		// don't cause a "failed to find property to extend" error.
		proptools.CloneEmptyProperties(reflect.ValueOf(defaultsProp)).Interface(),
	}

	filter := filterForTracker(defaults, tracker)

	return proptools.PrependMatchingProperties(dst, defaultsProp, filter)
}

func (defaultable *DefaultableModuleBase) applyDefaultProperties(defaults DefaultsModule,
	defaultableProp interface{}, checker defaultsTrackerFunc) error {

	filter := filterForTracker(defaults, checker)

	for _, def := range defaults.properties() {
		if proptools.TypeEqual(defaultableProp, def) {
			err := proptools.PrependProperties(defaultableProp, def, filter)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func RegisterDefaultsPreArchMutators(ctx RegisterMutatorsContext) {
	ctx.BottomUp("defaults_deps", defaultsDepsMutator).Parallel()
	ctx.TopDown("defaults", defaultsMutator).Parallel()
}

func defaultsDepsMutator(ctx BottomUpMutatorContext) {
	if defaultable, ok := ctx.Module().(Defaultable); ok {
		ctx.AddDependency(ctx.Module(), DefaultsDepTag, defaultable.defaults().Defaults...)
	}
}

func defaultsMutator(ctx TopDownMutatorContext) {
	if defaultable, ok := ctx.Module().(Defaultable); ok {
		if len(defaultable.defaults().Defaults) > 0 {
			var defaultsList []DefaultsModule
			seen := make(map[Defaults]bool)

			ctx.WalkDeps(func(module, parent Module) bool {
				if ctx.OtherModuleDependencyTag(module) == DefaultsDepTag {
					if defaults, ok := module.(DefaultsModule); ok {
						if !seen[defaults] {
							seen[defaults] = true
							defaultsList = append(defaultsList, defaults)
							return len(defaults.defaults().Defaults) > 0
						}
					} else {
						ctx.PropertyErrorf("defaults", "module %s is not an defaults module",
							ctx.OtherModuleName(module))
					}
				}
				return false
			})
			defaultable.applyDefaults(ctx, defaultsList)
		}

		defaultable.callHookIfAvailable(ctx)
	}
}
