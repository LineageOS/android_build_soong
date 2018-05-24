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
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/google/blueprint/parser"
)

// Reformat takes a blueprint file as a string and returns a formatted version
func Reformat(input string) (string, error) {
	tree, err := parse("<string>", bytes.NewBufferString(input))
	if err != nil {
		return "", err
	}

	res, err := parser.Print(tree)
	if err != nil {
		return "", err
	}

	return string(res), nil
}

// A FixRequest specifies the details of which fixes to apply to an individual file
// A FixRequest doesn't specify whether to do a dry run or where to write the results; that's in cmd/bpfix.go
type FixRequest struct {
	steps []fixStep
}

type fixStep struct {
	name string
	fix  func(f *Fixer) error
}

var fixSteps = []fixStep{
	{
		name: "simplifyKnownRedundantVariables",
		fix:  runPatchListMod(simplifyKnownPropertiesDuplicatingEachOther),
	},
	{
		name: "rewriteIncorrectAndroidmkPrebuilts",
		fix:  rewriteIncorrectAndroidmkPrebuilts,
	},
	{
		name: "rewriteIncorrectAndroidmkAndroidLibraries",
		fix:  rewriteIncorrectAndroidmkAndroidLibraries,
	},
	{
		name: "mergeMatchingModuleProperties",
		fix:  runPatchListMod(mergeMatchingModuleProperties),
	},
	{
		name: "reorderCommonProperties",
		fix:  runPatchListMod(reorderCommonProperties),
	},
	{
		name: "removeTags",
		fix:  runPatchListMod(removeTags),
	},
}

func NewFixRequest() FixRequest {
	return FixRequest{}
}

func (r FixRequest) AddAll() (result FixRequest) {
	result.steps = append([]fixStep(nil), r.steps...)
	result.steps = append(result.steps, fixSteps...)
	return result
}

type Fixer struct {
	tree *parser.File
}

func NewFixer(tree *parser.File) *Fixer {
	fixer := &Fixer{tree}

	// make a copy of the tree
	fixer.reparse()

	return fixer
}

// Fix repeatedly applies the fixes listed in the given FixRequest to the given File
// until there is no fix that affects the tree
func (f *Fixer) Fix(config FixRequest) (*parser.File, error) {
	prevIdentifier, err := f.fingerprint()
	if err != nil {
		return nil, err
	}

	maxNumIterations := 20
	i := 0
	for {
		err = f.fixTreeOnce(config)
		newIdentifier, err := f.fingerprint()
		if err != nil {
			return nil, err
		}
		if bytes.Equal(newIdentifier, prevIdentifier) {
			break
		}
		prevIdentifier = newIdentifier
		// any errors from a previous iteration generally get thrown away and overwritten by errors on the next iteration

		// detect infinite loop
		i++
		if i >= maxNumIterations {
			return nil, fmt.Errorf("Applied fixes %d times and yet the tree continued to change. Is there an infinite loop?", i)
			break
		}
	}
	return f.tree, err
}

// returns a unique identifier for the given tree that can be used to determine whether the tree changed
func (f *Fixer) fingerprint() (fingerprint []byte, err error) {
	bytes, err := parser.Print(f.tree)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}

