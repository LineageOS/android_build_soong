// Copyright 2019 Google Inc. All rights reserved.
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

	"github.com/google/blueprint/proptools"
)

func registerPathDepsMutator(ctx RegisterMutatorsContext) {
	ctx.BottomUp("pathdeps", pathDepsMutator).Parallel()
}

// The pathDepsMutator automatically adds dependencies on any module that is listed with ":module" syntax in a
// property that is tagged with android:"path".
func pathDepsMutator(ctx BottomUpMutatorContext) {
	m := ctx.Module().(Module)
	if m == nil {
		return
	}

	props := m.base().generalProperties

	var pathProperties []string
	for _, ps := range props {
		pathProperties = append(pathProperties, pathPropertiesForPropertyStruct(ctx, ps)...)
	}

	pathProperties = FirstUniqueStrings(pathProperties)

	for _, s := range pathProperties {
		if m, t := SrcIsModuleWithTag(s); m != "" {
			ctx.AddDependency(ctx.Module(), sourceOrOutputDepTag(t), m)
		}
	}
}

// pathPropertiesForPropertyStruct uses the indexes of properties that are tagged with android:"path" to extract
// all their values from a property struct, returning them as a single slice of strings..
func pathPropertiesForPropertyStruct(ctx BottomUpMutatorContext, ps interface{}) []string {
	v := reflect.ValueOf(ps)
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		panic(fmt.Errorf("type %s is not a pointer to a struct", v.Type()))
	}
	if v.IsNil() {
		return nil
	}
	v = v.Elem()

	pathPropertyIndexes := pathPropertyIndexesForPropertyStruct(ps)

	var ret []string

	for _, i := range pathPropertyIndexes {
		sv := fieldByIndex(v, i)
		if !sv.IsValid() {
			continue
		}

		if sv.Kind() == reflect.Ptr {
			if sv.IsNil() {
				continue
			}
			sv = sv.Elem()
		}
		switch sv.Kind() {
		case reflect.String:
			ret = append(ret, sv.String())
		case reflect.Slice:
			ret = append(ret, sv.Interface().([]string)...)
		default:
			panic(fmt.Errorf(`field %s in type %s has tag android:"path" but is not a string or slice of strings, it is a %s`,
				v.Type().FieldByIndex(i).Name, v.Type(), sv.Type()))
		}
	}

	return ret
}

// fieldByIndex is like reflect.Value.FieldByIndex, but returns an invalid reflect.Value when traversing a nil pointer
// to a struct.
func fieldByIndex(v reflect.Value, index []int) reflect.Value {
	if len(index) == 1 {
		return v.Field(index[0])
	}
	for _, x := range index {
		if v.Kind() == reflect.Ptr {
			if v.IsNil() {
				return reflect.Value{}
			}
			v = v.Elem()
		}
		v = v.Field(x)
	}
	return v
}

var pathPropertyIndexesCache OncePer

// pathPropertyIndexesForPropertyStruct returns a list of all of the indexes of properties in property struct type that
// are tagged with android:"path".  Each index is a []int suitable for passing to reflect.Value.FieldByIndex.  The value
// is cached in a global cache by type.
func pathPropertyIndexesForPropertyStruct(ps interface{}) [][]int {
	key := NewCustomOnceKey(reflect.TypeOf(ps))
	return pathPropertyIndexesCache.Once(key, func() interface{} {
		return proptools.PropertyIndexesWithTag(ps, "android", "path")
	}).([][]int)
}
