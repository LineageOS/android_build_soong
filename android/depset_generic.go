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

import (
	"fmt"
)

// DepSet is designed to be conceptually compatible with Bazel's depsets:
// https://docs.bazel.build/versions/master/skylark/depsets.html

type DepSetOrder int

const (
	PREORDER DepSetOrder = iota
	POSTORDER
	TOPOLOGICAL
)

func (o DepSetOrder) String() string {
	switch o {
	case PREORDER:
		return "PREORDER"
	case POSTORDER:
		return "POSTORDER"
	case TOPOLOGICAL:
		return "TOPOLOGICAL"
	default:
		panic(fmt.Errorf("Invalid DepSetOrder %d", o))
	}
}

type depSettableType comparable

// A DepSet efficiently stores a slice of an arbitrary type from transitive dependencies without
// copying. It is stored as a DAG of DepSet nodes, each of which has some direct contents and a list
// of dependency DepSet nodes.
//
// A DepSet has an order that will be used to walk the DAG when ToList() is called.  The order
// can be POSTORDER, PREORDER, or TOPOLOGICAL.  POSTORDER and PREORDER orders return a postordered
// or preordered left to right flattened list.  TOPOLOGICAL returns a list that guarantees that
// elements of children are listed after all of their parents (unless there are duplicate direct
// elements in the DepSet or any of its transitive dependencies, in which case the ordering of the
// duplicated element is not guaranteed).
//
// A DepSet is created by NewDepSet or NewDepSetBuilder.Build from the slice for direct contents
// and the *DepSets of dependencies. A DepSet is immutable once created.
type DepSet[T depSettableType] struct {
	preorder   bool
	reverse    bool
	order      DepSetOrder
	direct     []T
	transitive []*DepSet[T]
}

// NewDepSet returns an immutable DepSet with the given order, direct and transitive contents.
func NewDepSet[T depSettableType](order DepSetOrder, direct []T, transitive []*DepSet[T]) *DepSet[T] {
	var directCopy []T
	var transitiveCopy []*DepSet[T]
	for _, t := range transitive {
		if t.order != order {
			panic(fmt.Errorf("incompatible order, new DepSet is %s but transitive DepSet is %s",
				order, t.order))
		}
	}

	if order == TOPOLOGICAL {
		// TOPOLOGICAL is implemented as a postorder traversal followed by reversing the output.
		// Pre-reverse the inputs here so their order is maintained in the output.
		directCopy = ReverseSlice(direct)
		transitiveCopy = ReverseSlice(transitive)
	} else {
		directCopy = append([]T(nil), direct...)
		transitiveCopy = append([]*DepSet[T](nil), transitive...)
	}

	return &DepSet[T]{
		preorder:   order == PREORDER,
		reverse:    order == TOPOLOGICAL,
		order:      order,
		direct:     directCopy,
		transitive: transitiveCopy,
	}
}

// DepSetBuilder is used to create an immutable DepSet.
type DepSetBuilder[T depSettableType] struct {
	order      DepSetOrder
	direct     []T
	transitive []*DepSet[T]
}

// NewDepSetBuilder returns a DepSetBuilder to create an immutable DepSet with the given order and
// type, represented by a slice of type that will be in the DepSet.
func NewDepSetBuilder[T depSettableType](order DepSetOrder) *DepSetBuilder[T] {
	return &DepSetBuilder[T]{
		order: order,
	}
}

// DirectSlice adds direct contents to the DepSet being built by a DepSetBuilder. Newly added direct
// contents are to the right of any existing direct contents.
func (b *DepSetBuilder[T]) DirectSlice(direct []T) *DepSetBuilder[T] {
	b.direct = append(b.direct, direct...)
	return b
}

// Direct adds direct contents to the DepSet being built by a DepSetBuilder. Newly added direct
// contents are to the right of any existing direct contents.
func (b *DepSetBuilder[T]) Direct(direct ...T) *DepSetBuilder[T] {
	b.direct = append(b.direct, direct...)
	return b
}

// Transitive adds transitive contents to the DepSet being built by a DepSetBuilder. Newly added
// transitive contents are to the right of any existing transitive contents.
func (b *DepSetBuilder[T]) Transitive(transitive ...*DepSet[T]) *DepSetBuilder[T] {
	for _, t := range transitive {
		if t.order != b.order {
			panic(fmt.Errorf("incompatible order, new DepSet is %s but transitive DepSet is %s",
				b.order, t.order))
		}
	}
	b.transitive = append(b.transitive, transitive...)
	return b
}

// Returns the DepSet being built by this DepSetBuilder.  The DepSetBuilder retains its contents
// for creating more depSets.
func (b *DepSetBuilder[T]) Build() *DepSet[T] {
	return NewDepSet(b.order, b.direct, b.transitive)
}

// walk calls the visit method in depth-first order on a DepSet, preordered if d.preorder is set,
// otherwise postordered.
func (d *DepSet[T]) walk(visit func([]T)) {
	visited := make(map[*DepSet[T]]bool)

	var dfs func(d *DepSet[T])
	dfs = func(d *DepSet[T]) {
		visited[d] = true
		if d.preorder {
			visit(d.direct)
		}
		for _, dep := range d.transitive {
			if !visited[dep] {
				dfs(dep)
			}
		}

		if !d.preorder {
			visit(d.direct)
		}
	}

	dfs(d)
}

// ToList returns the DepSet flattened to a list.  The order in the list is based on the order
// of the DepSet.  POSTORDER and PREORDER orders return a postordered or preordered left to right
// flattened list.  TOPOLOGICAL returns a list that guarantees that elements of children are listed
// after all of their parents (unless there are duplicate direct elements in the DepSet or any of
// its transitive dependencies, in which case the ordering of the duplicated element is not
// guaranteed).
func (d *DepSet[T]) ToList() []T {
	if d == nil {
		return nil
	}
	var list []T
	d.walk(func(paths []T) {
		list = append(list, paths...)
	})
	list = firstUniqueInPlace(list)
	if d.reverse {
		ReverseSliceInPlace(list)
	}
	return list
}
