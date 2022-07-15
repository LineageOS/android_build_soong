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
	"fmt"
	"reflect"
	"strings"
)

// Supports customizing sdk snapshot output based on target build release.

// buildRelease represents the version of a build system used to create a specific release.
//
// The name of the release, is the same as the code for the dessert release, e.g. S, Tiramisu, etc.
type buildRelease struct {
	// The name of the release, e.g. S, Tiramisu, etc.
	name string

	// The index of this structure within the buildReleases list.
	ordinal int
}

func (br *buildRelease) EarlierThan(other *buildRelease) bool {
	return br.ordinal < other.ordinal
}

// String returns the name of the build release.
func (br *buildRelease) String() string {
	return br.name
}

// buildReleaseSet represents a set of buildRelease objects.
type buildReleaseSet struct {
	// Set of *buildRelease represented as a map from *buildRelease to struct{}.
	contents map[*buildRelease]struct{}
}

// addItem adds a build release to the set.
func (s *buildReleaseSet) addItem(release *buildRelease) {
	s.contents[release] = struct{}{}
}

// addRange adds all the build releases from start (inclusive) to end (inclusive).
func (s *buildReleaseSet) addRange(start *buildRelease, end *buildRelease) {
	for i := start.ordinal; i <= end.ordinal; i += 1 {
		s.addItem(buildReleases[i])
	}
}

// contains returns true if the set contains the specified build release.
func (s *buildReleaseSet) contains(release *buildRelease) bool {
	_, ok := s.contents[release]
	return ok
}

// String returns a string representation of the set, sorted from earliest to latest release.
func (s *buildReleaseSet) String() string {
	list := []string{}
	for _, release := range buildReleases {
		if _, ok := s.contents[release]; ok {
			list = append(list, release.name)
		}
	}
	return fmt.Sprintf("[%s]", strings.Join(list, ","))
}

var (
	// nameToBuildRelease contains a map from name to build release.
	nameToBuildRelease = map[string]*buildRelease{}

	// buildReleases lists all the available build releases.
	buildReleases = []*buildRelease{}

	// allBuildReleaseSet is the set of all build releases.
	allBuildReleaseSet = &buildReleaseSet{contents: map[*buildRelease]struct{}{}}

	// Add the build releases from oldest to newest.
	buildReleaseS = initBuildRelease("S")
	buildReleaseT = initBuildRelease("Tiramisu")
)

// initBuildRelease creates a new build release with the specified name.
func initBuildRelease(name string) *buildRelease {
	ordinal := len(nameToBuildRelease)
	release := &buildRelease{name: name, ordinal: ordinal}
	nameToBuildRelease[name] = release
	buildReleases = append(buildReleases, release)
	allBuildReleaseSet.addItem(release)
	return release
}

// latestBuildRelease returns the latest build release, i.e. the last one added.
func latestBuildRelease() *buildRelease {
	return buildReleases[len(buildReleases)-1]
}

// nameToRelease maps from build release name to the corresponding build release (if it exists) or
// the error if it does not.
func nameToRelease(name string) (*buildRelease, error) {
	if r, ok := nameToBuildRelease[name]; ok {
		return r, nil
	}

	return nil, fmt.Errorf("unknown release %q, expected one of %s", name, allBuildReleaseSet)
}

// parseBuildReleaseSet parses a build release set string specification into a build release set.
//
// The specification consists of one of the following:
// * a single build release name, e.g. S, T, etc.
// * a closed range (inclusive to inclusive), e.g. S-T
// * an open range, e.g. T+.
//
// This returns the set if the specification was valid or an error.
func parseBuildReleaseSet(specification string) (*buildReleaseSet, error) {
	set := &buildReleaseSet{contents: map[*buildRelease]struct{}{}}

	if strings.HasSuffix(specification, "+") {
		rangeStart := strings.TrimSuffix(specification, "+")
		start, err := nameToRelease(rangeStart)
		if err != nil {
			return nil, err
		}
		end := latestBuildRelease()
		set.addRange(start, end)
	} else if strings.Contains(specification, "-") {
		limits := strings.SplitN(specification, "-", 2)
		start, err := nameToRelease(limits[0])
		if err != nil {
			return nil, err
		}

		end, err := nameToRelease(limits[1])
		if err != nil {
			return nil, err
		}

		if start.ordinal > end.ordinal {
			return nil, fmt.Errorf("invalid closed range, start release %q is later than end release %q", start.name, end.name)
		}

		set.addRange(start, end)
	} else {
		release, err := nameToRelease(specification)
		if err != nil {
			return nil, err
		}
		set.addItem(release)
	}

	return set, nil
}

