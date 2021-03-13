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

	"github.com/google/blueprint/proptools"
)

func propertySetFixture() interface{} {
	set := newPropertySet()
	set.AddProperty("x", "taxi")
	set.AddPropertyWithTag("y", 1729, "tag_y")
	subset := set.AddPropertySet("sub")
	subset.AddPropertyWithTag("x", "taxi", "tag_x")
	subset.AddProperty("y", 1729)
	return set
}

func intPtr(i int) *int { return &i }

type propertyStruct struct {
	X     *string
	Y     *int
	Unset *bool
	Sub   struct {
		X     *string
		Y     *int
		Unset *bool
	}
}

func propertyStructFixture() interface{} {
	str := &propertyStruct{}
	str.X = proptools.StringPtr("taxi")
	str.Y = intPtr(1729)
	str.Sub.X = proptools.StringPtr("taxi")
	str.Sub.Y = intPtr(1729)
	return str
}

func checkPropertySetFixture(t *testing.T, val interface{}, hasTags bool) {
	set := val.(*bpPropertySet)
	android.AssertDeepEquals(t, "wrong x value", "taxi", set.getValue("x"))
	android.AssertDeepEquals(t, "wrong y value", 1729, set.getValue("y"))

	subset := set.getValue("sub").(*bpPropertySet)
	android.AssertDeepEquals(t, "wrong sub.x value", "taxi", subset.getValue("x"))
	android.AssertDeepEquals(t, "wrong sub.y value", 1729, subset.getValue("y"))

	if hasTags {
		android.AssertDeepEquals(t, "wrong y tag", "tag_y", set.getTag("y"))
		android.AssertDeepEquals(t, "wrong sub.x tag", "tag_x", subset.getTag("x"))
	} else {
		android.AssertDeepEquals(t, "wrong y tag", nil, set.getTag("y"))
		android.AssertDeepEquals(t, "wrong sub.x tag", nil, subset.getTag("x"))
	}
}

func TestAddPropertySimple(t *testing.T) {
	set := newPropertySet()
	for name, val := range map[string]interface{}{
		"x":   "taxi",
		"y":   1729,
		"t":   true,
		"f":   false,
		"arr": []string{"a", "b", "c"},
	} {
		set.AddProperty(name, val)
		android.AssertDeepEquals(t, "wrong value", val, set.getValue(name))
	}
	android.AssertPanic(t, "adding x again should panic",
		func() { set.AddProperty("x", "taxi") })
	android.AssertPanic(t, "adding arr again should panic",
		func() { set.AddProperty("arr", []string{"d"}) })
}

func TestAddPropertySubset(t *testing.T) {
	getFixtureMap := map[string]func() interface{}{
		"property set":    propertySetFixture,
		"property struct": propertyStructFixture,
	}

	t.Run("add new subset", func(t *testing.T) {
		for name, getFixture := range getFixtureMap {
			t.Run(name, func(t *testing.T) {
				set := propertySetFixture().(*bpPropertySet)
				set.AddProperty("new", getFixture())
				checkPropertySetFixture(t, set, true)
				checkPropertySetFixture(t, set.getValue("new"), name == "property set")
			})
		}
	})

	t.Run("merge existing subset", func(t *testing.T) {
		for name, getFixture := range getFixtureMap {
			t.Run(name, func(t *testing.T) {
				set := newPropertySet()
				subset := set.AddPropertySet("sub")
				subset.AddProperty("flag", false)
				subset.AddPropertySet("sub")
				set.AddProperty("sub", getFixture())
				merged := set.getValue("sub").(*bpPropertySet)
				android.AssertDeepEquals(t, "wrong flag value", false, merged.getValue("flag"))
				checkPropertySetFixture(t, merged, name == "property set")
			})
		}
	})

	t.Run("add conflicting subset", func(t *testing.T) {
		set := propertySetFixture().(*bpPropertySet)
		android.AssertPanic(t, "adding x again should panic",
			func() { set.AddProperty("x", propertySetFixture()) })
	})

	t.Run("add non-pointer struct", func(t *testing.T) {
		set := propertySetFixture().(*bpPropertySet)
		str := propertyStructFixture().(*propertyStruct)
		android.AssertPanic(t, "adding a non-pointer struct should panic",
			func() { set.AddProperty("new", *str) })
	})
}

func TestAddPropertySetNew(t *testing.T) {
	set := newPropertySet()
	subset := set.AddPropertySet("sub")
	subset.AddProperty("new", "d^^b")
	android.AssertDeepEquals(t, "wrong sub.new value", "d^^b", set.getValue("sub").(*bpPropertySet).getValue("new"))
}

func TestAddPropertySetExisting(t *testing.T) {
	set := propertySetFixture().(*bpPropertySet)
	subset := set.AddPropertySet("sub")
	subset.AddProperty("new", "d^^b")
	android.AssertDeepEquals(t, "wrong sub.new value", "d^^b", set.getValue("sub").(*bpPropertySet).getValue("new"))
}

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
	set := newPropertySet()
	set.AddProperty("name", "name")
	set.AddProperty("fred", "12")

	set.transformContents(removeFredTransformation{})

	contents := &generatedContents{}
	outputPropertySet(contents, set)
	android.AssertTrimmedStringEquals(t, "removing property failed", "name: \"name\",\n", contents.content.String())
}

func TestTransformRemovePropertySet(t *testing.T) {
	set := newPropertySet()
	set.AddProperty("name", "name")
	set.AddPropertySet("fred")

	set.transformContents(removeFredTransformation{})

	contents := &generatedContents{}
	outputPropertySet(contents, set)
	android.AssertTrimmedStringEquals(t, "removing property set failed", "name: \"name\",\n", contents.content.String())
}
