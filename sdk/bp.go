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
	order      []string
}

var _ android.BpPropertySet = (*bpPropertySet)(nil)

func (s *bpPropertySet) init() {
	s.properties = make(map[string]interface{})
}

func (s *bpPropertySet) AddProperty(name string, value interface{}) {
	if s.properties[name] != nil {
		panic("Property %q already exists in property set")
	}

	s.properties[name] = value
	s.order = append(s.order, name)
}

func (s *bpPropertySet) AddPropertySet(name string) android.BpPropertySet {
	set := &bpPropertySet{}
	set.init()
	s.AddProperty(name, set)
	return set
}

func (s *bpPropertySet) getValue(name string) interface{} {
	return s.properties[name]
}

func (s *bpPropertySet) copy() bpPropertySet {
	propertiesCopy := make(map[string]interface{})
	for p, v := range s.properties {
		propertiesCopy[p] = v
	}

	return bpPropertySet{
		properties: propertiesCopy,
		order:      append([]string(nil), s.order...),
	}
}

func (s *bpPropertySet) setProperty(name string, value interface{}) {
	if s.properties[name] == nil {
		s.AddProperty(name, value)
	} else {
		s.properties[name] = value
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
	bpPropertySet
	moduleType string
}

var _ android.BpModule = (*bpModule)(nil)

func (m *bpModule) copy() *bpModule {
	return &bpModule{
		bpPropertySet: m.bpPropertySet.copy(),
		moduleType:    m.moduleType,
	}
}

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
	module := &bpModule{
		moduleType: moduleType,
	}
	(&module.bpPropertySet).init()
	return module
}
