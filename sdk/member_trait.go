// Copyright (C) 2021 The Android Open Source Project
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

package sdk

import (
	"reflect"

	"android/soong/android"
	"github.com/google/blueprint/proptools"
)

// Contains information about the sdk properties that list sdk members by trait, e.g.
// native_bridge.
type sdkMemberTraitListProperty struct {
	// getter for the list of member names
	getter func(properties interface{}) []string

	// the trait of member referenced in the list
	memberTrait android.SdkMemberTrait
}

// Cache of dynamically generated dynamicSdkMemberTraits objects. The key is the pointer
// to a slice of SdkMemberTrait instances returned by android.RegisteredSdkMemberTraits().
var dynamicSdkMemberTraitsMap android.OncePer

// A dynamically generated set of member list properties and associated structure type.
//
// Instances of this are created by createDynamicSdkMemberTraits.
type dynamicSdkMemberTraits struct {
	// The dynamically generated structure type.
	//
	// Contains one []string exported field for each SdkMemberTrait returned by android.RegisteredSdkMemberTraits(). The name of
	// the field is the exported form of the value returned by SdkMemberTrait.SdkPropertyName().
	propertiesStructType reflect.Type

	// Information about each of the member trait specific list properties.
	memberTraitListProperties []*sdkMemberTraitListProperty
}

func (d *dynamicSdkMemberTraits) createMemberTraitListProperties() interface{} {
	return reflect.New(d.propertiesStructType).Interface()
}

func getDynamicSdkMemberTraits(key android.OnceKey, registeredTraits []android.SdkMemberTrait) *dynamicSdkMemberTraits {
	// Get the cached value, creating new instance if necessary.
	return dynamicSdkMemberTraitsMap.Once(key, func() interface{} {
		return createDynamicSdkMemberTraits(registeredTraits)
	}).(*dynamicSdkMemberTraits)
}

// Create the dynamicSdkMemberTraits from the list of registered member traits.
//
// A struct is created which contains one exported field per member trait corresponding to
// the SdkMemberTrait.SdkPropertyName() value.
//
// A list of sdkMemberTraitListProperty instances is created, one per member trait that provides:
// * a reference to the member trait.
// * a getter for the corresponding field in the properties struct.
func createDynamicSdkMemberTraits(sdkMemberTraits []android.SdkMemberTrait) *dynamicSdkMemberTraits {

	var listProperties []*sdkMemberTraitListProperty
	memberTraitToProperty := map[android.SdkMemberTrait]*sdkMemberTraitListProperty{}
	var fields []reflect.StructField

	// Iterate over the member traits creating StructField and sdkMemberTraitListProperty objects.
	nextFieldIndex := 0
	for _, memberTrait := range sdkMemberTraits {

		p := memberTrait.SdkPropertyName()

		var getter func(properties interface{}) []string

		// Create a dynamic exported field for the member trait's property.
		fields = append(fields, reflect.StructField{
			Name: proptools.FieldNameForProperty(p),
			Type: reflect.TypeOf([]string{}),
		})

		// Copy the field index for use in the getter func as using the loop variable directly will
		// cause all funcs to use the last value.
		fieldIndex := nextFieldIndex
		nextFieldIndex += 1

		getter = func(properties interface{}) []string {
			// The properties is expected to be of the following form (where
			// <Module_traits> is the name of an SdkMemberTrait.SdkPropertyName().
			//     properties *struct {<Module_traits> []string, ....}
			//
			// Although it accesses the field by index the following reflection code is equivalent to:
			//    *properties.<Module_traits>
			//
			list := reflect.ValueOf(properties).Elem().Field(fieldIndex).Interface().([]string)
			return list
		}

		// Create an sdkMemberTraitListProperty for the member trait.
		memberListProperty := &sdkMemberTraitListProperty{
			getter:      getter,
			memberTrait: memberTrait,
		}

		memberTraitToProperty[memberTrait] = memberListProperty
		listProperties = append(listProperties, memberListProperty)
	}

	// Create a dynamic struct from the collated fields.
	propertiesStructType := reflect.StructOf(fields)

	return &dynamicSdkMemberTraits{
		memberTraitListProperties: listProperties,
		propertiesStructType:      propertiesStructType,
	}
}
