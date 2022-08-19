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

// Contains information about the sdk properties that list sdk members by type, e.g.
// Java_header_libs.
type sdkMemberTypeListProperty struct {
	// getter for the list of member names
	getter func(properties interface{}) []string

	// setter for the list of member names
	setter func(properties interface{}, list []string)

	// the type of member referenced in the list
	memberType android.SdkMemberType

	// the dependency tag used for items in this list that can be used to determine the memberType
	// for a resolved dependency.
	dependencyTag android.SdkMemberDependencyTag
}

func (p *sdkMemberTypeListProperty) propertyName() string {
	return p.memberType.SdkPropertyName()
}

// Cache of dynamically generated dynamicSdkMemberTypes objects. The key is the pointer
// to a slice of SdkMemberType instances held in android.SdkMemberTypes.
var dynamicSdkMemberTypesMap android.OncePer

// A dynamically generated set of member list properties and associated structure type.
type dynamicSdkMemberTypes struct {
	// The dynamically generated structure type.
	//
	// Contains one []string exported field for each android.SdkMemberTypes. The name of the field
	// is the exported form of the value returned by SdkMemberType.SdkPropertyName().
	propertiesStructType reflect.Type

	// Information about each of the member type specific list properties.
	memberTypeListProperties []*sdkMemberTypeListProperty

	memberTypeToProperty map[android.SdkMemberType]*sdkMemberTypeListProperty
}

func (d *dynamicSdkMemberTypes) createMemberTypeListProperties() interface{} {
	return reflect.New(d.propertiesStructType).Interface()
}

func getDynamicSdkMemberTypes(key android.OnceKey, registeredTypes []android.SdkMemberType) *dynamicSdkMemberTypes {
	// Get the cached value, creating new instance if necessary.
	return dynamicSdkMemberTypesMap.Once(key, func() interface{} {
		return createDynamicSdkMemberTypes(registeredTypes)
	}).(*dynamicSdkMemberTypes)
}

// Create the dynamicSdkMemberTypes from the list of registered member types.
//
// A struct is created which contains one exported field per member type corresponding to
// the SdkMemberType.SdkPropertyName() value.
//
// A list of sdkMemberTypeListProperty instances is created, one per member type that provides:
// * a reference to the member type.
// * a getter for the corresponding field in the properties struct.
// * a dependency tag that identifies the member type of a resolved dependency.
func createDynamicSdkMemberTypes(sdkMemberTypes []android.SdkMemberType) *dynamicSdkMemberTypes {

	var listProperties []*sdkMemberTypeListProperty
	memberTypeToProperty := map[android.SdkMemberType]*sdkMemberTypeListProperty{}
	var fields []reflect.StructField

	// Iterate over the member types creating StructField and sdkMemberTypeListProperty objects.
	nextFieldIndex := 0
	for _, memberType := range sdkMemberTypes {

		p := memberType.SdkPropertyName()

		var getter func(properties interface{}) []string
		var setter func(properties interface{}, list []string)
		if memberType.RequiresBpProperty() {
			// Create a dynamic exported field for the member type's property.
			fields = append(fields, reflect.StructField{
				Name: proptools.FieldNameForProperty(p),
				Type: reflect.TypeOf([]string{}),
				Tag:  `android:"arch_variant"`,
			})

			// Copy the field index for use in the getter func as using the loop variable directly will
			// cause all funcs to use the last value.
			fieldIndex := nextFieldIndex
			nextFieldIndex += 1

			getter = func(properties interface{}) []string {
				// The properties is expected to be of the following form (where
				// <Module_types> is the name of an SdkMemberType.SdkPropertyName().
				//     properties *struct {<Module_types> []string, ....}
				//
				// Although it accesses the field by index the following reflection code is equivalent to:
				//    *properties.<Module_types>
				//
				list := reflect.ValueOf(properties).Elem().Field(fieldIndex).Interface().([]string)
				return list
			}

			setter = func(properties interface{}, list []string) {
				// The properties is expected to be of the following form (where
				// <Module_types> is the name of an SdkMemberType.SdkPropertyName().
				//     properties *struct {<Module_types> []string, ....}
				//
				// Although it accesses the field by index the following reflection code is equivalent to:
				//    *properties.<Module_types> = list
				//
				reflect.ValueOf(properties).Elem().Field(fieldIndex).Set(reflect.ValueOf(list))
			}
		}

		// Create an sdkMemberTypeListProperty for the member type.
		memberListProperty := &sdkMemberTypeListProperty{
			getter:     getter,
			setter:     setter,
			memberType: memberType,

			// Dependencies added directly from member properties are always exported.
			dependencyTag: android.DependencyTagForSdkMemberType(memberType, true),
		}

		memberTypeToProperty[memberType] = memberListProperty
		listProperties = append(listProperties, memberListProperty)
	}

	// Create a dynamic struct from the collated fields.
	propertiesStructType := reflect.StructOf(fields)

	return &dynamicSdkMemberTypes{
		memberTypeListProperties: listProperties,
		memberTypeToProperty:     memberTypeToProperty,
		propertiesStructType:     propertiesStructType,
	}
}
