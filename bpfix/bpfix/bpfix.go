// Copyright 2017 Google Inc. All rights reserved.
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

// This file implements the logic of bpfix and also provides a programmatic interface

package bpfix

import (
	"bytes"
	"fmt"
	"path/filepath"

	"github.com/google/blueprint/parser"
)

// A FixRequest specifies the details of which fixes to apply to an individual file
// A FixRequest doesn't specify whether to do a dry run or where to write the results; that's in cmd/bpfix.go
type FixRequest struct {
	simplifyKnownRedundantVariables    bool
	rewriteIncorrectAndroidmkPrebuilts bool
}

func NewFixRequest() FixRequest {
	return FixRequest{}
}

func (r FixRequest) AddAll() (result FixRequest) {
	result = r
	result.simplifyKnownRedundantVariables = true
	result.rewriteIncorrectAndroidmkPrebuilts = true
	return result
}

// FixTree repeatedly applies the fixes listed in the given FixRequest to the given File
// until there is no fix that affects the tree
func FixTree(tree *parser.File, config FixRequest) error {
	prevIdentifier, err := fingerprint(tree)
	if err != nil {
		return err
	}

	maxNumIterations := 20
	i := 0
	for {
		err = fixTreeOnce(tree, config)
		newIdentifier, err := fingerprint(tree)
		if err != nil {
			return err
		}
		if bytes.Equal(newIdentifier, prevIdentifier) {
			break
		}
		prevIdentifier = newIdentifier
		// any errors from a previous iteration generally get thrown away and overwritten by errors on the next iteration

		// detect infinite loop
		i++
		if i >= maxNumIterations {
			return fmt.Errorf("Applied fixes %d times and yet the tree continued to change. Is there an infinite loop?", i)
			break
		}
	}
	return err
}

// returns a unique identifier for the given tree that can be used to determine whether the tree changed
func fingerprint(tree *parser.File) (fingerprint []byte, err error) {
	bytes, err := parser.Print(tree)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}

func fixTreeOnce(tree *parser.File, config FixRequest) error {
	if config.simplifyKnownRedundantVariables {
		err := simplifyKnownPropertiesDuplicatingEachOther(tree)
		if err != nil {
			return err
		}
	}
	if config.rewriteIncorrectAndroidmkPrebuilts {
		err := rewriteIncorrectAndroidmkPrebuilts(tree)
		if err != nil {
			return err
		}
	}
	return nil
}

func simplifyKnownPropertiesDuplicatingEachOther(tree *parser.File) error {
	// remove from local_include_dirs anything in export_include_dirs
	return removeMatchingModuleListProperties(tree, "export_include_dirs", "local_include_dirs")
}

func rewriteIncorrectAndroidmkPrebuilts(tree *parser.File) error {
	for _, def := range tree.Defs {
		mod, ok := def.(*parser.Module)
		if !ok {
			continue
		}
		if mod.Type != "java_import" {
			continue
		}
		srcs, ok := getLiteralListProperty(mod, "srcs")
		if !ok {
			continue
		}
		if len(srcs.Values) == 0 {
			continue
		}
		src, ok := srcs.Values[0].(*parser.String)
		if !ok {
			continue
		}
		switch filepath.Ext(src.Value) {
		case ".jar":
			renameProperty(mod, "srcs", "jars")
		case ".aar":
			renameProperty(mod, "srcs", "aars")
			mod.Type = "android_library_import"

			// An android_library_import doesn't get installed, so setting "installable = false" isn't supported
			removeProperty(mod, "installable")
		}
	}

	return nil
}

// removes from <items> every item present in <removals>
func filterExpressionList(items *parser.List, removals *parser.List) {
	writeIndex := 0
	for _, item := range items.Values {
		included := true
		for _, removal := range removals.Values {
			equal, err := parser.ExpressionsAreSame(item, removal)
			if err != nil {
				continue
			}
			if equal {
				included = false
				break
			}
		}
		if included {
			items.Values[writeIndex] = item
			writeIndex++
		}
	}
	items.Values = items.Values[:writeIndex]
}

// Remove each modules[i].Properties[<legacyName>][j] that matches a modules[i].Properties[<canonicalName>][k]
func removeMatchingModuleListProperties(tree *parser.File, canonicalName string, legacyName string) error {
	for _, def := range tree.Defs {
		mod, ok := def.(*parser.Module)
		if !ok {
			continue
		}
		legacyList, ok := getLiteralListProperty(mod, legacyName)
		if !ok {
			continue
		}
		canonicalList, ok := getLiteralListProperty(mod, canonicalName)
		if !ok {
			continue
		}
		filterExpressionList(legacyList, canonicalList)
	}
	return nil
}

func getLiteralListProperty(mod *parser.Module, name string) (list *parser.List, found bool) {
	prop, ok := mod.GetProperty(name)
	if !ok {
		return nil, false
	}
	list, ok = prop.Value.(*parser.List)
	return list, ok
}

func renameProperty(mod *parser.Module, from, to string) {
	for _, prop := range mod.Properties {
		if prop.Name == from {
			prop.Name = to
		}
	}
}

func removeProperty(mod *parser.Module, propertyName string) {
	newList := make([]*parser.Property, 0, len(mod.Properties))
	for _, prop := range mod.Properties {
		if prop.Name != propertyName {
			newList = append(newList, prop)
		}
	}
	mod.Properties = newList
}
