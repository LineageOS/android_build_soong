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

package common

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

// TODO: move this to proptools
func extendProperties(ctx blueprint.EarlyMutatorContext,
	requiredTag, srcPrefix string, dstValues []reflect.Value, srcValue reflect.Value,
	callback func(string, string)) {
	if srcPrefix != "" {
		srcPrefix += "."
	}
	extendPropertiesRecursive(ctx, requiredTag, srcValue, dstValues, srcPrefix, "", callback)
}

func extendPropertiesRecursive(ctx blueprint.EarlyMutatorContext, requiredTag string,
	srcValue reflect.Value, dstValues []reflect.Value, srcPrefix, dstPrefix string,
	callback func(string, string)) {

	typ := srcValue.Type()
	for i := 0; i < srcValue.NumField(); i++ {
		srcField := typ.Field(i)
		if srcField.PkgPath != "" {
			// The field is not exported so just skip it.
			continue
		}

		localPropertyName := proptools.PropertyNameForField(srcField.Name)
		srcPropertyName := srcPrefix + localPropertyName
		srcFieldValue := srcValue.Field(i)

		if !ctx.ContainsProperty(srcPropertyName) {
			continue
		}

		found := false
		for _, dstValue := range dstValues {
			dstField, ok := dstValue.Type().FieldByName(srcField.Name)
			if !ok {
				continue
			}

			dstFieldValue := dstValue.FieldByIndex(dstField.Index)

			if srcFieldValue.Type() != dstFieldValue.Type() {
				panic(fmt.Errorf("can't extend mismatching types for %q (%s <- %s)",
					srcPropertyName, dstFieldValue.Type(), srcFieldValue.Type()))
			}

			dstPropertyName := dstPrefix + localPropertyName

			if requiredTag != "" {
				tag := dstField.Tag.Get("android")
				tags := map[string]bool{}
				for _, entry := range strings.Split(tag, ",") {
					if entry != "" {
						tags[entry] = true
					}
				}

				if !tags[requiredTag] {
					ctx.PropertyErrorf(srcPropertyName, "property can't be specific to a build variant")
					continue
				}
			}

			if callback != nil {
				callback(srcPropertyName, dstPropertyName)
			}

			found = true

			switch srcFieldValue.Kind() {
			case reflect.Bool:
				// Replace the original value.
				dstFieldValue.Set(srcFieldValue)
			case reflect.String:
				// Append the extension string.
				dstFieldValue.SetString(dstFieldValue.String() +
					srcFieldValue.String())
			case reflect.Slice:
				dstFieldValue.Set(reflect.AppendSlice(dstFieldValue, srcFieldValue))
			case reflect.Interface:
				if dstFieldValue.IsNil() != srcFieldValue.IsNil() {
					panic(fmt.Errorf("can't extend field %q: nilitude mismatch", srcPropertyName))
				}
				if dstFieldValue.IsNil() {
					continue
				}

				dstFieldValue = dstFieldValue.Elem()
				srcFieldValue = srcFieldValue.Elem()

				if dstFieldValue.Type() != srcFieldValue.Type() {
					panic(fmt.Errorf("can't extend field %q: type mismatch", srcPropertyName))
				}
				if srcFieldValue.Kind() != reflect.Ptr {
					panic(fmt.Errorf("can't extend field %q: interface not a pointer", srcPropertyName))
				}
				fallthrough
			case reflect.Ptr:
				if dstFieldValue.IsNil() != srcFieldValue.IsNil() {
					panic(fmt.Errorf("can't extend field %q: nilitude mismatch", srcPropertyName))
				}
				if dstFieldValue.IsNil() {
					continue
				}

				dstFieldValue = dstFieldValue.Elem()
				srcFieldValue = srcFieldValue.Elem()

				if dstFieldValue.Type() != srcFieldValue.Type() {
					panic(fmt.Errorf("can't extend field %q: type mismatch", srcPropertyName))
				}
				if srcFieldValue.Kind() != reflect.Struct {
					panic(fmt.Errorf("can't extend field %q: pointer not to a struct", srcPropertyName))
				}
				fallthrough
			case reflect.Struct:
				// Recursively extend the struct's fields.
				extendPropertiesRecursive(ctx, requiredTag, srcFieldValue, []reflect.Value{dstFieldValue},
					srcPropertyName+".", srcPropertyName+".", callback)
			default:
				panic(fmt.Errorf("unexpected kind for property struct field %q: %s",
					srcPropertyName, srcFieldValue.Kind()))
			}
		}
		if !found {
			ctx.PropertyErrorf(srcPropertyName, "failed to find property to extend")
		}
	}
}