// Given a set of properties (struct value), set the value of a field within that struct (or one of
// its embedded structs) to its zero value.
type fieldPrunerFunc func(structValue reflect.Value)

// A property that can be cleared by a propertyPruner.
type prunerProperty struct {
	// The name of the field for this property. It is a "."-separated path for fields in non-anonymous
	// sub-structs.
	name string

	// Sets the associated field to its zero value.
	prunerFunc fieldPrunerFunc
}

// propertyPruner provides support for pruning (i.e. setting to their zero value) properties from
// a properties structure.
type propertyPruner struct {
	// The properties that the pruner will clear.
	properties []prunerProperty
}

// gatherFields recursively processes the supplied structure and a nested structures, selecting the
// fields that require pruning and populates the propertyPruner.properties with the information
// needed to prune those fields.
//
// containingStructAccessor is a func that if given an object will return a field whose value is
// of the supplied structType. It is nil on initial entry to this method but when this method is
// called recursively on a field that is a nested structure containingStructAccessor is set to a
// func that provides access to the field's value.
//
// namePrefix is the prefix to the fields that are being visited. It is "" on initial entry to this
// method but when this method is called recursively on a field that is a nested structure
// namePrefix is the result of appending the field name (plus a ".") to the previous name prefix.
// Unless the field is anonymous in which case it is passed through unchanged.
//
// selector is a func that will select whether the supplied field requires pruning or not. If it
// returns true then the field will be added to those to be pruned, otherwise it will not.
func (p *propertyPruner) gatherFields(structType reflect.Type, containingStructAccessor fieldAccessorFunc, namePrefix string, selector fieldSelectorFunc) {
	for f := 0; f < structType.NumField(); f++ {
		field := structType.Field(f)
		if field.PkgPath != "" {
			// Ignore unexported fields.
			continue
		}

		// Save a copy of the field index for use in the function.
		fieldIndex := f

		name := namePrefix + field.Name

		fieldGetter := func(container reflect.Value) reflect.Value {
			if containingStructAccessor != nil {
				// This is an embedded structure so first access the field for the embedded
				// structure.
				container = containingStructAccessor(container)
			}

			// Skip through interface and pointer values to find the structure.
			container = getStructValue(container)

			defer func() {
				if r := recover(); r != nil {
					panic(fmt.Errorf("%s for fieldIndex %d of field %s of container %#v", r, fieldIndex, name, container.Interface()))
				}
			}()

			// Return the field.
			return container.Field(fieldIndex)
		}

		fieldType := field.Type
		if selector(name, field) {
			zeroValue := reflect.Zero(fieldType)
			fieldPruner := func(container reflect.Value) {
				if containingStructAccessor != nil {
					// This is an embedded structure so first access the field for the embedded
					// structure.
					container = containingStructAccessor(container)
				}

				// Skip through interface and pointer values to find the structure.
				container = getStructValue(container)

				defer func() {
					if r := recover(); r != nil {
						panic(fmt.Errorf("%s\n\tfor field (index %d, name %s)", r, fieldIndex, name))
					}
				}()

				// Set the field.
				container.Field(fieldIndex).Set(zeroValue)
			}

			property := prunerProperty{
				name,
				fieldPruner,
			}
			p.properties = append(p.properties, property)
		} else {
			switch fieldType.Kind() {
			case reflect.Struct:
				// Gather fields from the nested or embedded structure.
				var subNamePrefix string
				if field.Anonymous {
					subNamePrefix = namePrefix
				} else {
					subNamePrefix = name + "."
				}
				p.gatherFields(fieldType, fieldGetter, subNamePrefix, selector)

			case reflect.Map:
				// Get the type of the values stored in the map.
				valueType := fieldType.Elem()
				// Skip over * types.
				if valueType.Kind() == reflect.Ptr {
					valueType = valueType.Elem()
				}
				if valueType.Kind() == reflect.Struct {
					// If this is not referenced by a pointer then it is an error as it is impossible to
					// modify a struct that is stored directly as a value in a map.
					if fieldType.Elem().Kind() != reflect.Ptr {
						panic(fmt.Errorf("Cannot prune struct %s stored by value in map %s, map values must"+
							" be pointers to structs",
							fieldType.Elem(), name))
					}

					// Create a new pruner for the values of the map.
					valuePruner := newPropertyPrunerForStructType(valueType, selector)

					// Create a new fieldPruner that will iterate over all the items in the map and call the
					// pruner on them.
					fieldPruner := func(container reflect.Value) {
						mapValue := fieldGetter(container)

						for _, keyValue := range mapValue.MapKeys() {
							itemValue := mapValue.MapIndex(keyValue)

							defer func() {
								if r := recover(); r != nil {
									panic(fmt.Errorf("%s\n\tfor key %q", r, keyValue))
								}
							}()

							valuePruner.pruneProperties(itemValue.Interface())
						}
					}

					// Add the map field pruner to the list of property pruners.
					property := prunerProperty{
						name + "[*]",
						fieldPruner,
					}
					p.properties = append(p.properties, property)
				}
			}
		}
	}
}

