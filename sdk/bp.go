// Copyright (C) 2019 The Android Open Source Project
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

	"android/soong/android"
)

type bpPropertySet struct {
	properties map[string]interface{}
	tags       map[string]android.BpPropertyTag
	order      []string
}

var _ android.BpPropertySet = (*bpPropertySet)(nil)

func (s *bpPropertySet) init() {
	s.properties = make(map[string]interface{})
	s.tags = make(map[string]android.BpPropertyTag)
}

func (s *bpPropertySet) AddProperty(name string, value interface{}) {
	if s.properties[name] != nil {
		panic(fmt.Sprintf("Property %q already exists in property set", name))
	}

	s.properties[name] = value
	s.order = append(s.order, name)
}

func (s *bpPropertySet) AddPropertyWithTag(name string, value interface{}, tag android.BpPropertyTag) {
	s.AddProperty(name, value)
	s.tags[name] = tag
}

func (s *bpPropertySet) AddPropertySet(name string) android.BpPropertySet {
	set := newPropertySet()
	s.AddProperty(name, set)
	return set
}

func (s *bpPropertySet) getValue(name string) interface{} {
	return s.properties[name]
}

func (s *bpPropertySet) getTag(name string) interface{} {
	return s.tags[name]
}

func (s *bpPropertySet) transformContents(transformer bpPropertyTransformer) {
	var newOrder []string
	for _, name := range s.order {
		value := s.properties[name]
		tag := s.tags[name]
		var newValue interface{}
		var newTag android.BpPropertyTag
		if propertySet, ok := value.(*bpPropertySet); ok {
			var newPropertySet *bpPropertySet
			newPropertySet, newTag = transformPropertySet(transformer, name, propertySet, tag)
			if newPropertySet == nil {
				newValue = nil
			} else {
				newValue = newPropertySet
			}
		} else {
			newValue, newTag = transformer.transformProperty(name, value, tag)
		}

		if newValue == nil {
			// Delete the property from the map and exclude it from the new order.
			delete(s.properties, name)
		} else {
			// Update the property in the map and add the name to the new order list.
			s.properties[name] = newValue
			s.tags[name] = newTag
			newOrder = append(newOrder, name)
		}
	}
	s.order = newOrder
}

func transformPropertySet(transformer bpPropertyTransformer, name string, propertySet *bpPropertySet, tag android.BpPropertyTag) (*bpPropertySet, android.BpPropertyTag) {
	newPropertySet, newTag := transformer.transformPropertySetBeforeContents(name, propertySet, tag)
	if newPropertySet != nil {
		newPropertySet.transformContents(transformer)

		newPropertySet, newTag = transformer.transformPropertySetAfterContents(name, newPropertySet, newTag)
	}
	return newPropertySet, newTag
}

func (s *bpPropertySet) setProperty(name string, value interface{}) {
	if s.properties[name] == nil {
		s.AddProperty(name, value)
	} else {
		s.properties[name] = value
		s.tags[name] = nil
	}
}

func (s *bpPropertySet) insertAfter(position string, name string, value interface{}) {
	if s.properties[name] != nil {
		panic("Property %q already exists in property set")
	}

	// Add the name to the end of the order, to ensure it has necessary capacity
	// and to handle the case when the position does not exist.
	s.order = append(s.order, name)

	// Search through the order for the item that matches supplied position. If
	// found then insert the name of the new property after it.
	for i, v := range s.order {
		if v == position {
			// Copy the items after the one where the new property should be inserted.
			copy(s.order[i+2:], s.order[i+1:])
			// Insert the item in the list.
			s.order[i+1] = name
		}
	}

	s.properties[name] = value
}

type bpModule struct {
	*bpPropertySet
	moduleType string
}

var _ android.BpModule = (*bpModule)(nil)

type bpPropertyTransformer interface {
	// Transform the property set, returning the new property set/tag to insert back into the
	// parent property set (or module if this is the top level property set).
	//
	// This will be called before transforming the properties in the supplied set.
	//
	// The name will be "" for the top level property set.
	//
	// Returning (nil, ...) will cause the property set to be removed.
	transformPropertySetBeforeContents(name string, propertySet *bpPropertySet, tag android.BpPropertyTag) (*bpPropertySet, android.BpPropertyTag)

	// Transform the property set, returning the new property set/tag to insert back into the
	// parent property set (or module if this is the top level property set).
	//
	// This will be called after transforming the properties in the supplied set.
	//
	// The name will be "" for the top level property set.
	//
	// Returning (nil, ...) will cause the property set to be removed.
	transformPropertySetAfterContents(name string, propertySet *bpPropertySet, tag android.BpPropertyTag) (*bpPropertySet, android.BpPropertyTag)

	// Transform a property, return the new value/tag to insert back into the property set.
	//
	// Returning (nil, ...) will cause the property to be removed.
	transformProperty(name string, value interface{}, tag android.BpPropertyTag) (interface{}, android.BpPropertyTag)
}

