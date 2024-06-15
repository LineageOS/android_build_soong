/*
 * Copyright (C) 2021 The Android Open Source Project
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package java

import (
	"fmt"
	"strings"

	"android/soong/android"
)

// Supports constructing a list of ClasspathElement from a set of fragments and modules.

// ClasspathElement represents a component that contributes to a classpath. That can be
// either a java module or a classpath fragment module.
type ClasspathElement interface {
	Module() android.Module
	String() string
}

type ClasspathElements []ClasspathElement

// ClasspathFragmentElement is a ClasspathElement that encapsulates a classpath fragment module.
type ClasspathFragmentElement struct {
	Fragment android.Module
	Contents []android.Module
}

func (b *ClasspathFragmentElement) Module() android.Module {
	return b.Fragment
}

func (b *ClasspathFragmentElement) String() string {
	contents := []string{}
	for _, module := range b.Contents {
		contents = append(contents, module.String())
	}
	return fmt.Sprintf("fragment(%s, %s)", b.Fragment, strings.Join(contents, ", "))
}

var _ ClasspathElement = (*ClasspathFragmentElement)(nil)

// ClasspathLibraryElement is a ClasspathElement that encapsulates a java library.
type ClasspathLibraryElement struct {
	Library android.Module
}

func (b *ClasspathLibraryElement) Module() android.Module {
	return b.Library
}

func (b *ClasspathLibraryElement) String() string {
	return fmt.Sprintf("library{%s}", b.Library)
}

var _ ClasspathElement = (*ClasspathLibraryElement)(nil)

// ClasspathElementContext defines the context methods needed by CreateClasspathElements
type ClasspathElementContext interface {
	android.OtherModuleProviderContext
	ModuleErrorf(fmt string, args ...interface{})
}

// CreateClasspathElements creates a list of ClasspathElement objects from a list of libraries and
// a list of fragments.
//
// The libraries parameter contains the set of libraries from which the classpath is constructed.
// The fragments parameter contains the classpath fragment modules whose contents are libraries that
// are part of the classpath. Each library in the libraries parameter may be part of a fragment. The
// determination as to which libraries belong to fragments and which do not is based on the apex to
// which they belong, if any.
//
// Every fragment in the fragments list must be part of one or more apexes and each apex is assumed
// to contain only a single fragment from the fragments list. A library in the libraries parameter
// that is part of an apex must be provided by a classpath fragment in the corresponding apex.
//
// This will return a ClasspathElements list that contains a ClasspathElement for each standalone
// library and each fragment. The order of the elements in the list is such that if the list was
// flattened into a list of library modules that it would result in the same list or modules as the
// input libraries. Flattening the list can be done by replacing each ClasspathFragmentElement in
// the list with its Contents field.
//
// Requirements/Assumptions:
//   - A fragment can be associated with more than one apex but each apex must only be associated with
//     a single fragment from the fragments list.
//   - All of a fragment's contents must appear as a contiguous block in the same order in the
//     libraries list.
//   - Each library must only appear in a single fragment.
//
// The apex is used to identify which libraries belong to which fragment. First a mapping is created
// from apex to fragment. Then the libraries are iterated over and any library in an apex is
// associated with an element for the fragment to which it belongs. Otherwise, the libraries are
// standalone and have their own element.
//
// e.g. Given the following input:
//
//	libraries: com.android.art:core-oj, com.android.art:core-libart, framework, ext
//	fragments: com.android.art:art-bootclasspath-fragment
//
// Then this will return:
//
//	ClasspathFragmentElement(art-bootclasspath-fragment, [core-oj, core-libart]),
//	ClasspathLibraryElement(framework),
//	ClasspathLibraryElement(ext),
func CreateClasspathElements(ctx ClasspathElementContext, libraries []android.Module, fragments []android.Module) ClasspathElements {
	// Create a map from apex name to the fragment module. This makes it easy to find the fragment
	// associated with a particular apex.
	apexToFragment := map[string]android.Module{}
	for _, fragment := range fragments {
		apexInfo, ok := android.OtherModuleProvider(ctx, fragment, android.ApexInfoProvider)
		if !ok {
			ctx.ModuleErrorf("fragment %s is not part of an apex", fragment)
			continue
		}

		for _, apex := range apexInfo.InApexVariants {
			if existing, ok := apexToFragment[apex]; ok {
				ctx.ModuleErrorf("apex %s has multiple fragments, %s and %s", apex, fragment, existing)
				continue
			}
			apexToFragment[apex] = fragment
		}
	}

	fragmentToElement := map[android.Module]*ClasspathFragmentElement{}
	elements := []ClasspathElement{}
	var currentElement ClasspathElement

skipLibrary:
	// Iterate over the libraries to construct the ClasspathElements list.
	for _, library := range libraries {
		var element ClasspathElement
		if apexInfo, ok := android.OtherModuleProvider(ctx, library, android.ApexInfoProvider); ok {

			var fragment android.Module

			// Make sure that the library is in only one fragment of the classpath.
			for _, apex := range apexInfo.InApexVariants {
				if f, ok := apexToFragment[apex]; ok {
					if fragment == nil {
						// This is the first fragment so just save it away.
						fragment = f
					} else if f != fragment {
						// This apex variant of the library is in a different fragment.
						ctx.ModuleErrorf("library %s is in two separate fragments, %s and %s", library, fragment, f)
						// Skip over this library entirely as otherwise the resulting classpath elements would
						// be invalid.
						continue skipLibrary
					}
				} else {
					// There is no fragment associated with the library's apex.
				}
			}

			if fragment == nil {
				ctx.ModuleErrorf("library %s is from apexes %s which have no corresponding fragment in %s",
					library, apexInfo.InApexVariants, fragments)
				// Skip over this library entirely as otherwise the resulting classpath elements would
				// be invalid.
				continue skipLibrary
			} else if existingFragmentElement, ok := fragmentToElement[fragment]; ok {
				// This library is in a fragment element that has already been added.

				// If the existing fragment element is still the current element then this library is
				// contiguous with other libraries in that fragment so there is nothing more to do.
				// Otherwise this library is not contiguous with other libraries in the same fragment which
				// is an error.
				if existingFragmentElement != currentElement {
					separator := ""
					if fragmentElement, ok := currentElement.(*ClasspathFragmentElement); ok {
						separator = fmt.Sprintf("libraries from fragment %s like %s", fragmentElement.Fragment, fragmentElement.Contents[0])
					} else {
						libraryElement := currentElement.(*ClasspathLibraryElement)
						separator = fmt.Sprintf("library %s", libraryElement.Library)
					}

					// Get the library that precedes this library in the fragment. That is the last library as
					// this library has not yet been added.
					precedingLibraryInFragment := existingFragmentElement.Contents[len(existingFragmentElement.Contents)-1]
					ctx.ModuleErrorf("libraries from the same fragment must be contiguous, however %s and %s from fragment %s are separated by %s",
						precedingLibraryInFragment, library, fragment, separator)
				}

				// Add this library to the fragment element's contents.
				existingFragmentElement.Contents = append(existingFragmentElement.Contents, library)
			} else {
				// This is the first library in this fragment so add a new element for the fragment,
				// including the library.
				fragmentElement := &ClasspathFragmentElement{
					Fragment: fragment,
					Contents: []android.Module{library},
				}

				// Store it away so we can detect when attempting to create another element for the same
				// fragment.
				fragmentToElement[fragment] = fragmentElement
				element = fragmentElement
			}
		} else {
			// The library is from the platform so just add an element for it.
			element = &ClasspathLibraryElement{Library: library}
		}

		// If no element was created then it means that the library has been added to an existing
		// fragment element so the list of elements and current element are unaffected.
		if element != nil {
			// Add the element to the list and make it the current element for the next iteration.
			elements = append(elements, element)
			currentElement = element
		}
	}

	return elements
}