// pruneProperties will prune (set to zero value) any properties in the struct referenced by the
// supplied struct pointer.
//
// The struct must be of the same type as was originally passed to newPropertyPruner to create this
// propertyPruner.
func (p *propertyPruner) pruneProperties(propertiesStruct interface{}) {

	defer func() {
		if r := recover(); r != nil {
			panic(fmt.Errorf("%s\n\tof container %#v", r, propertiesStruct))
		}
	}()

	structValue := reflect.ValueOf(propertiesStruct)
	for _, property := range p.properties {
		property.prunerFunc(structValue)
	}
}

// fieldSelectorFunc is called to select whether a specific field should be pruned or not.
// name is the name of the field, including any prefixes from containing str
type fieldSelectorFunc func(name string, field reflect.StructField) bool

// newPropertyPruner creates a new property pruner for the structure type for the supplied
// properties struct.
//
// The returned pruner can be used on any properties structure of the same type as the supplied set
// of properties.
func newPropertyPruner(propertiesStruct interface{}, selector fieldSelectorFunc) *propertyPruner {
	structType := getStructValue(reflect.ValueOf(propertiesStruct)).Type()
	return newPropertyPrunerForStructType(structType, selector)
}

// newPropertyPruner creates a new property pruner for the supplied properties struct type.
//
// The returned pruner can be used on any properties structure of the supplied type.
func newPropertyPrunerForStructType(structType reflect.Type, selector fieldSelectorFunc) *propertyPruner {
	pruner := &propertyPruner{}
	pruner.gatherFields(structType, nil, "", selector)
	return pruner
}

// newPropertyPrunerByBuildRelease creates a property pruner that will clear any properties in the
// structure which are not supported by the specified target build release.
//
// A property is pruned if its field has a tag of the form:
//     `supported_build_releases:"<build-release-set>"`
// and the resulting build release set does not contain the target build release. Properties that
// have no such tag are assumed to be supported by all releases.
func newPropertyPrunerByBuildRelease(propertiesStruct interface{}, targetBuildRelease *buildRelease) *propertyPruner {
	return newPropertyPruner(propertiesStruct, func(name string, field reflect.StructField) bool {
		if supportedBuildReleases, ok := field.Tag.Lookup("supported_build_releases"); ok {
			set, err := parseBuildReleaseSet(supportedBuildReleases)
			if err != nil {
				panic(fmt.Errorf("invalid `supported_build_releases` tag on %s of %T: %s", name, propertiesStruct, err))
			}

			// If the field does not support tha target release then prune it.
			return !set.contains(targetBuildRelease)

		} else {
			// Any untagged fields are assumed to be supported by all build releases so should never be
			// pruned.
			return false
		}
	})
}
