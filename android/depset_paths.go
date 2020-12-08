// Copyright 2020 Google Inc. All rights reserved.
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

// This file implements DepSet, a thin type-safe wrapper around depSet that contains Paths.

// A DepSet efficiently stores Paths from transitive dependencies without copying. It is stored
// as a DAG of DepSet nodes, each of which has some direct contents and a list of dependency
// DepSet nodes.
//
// A DepSet has an order that will be used to walk the DAG when ToList() is called.  The order
// can be POSTORDER, PREORDER, or TOPOLOGICAL.  POSTORDER and PREORDER orders return a postordered
// or preordered left to right flattened list.  TOPOLOGICAL returns a list that guarantees that
// elements of children are listed after all of their parents (unless there are duplicate direct
// elements in the DepSet or any of its transitive dependencies, in which case the ordering of the
// duplicated element is not guaranteed).
//
// A DepSet is created by NewDepSet or NewDepSetBuilder.Build from the Paths for direct contents
// and the *DepSets of dependencies. A DepSet is immutable once created.
type DepSet struct {
	depSet
}

// DepSetBuilder is used to create an immutable DepSet.
type DepSetBuilder struct {
	depSetBuilder
}

// NewDepSet returns an immutable DepSet with the given order, direct and transitive contents.
func NewDepSet(order DepSetOrder, direct Paths, transitive []*DepSet) *DepSet {
	return &DepSet{*newDepSet(order, direct, transitive)}
}

// NewDepSetBuilder returns a DepSetBuilder to create an immutable DepSet with the given order.
func NewDepSetBuilder(order DepSetOrder) *DepSetBuilder {
	return &DepSetBuilder{*newDepSetBuilder(order, Paths(nil))}
}

// Direct adds direct contents to the DepSet being built by a DepSetBuilder. Newly added direct
// contents are to the right of any existing direct contents.
func (b *DepSetBuilder) Direct(direct ...Path) *DepSetBuilder {
	b.depSetBuilder.DirectSlice(direct)
	return b
}

// Transitive adds transitive contents to the DepSet being built by a DepSetBuilder. Newly added
// transitive contents are to the right of any existing transitive contents.
func (b *DepSetBuilder) Transitive(transitive ...*DepSet) *DepSetBuilder {
	b.depSetBuilder.Transitive(transitive)
	return b
}

// Returns the DepSet being built by this DepSetBuilder.  The DepSetBuilder retains its contents
// for creating more DepSets.
func (b *DepSetBuilder) Build() *DepSet {
	return &DepSet{*b.depSetBuilder.Build()}
}

// ToList returns the DepSet flattened to a list.  The order in the list is based on the order
// of the DepSet.  POSTORDER and PREORDER orders return a postordered or preordered left to right
// flattened list.  TOPOLOGICAL returns a list that guarantees that elements of children are listed
// after all of their parents (unless there are duplicate direct elements in the DepSet or any of
// its transitive dependencies, in which case the ordering of the duplicated element is not
// guaranteed).
func (d *DepSet) ToList() Paths {
	if d == nil {
		return nil
	}
	return d.toList(func(paths interface{}) interface{} {
		return FirstUniquePaths(paths.(Paths))
	}).(Paths)
}

// ToSortedList returns the direct and transitive contents of a DepSet in lexically sorted order
// with duplicates removed.
func (d *DepSet) ToSortedList() Paths {
	if d == nil {
		return nil
	}
	paths := d.ToList()
	return SortedUniquePaths(paths)
}