func (f *Fixer) reparse() ([]byte, error) {
	buf, err := parser.Print(f.tree)
	if err != nil {
		return nil, err
	}
	newTree, err := parse(f.tree.Name, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	f.tree = newTree
	return buf, nil
}

func parse(name string, r io.Reader) (*parser.File, error) {
	tree, errs := parser.Parse(name, r, parser.NewScope(nil))
	if errs != nil {
		s := "parse error: "
		for _, err := range errs {
			s += "\n" + err.Error()
		}
		return nil, errors.New(s)
	}
	return tree, nil
}

func (f *Fixer) fixTreeOnce(config FixRequest) error {
	for _, fix := range config.steps {
		err := fix.fix(f)
		if err != nil {
			return err
		}
	}
	return nil
}

func simplifyKnownPropertiesDuplicatingEachOther(mod *parser.Module, buf []byte, patchList *parser.PatchList) error {
	// remove from local_include_dirs anything in export_include_dirs
	return removeMatchingModuleListProperties(mod, patchList,
		"export_include_dirs", "local_include_dirs")
}

func rewriteIncorrectAndroidmkPrebuilts(f *Fixer) error {
	for _, def := range f.tree.Defs {
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

func rewriteIncorrectAndroidmkAndroidLibraries(f *Fixer) error {
	for _, def := range f.tree.Defs {
		mod, ok := def.(*parser.Module)
		if !ok {
			continue
		}

		if !strings.HasPrefix(mod.Type, "java_") && !strings.HasPrefix(mod.Type, "android_") {
			continue
		}

		hasAndroidLibraries := hasNonEmptyLiteralListProperty(mod, "android_libs")
		hasStaticAndroidLibraries := hasNonEmptyLiteralListProperty(mod, "android_static_libs")
		hasResourceDirs := hasNonEmptyLiteralListProperty(mod, "resource_dirs")

		if hasAndroidLibraries || hasStaticAndroidLibraries || hasResourceDirs {
			if mod.Type == "java_library_static" {
				mod.Type = "android_library"
			}
		}

		if mod.Type == "java_import" && !hasStaticAndroidLibraries {
			removeProperty(mod, "android_static_libs")
		}

		// These may conflict with existing libs and static_libs properties, but the
		// mergeMatchingModuleProperties pass will fix it.
		renameProperty(mod, "shared_libs", "libs")
		renameProperty(mod, "android_libs", "libs")
		renameProperty(mod, "android_static_libs", "static_libs")
	}

	return nil
}

func runPatchListMod(modFunc func(mod *parser.Module, buf []byte, patchlist *parser.PatchList) error) func(*Fixer) error {
	return func(f *Fixer) error {
		// Make sure all the offsets are accurate
		buf, err := f.reparse()
		if err != nil {
			return err
		}

		var patchlist parser.PatchList
		for _, def := range f.tree.Defs {
			mod, ok := def.(*parser.Module)
			if !ok {
				continue
			}

			err := modFunc(mod, buf, &patchlist)
			if err != nil {
				return err
			}
		}

		newBuf := new(bytes.Buffer)
		err = patchlist.Apply(bytes.NewReader(buf), newBuf)
		if err != nil {
			return err
		}

		// Save a copy of the buffer to print for errors below
		bufCopy := append([]byte(nil), newBuf.Bytes()...)

		newTree, err := parse(f.tree.Name, newBuf)
		if err != nil {
			return fmt.Errorf("Failed to parse: %v\nBuffer:\n%s", err, string(bufCopy))
		}

		f.tree = newTree

		return nil
	}
}

var commonPropertyPriorities = []string{
	"name",
	"defaults",
	"device_supported",
	"host_supported",
}

func reorderCommonProperties(mod *parser.Module, buf []byte, patchlist *parser.PatchList) error {
	if len(mod.Properties) == 0 {
		return nil
	}

	pos := mod.LBracePos.Offset + 1
	stage := ""

	for _, name := range commonPropertyPriorities {
		idx := propertyIndex(mod.Properties, name)
		if idx == -1 {
			continue
		}
		if idx == 0 {
			err := patchlist.Add(pos, pos, stage)
			if err != nil {
				return err
			}
			stage = ""

			pos = mod.Properties[0].End().Offset + 1
			mod.Properties = mod.Properties[1:]
			continue
		}

		prop := mod.Properties[idx]
		mod.Properties = append(mod.Properties[:idx], mod.Properties[idx+1:]...)

		stage += string(buf[prop.Pos().Offset : prop.End().Offset+1])

		err := patchlist.Add(prop.Pos().Offset, prop.End().Offset+2, "")
		if err != nil {
			return err
		}
	}

	if stage != "" {
		err := patchlist.Add(pos, pos, stage)
		if err != nil {
			return err
		}
	}

	return nil
}

func removeTags(mod *parser.Module, buf []byte, patchlist *parser.PatchList) error {
	prop, ok := mod.GetProperty("tags")
	if !ok {
		return nil
	}
	list, ok := prop.Value.(*parser.List)
	if !ok {
		return nil
	}

	replaceStr := ""

	for _, item := range list.Values {
		str, ok := item.(*parser.String)
		if !ok {
			replaceStr += fmt.Sprintf("// ERROR: Unable to parse tag %q\n", item)
			continue
		}

		switch str.Value {
		case "optional":
			continue
		case "debug":
			replaceStr += `// WARNING: Module tags are not supported in Soong.
				// Add this module to PRODUCT_PACKAGES_DEBUG in your product file if you want to
				// force installation for -userdebug and -eng builds.
				`
		case "eng":
			replaceStr += `// WARNING: Module tags are not supported in Soong.
				// Add this module to PRODUCT_PACKAGES_ENG in your product file if you want to
				// force installation for -eng builds.
				`
		case "tests":
			if strings.Contains(mod.Type, "cc_test") || strings.Contains(mod.Type, "cc_library_static") {
				continue
			} else if strings.Contains(mod.Type, "cc_lib") {
				replaceStr += `// WARNING: Module tags are not supported in Soong.
					// To make a shared library only for tests, use the "cc_test_library" module
					// type. If you don't use gtest, set "gtest: false".
					`
			} else if strings.Contains(mod.Type, "cc_bin") {
				replaceStr += `// WARNING: Module tags are not supported in Soong.
					// For native test binaries, use the "cc_test" module type. Some differences:
					//  - If you don't use gtest, set "gtest: false"
					//  - Binaries will be installed into /data/nativetest[64]/<name>/<name>
					//  - Both 32 & 64 bit versions will be built (as appropriate)
					`
			} else if strings.Contains(mod.Type, "java_lib") {
				replaceStr += `// WARNING: Module tags are not supported in Soong.
					// For JUnit or similar tests, use the "java_test" module type. A dependency on
					// Junit will be added by default, if it is using some other runner, set "junit: false".
					`
			} else {
				replaceStr += `// WARNING: Module tags are not supported in Soong.
					// In most cases, tests are now identified by their module type:
					// cc_test, java_test, python_test
					`
			}
		default:
			replaceStr += fmt.Sprintf("// WARNING: Unknown module tag %q\n", str.Value)
		}
	}

	return patchlist.Add(prop.Pos().Offset, prop.End().Offset+2, replaceStr)
}

func mergeMatchingModuleProperties(mod *parser.Module, buf []byte, patchlist *parser.PatchList) error {
	return mergeMatchingProperties(&mod.Properties, buf, patchlist)
}

func mergeMatchingProperties(properties *[]*parser.Property, buf []byte, patchlist *parser.PatchList) error {
	seen := make(map[string]*parser.Property)
	for i := 0; i < len(*properties); i++ {
		property := (*properties)[i]
		if prev, exists := seen[property.Name]; exists {
			err := mergeProperties(prev, property, buf, patchlist)
			if err != nil {
				return err
			}
			*properties = append((*properties)[:i], (*properties)[i+1:]...)
		} else {
			seen[property.Name] = property
			if mapProperty, ok := property.Value.(*parser.Map); ok {
				err := mergeMatchingProperties(&mapProperty.Properties, buf, patchlist)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func mergeProperties(a, b *parser.Property, buf []byte, patchlist *parser.PatchList) error {
	if a.Value.Type() != b.Value.Type() {
		return fmt.Errorf("type mismatch when merging properties %q: %s and %s", a.Name, a.Value.Type(), b.Value.Type())
	}

	switch a.Value.Type() {
	case parser.StringType:
		return fmt.Errorf("conflicting definitions of string property %q", a.Name)
	case parser.ListType:
		return mergeListProperties(a, b, buf, patchlist)
	}

	return nil
}

func mergeListProperties(a, b *parser.Property, buf []byte, patchlist *parser.PatchList) error {
	aval, oka := a.Value.(*parser.List)
	bval, okb := b.Value.(*parser.List)
	if !oka || !okb {
		// Merging expressions not supported yet
		return nil
	}

	s := string(buf[bval.LBracePos.Offset+1 : bval.RBracePos.Offset])
	if bval.LBracePos.Line != bval.RBracePos.Line {
		if s[0] != '\n' {
			panic("expected \n")
		}
		// If B is a multi line list, skip the first "\n" in case A already has a trailing "\n"
		s = s[1:]
	}
	if aval.LBracePos.Line == aval.RBracePos.Line {
		// A is a single line list with no trailing comma
		if len(aval.Values) > 0 {
			s = "," + s
		}
	}

	err := patchlist.Add(aval.RBracePos.Offset, aval.RBracePos.Offset, s)
	if err != nil {
		return err
	}
	err = patchlist.Add(b.NamePos.Offset, b.End().Offset+2, "")
	if err != nil {
		return err
	}

	return nil
}

// removes from <items> every item present in <removals>
func filterExpressionList(patchList *parser.PatchList, items *parser.List, removals *parser.List) {
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
		} else {
			patchList.Add(item.Pos().Offset, item.End().Offset+2, "")
		}
	}
	items.Values = items.Values[:writeIndex]
}

// Remove each modules[i].Properties[<legacyName>][j] that matches a modules[i].Properties[<canonicalName>][k]
func removeMatchingModuleListProperties(mod *parser.Module, patchList *parser.PatchList, canonicalName string, legacyName string) error {
	legacyProp, ok := mod.GetProperty(legacyName)
	if !ok {
		return nil
	}
	legacyList, ok := legacyProp.Value.(*parser.List)
	if !ok || len(legacyList.Values) == 0 {
		return nil
	}
	canonicalList, ok := getLiteralListProperty(mod, canonicalName)
	if !ok {
		return nil
	}

	localPatches := parser.PatchList{}
	filterExpressionList(&localPatches, legacyList, canonicalList)

	if len(legacyList.Values) == 0 {
		patchList.Add(legacyProp.Pos().Offset, legacyProp.End().Offset+2, "")
	} else {
		for _, p := range localPatches {
			patchList.Add(p.Start, p.End, p.Replacement)
		}
	}

	return nil
}

func hasNonEmptyLiteralListProperty(mod *parser.Module, name string) bool {
	list, found := getLiteralListProperty(mod, name)
	return found && len(list.Values) > 0
}

func getLiteralListProperty(mod *parser.Module, name string) (list *parser.List, found bool) {
	prop, ok := mod.GetProperty(name)
	if !ok {
		return nil, false
	}
	list, ok = prop.Value.(*parser.List)
	return list, ok
}

func propertyIndex(props []*parser.Property, propertyName string) int {
	for i, prop := range props {
		if prop.Name == propertyName {
			return i
		}
	}
	return -1
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