// Interface for transforming bpModule objects.
type bpTransformer interface {
	// Transform the module, returning the result.
	//
	// The method can either create a new module and return that, or modify the supplied module
	// in place and return that.
	//
	// After this returns the transformer is applied to the contents of the returned module.
	transformModule(module *bpModule) *bpModule

	bpPropertyTransformer
}

type identityTransformation struct{}

var _ bpTransformer = (*identityTransformation)(nil)

func (t identityTransformation) transformModule(module *bpModule) *bpModule {
	return module
}

func (t identityTransformation) transformPropertySetBeforeContents(name string, propertySet *bpPropertySet, tag android.BpPropertyTag) (*bpPropertySet, android.BpPropertyTag) {
	return propertySet, tag
}

func (t identityTransformation) transformPropertySetAfterContents(name string, propertySet *bpPropertySet, tag android.BpPropertyTag) (*bpPropertySet, android.BpPropertyTag) {
	return propertySet, tag
}

func (t identityTransformation) transformProperty(name string, value interface{}, tag android.BpPropertyTag) (interface{}, android.BpPropertyTag) {
	return value, tag
}

func (m *bpModule) deepCopy() *bpModule {
	return m.transform(deepCopyTransformer)
}

func (m *bpModule) transform(transformer bpTransformer) *bpModule {
	transformedModule := transformer.transformModule(m)
	// Copy the contents of the returned property set into the module and then transform that.
	transformedModule.bpPropertySet, _ = transformPropertySet(transformer, "", transformedModule.bpPropertySet, nil)
	return transformedModule
}

type deepCopyTransformation struct {
	identityTransformation
}

func (t deepCopyTransformation) transformModule(module *bpModule) *bpModule {
	// Take a shallow copy of the module. Any mutable property values will be copied by the
	// transformer.
	moduleCopy := *module
	return &moduleCopy
}

func (t deepCopyTransformation) transformPropertySetBeforeContents(name string, propertySet *bpPropertySet, tag android.BpPropertyTag) (*bpPropertySet, android.BpPropertyTag) {
	// Create a shallow copy of the properties map. Any mutable property values will be copied by the
	// transformer.
	propertiesCopy := make(map[string]interface{})
	for propertyName, value := range propertySet.properties {
		propertiesCopy[propertyName] = value
	}

	// Ditto for tags map.
	tagsCopy := make(map[string]android.BpPropertyTag)
	for propertyName, propertyTag := range propertySet.tags {
		tagsCopy[propertyName] = propertyTag
	}

	// Create a new property set.
	return &bpPropertySet{
		properties: propertiesCopy,
		tags:       tagsCopy,
		order:      append([]string(nil), propertySet.order...),
	}, tag
}

func (t deepCopyTransformation) transformProperty(name string, value interface{}, tag android.BpPropertyTag) (interface{}, android.BpPropertyTag) {
	// Copy string slice, otherwise return value.
	if values, ok := value.([]string); ok {
		valuesCopy := make([]string, len(values))
		copy(valuesCopy, values)
		return valuesCopy, tag
	}
	return value, tag
}

var deepCopyTransformer bpTransformer = deepCopyTransformation{}

// A .bp file
type bpFile struct {
	modules map[string]*bpModule
	order   []*bpModule
}

// Add a module.
//
// The module must have had its "name" property set to a string value that
// is unique within this file.
func (f *bpFile) AddModule(module android.BpModule) {
	m := module.(*bpModule)
	if name, ok := m.getValue("name").(string); ok {
		if f.modules[name] != nil {
			panic(fmt.Sprintf("Module %q already exists in bp file", name))
		}

		f.modules[name] = m
		f.order = append(f.order, m)
	} else {
		panic("Module does not have a name property, or it is not a string")
	}
}

func (f *bpFile) newModule(moduleType string) *bpModule {
	return newModule(moduleType)
}

func newModule(moduleType string) *bpModule {
	module := &bpModule{
		moduleType:    moduleType,
		bpPropertySet: newPropertySet(),
	}
	return module
}

func newPropertySet() *bpPropertySet {
	set := &bpPropertySet{}
	set.init()
	return set
}
