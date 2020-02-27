// Copyright (C) 2020 The Android Open Source Project
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
	"testing"

	"android/soong/android"
)

type removeFredTransformation struct {
	identityTransformation
}

func (t removeFredTransformation) transformProperty(name string, value interface{}, tag android.BpPropertyTag) (interface{}, android.BpPropertyTag) {
	if name == "fred" {
		return nil, nil
	}
	return value, tag
}

func (t removeFredTransformation) transformPropertySetBeforeContents(name string, propertySet *bpPropertySet, tag android.BpPropertyTag) (*bpPropertySet, android.BpPropertyTag) {
	if name == "fred" {
		return nil, nil
	}
	return propertySet, tag
}

func (t removeFredTransformation) transformPropertySetAfterContents(name string, propertySet *bpPropertySet, tag android.BpPropertyTag) (*bpPropertySet, android.BpPropertyTag) {
	if len(propertySet.properties) == 0 {
		return nil, nil
	}
	return propertySet, tag
}

func TestTransformRemoveProperty(t *testing.T) {

	helper := &TestHelper{t}

	set := newPropertySet()
	set.AddProperty("name", "name")
	set.AddProperty("fred", "12")

	set.transformContents(removeFredTransformation{})

	contents := &generatedContents{}
	outputPropertySet(contents, set)
	helper.AssertTrimmedStringEquals("removing property failed", "name: \"name\",\\n", contents.content.String())
}

func TestTransformRemovePropertySet(t *testing.T) {

	helper := &TestHelper{t}

	set := newPropertySet()
	set.AddProperty("name", "name")
	set.AddPropertySet("fred")

	set.transformContents(removeFredTransformation{})

	contents := &generatedContents{}
	outputPropertySet(contents, set)
	helper.AssertTrimmedStringEquals("removing property set failed", "name: \"name\",\\n", contents.content.String())
}
