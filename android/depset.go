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

import "fmt"

// DepSet is designed to be conceptually compatible with Bazel's depsets:
// https://docs.bazel.build/versions/master/skylark/depsets.html

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
	preorder   bool
	reverse    bool
	order      DepSetOrder
	direct     Paths
	transitive []*DepSet
}

// DepSetBuilder is used to create an immutable DepSet.
type DepSetBuilder struct {
	order      DepSetOrder
	direct     Paths
	transitive []*DepSet
}

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

// NewDepSet returns an immutable DepSet with the given order, direct and transitive contents.
func NewDepSet(order DepSetOrder, direct Paths, transitive []*DepSet) *DepSet {
	var directCopy Paths
	var transitiveCopy []*DepSet
	if order == TOPOLOGICAL {
		directCopy = ReversePaths(direct)
		transitiveCopy = reverseDepSets(transitive)
	} else {
		// Use copy instead of append(nil, ...) to make a slice that is exactly the size of the input
		// slice.  The DepSet is immutable, there is no need for additional capacity.
		directCopy = make(Paths, len(direct))
		copy(directCopy, direct)
		transitiveCopy = make([]*DepSet, len(transitive))
		copy(transitiveCopy, transitive)
	}

	for _, dep := range transitive {
		if dep.order != order {
			panic(fmt.Errorf("incompatible order, new DepSet is %s but transitive DepSet is %s",
				order, dep.order))
		}
	}

	return &DepSet{
		preorder:   order == PREORDER,
		reverse:    order == TOPOLOGICAL,
		order:      order,
		direct:     directCopy,
		transitive: transitiveCopy,
	}
}

// NewDepSetBuilder returns a DepSetBuilder to create an immutable DepSet with the given order.
func NewDepSetBuilder(order DepSetOrder) *DepSetBuilder {
	return &DepSetBuilder{order: order}
}

// Direct adds direct contents to the DepSet being built by a DepSetBuilder. Newly added direct
// contents are to the right of any existing direct contents.
func (b *DepSetBuilder) Direct(direct ...Path) *DepSetBuilder {
	b.direct = append(b.direct, direct...)
	return b
}

// Transitive adds transitive contents to the DepSet being built by a DepSetBuilder. Newly added
// transitive contents are to the right of any existing transitive contents.
func (b *DepSetBuilder) Transitive(transitive ...*DepSet) *DepSetBuilder {
	b.transitive = append(b.transitive, transitive...)
	return b
}

// Returns the DepSet being built by this DepSetBuilder.  The DepSetBuilder retains its contents
// for creating more DepSets.
func (b *DepSetBuilder) Build() *DepSet {
	return NewDepSet(b.order, b.direct, b.transitive)
}

// walk calls the visit method in depth-first order on a DepSet, preordered if d.preorder is set,
// otherwise postordered.
func (d *DepSet) walk(visit func(Paths)) {
	visited := make(map[*DepSet]bool)

	var dfs func(d *DepSet)
	dfs = func(d *DepSet) {
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
func (d *DepSet) ToList() Paths {
	var list Paths
	d.walk(func(paths Paths) {
		list = append(list, paths...)
	})
	list = FirstUniquePaths(list)
	if d.reverse {
		reversePathsInPlace(list)
	}
	return list
}

// ToSortedList returns the direct and transitive contents of a DepSet in lexically sorted order
// with duplicates removed.
func (d *DepSet) ToSortedList() Paths {
	list := d.ToList()
	return SortedUniquePaths(list)
}

func reversePathsInPlace(list Paths) {
	for i, j := 0, len(list)-1; i < j; i, j = i+1, j-1 {
		list[i], list[j] = list[j], list[i]
	}
}

func reverseDepSets(list []*DepSet) []*DepSet {
	ret := make([]*DepSet, len(list))
	for i := range list {
		ret[i] = list[len(list)-1-i]
	}
	return ret
}
