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
	"reflect"
)

// depSet is designed to be conceptually compatible with Bazel's depsets:
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

// A depSet efficiently stores a slice of an arbitrary type from transitive dependencies without
// copying. It is stored as a DAG of depSet nodes, each of which has some direct contents and a list
// of dependency depSet nodes.
//
// A depSet has an order that will be used to walk the DAG when ToList() is called.  The order
// can be POSTORDER, PREORDER, or TOPOLOGICAL.  POSTORDER and PREORDER orders return a postordered
// or preordered left to right flattened list.  TOPOLOGICAL returns a list that guarantees that
// elements of children are listed after all of their parents (unless there are duplicate direct
// elements in the depSet or any of its transitive dependencies, in which case the ordering of the
// duplicated element is not guaranteed).
//
// A depSet is created by newDepSet or newDepSetBuilder.Build from the slice for direct contents
// and the *depSets of dependencies. A depSet is immutable once created.
//
// This object uses reflection to remain agnostic to the type it contains.  It should be replaced
// with generics once those exist in Go.  Callers should generally use a thin wrapper around depSet
// that provides type-safe methods like DepSet for Paths.
type depSet struct {
	preorder   bool
	reverse    bool
	order      DepSetOrder
	direct     interface{}
	transitive []*depSet
}

type depSetInterface interface {
	embeddedDepSet() *depSet
}

func (d *depSet) embeddedDepSet() *depSet {
	return d
}

var _ depSetInterface = (*depSet)(nil)

// newDepSet returns an immutable depSet with the given order, direct and transitive contents.
// direct must be a slice, but is not type-safe due to the lack of generics in Go.  It can be a
// nil slice, but not a nil interface{}, i.e. []string(nil) but not nil.
func newDepSet(order DepSetOrder, direct interface{}, transitive interface{}) *depSet {
	var directCopy interface{}
	transitiveDepSet := sliceToDepSets(transitive, order)

	if order == TOPOLOGICAL {
		directCopy = reverseSlice(direct)
		reverseSliceInPlace(transitiveDepSet)
	} else {
		directCopy = copySlice(direct)
	}

	return &depSet{
		preorder:   order == PREORDER,
		reverse:    order == TOPOLOGICAL,
		order:      order,
		direct:     directCopy,
		transitive: transitiveDepSet,
	}
}

// depSetBuilder is used to create an immutable depSet.
type depSetBuilder struct {
	order      DepSetOrder
	direct     reflect.Value
	transitive []*depSet
}

// newDepSetBuilder returns a depSetBuilder to create an immutable depSet with the given order and
// type, represented by a slice of type that will be in the depSet.
func newDepSetBuilder(order DepSetOrder, typ interface{}) *depSetBuilder {
	empty := reflect.Zero(reflect.TypeOf(typ))
	return &depSetBuilder{
		order:  order,
		direct: empty,
	}
}

// sliceToDepSets converts a slice of any type that implements depSetInterface (by having a depSet
// embedded in it) into a []*depSet.
func sliceToDepSets(in interface{}, order DepSetOrder) []*depSet {
	slice := reflect.ValueOf(in)
	length := slice.Len()
	out := make([]*depSet, length)
	for i := 0; i < length; i++ {
		vi := slice.Index(i)
		depSetIntf, ok := vi.Interface().(depSetInterface)
		if !ok {
			panic(fmt.Errorf("element %d is a %s, not a depSetInterface", i, vi.Type()))
		}
		depSet := depSetIntf.embeddedDepSet()
		if depSet.order != order {
			panic(fmt.Errorf("incompatible order, new depSet is %s but transitive depSet is %s",
				order, depSet.order))
		}
		out[i] = depSet
	}
	return out
}

// DirectSlice adds direct contents to the depSet being built by a depSetBuilder. Newly added direct
// contents are to the right of any existing direct contents.  The argument must be a slice, but
// is not type-safe due to the lack of generics in Go.
func (b *depSetBuilder) DirectSlice(direct interface{}) *depSetBuilder {
	b.direct = reflect.AppendSlice(b.direct, reflect.ValueOf(direct))
	return b
}

// Direct adds direct contents to the depSet being built by a depSetBuilder. Newly added direct
// contents are to the right of any existing direct contents.  The argument must be the same type
// as the element of the slice passed to newDepSetBuilder, but is not type-safe due to the lack of
// generics in Go.
func (b *depSetBuilder) Direct(direct interface{}) *depSetBuilder {
	b.direct = reflect.Append(b.direct, reflect.ValueOf(direct))
	return b
}

// Transitive adds transitive contents to the DepSet being built by a DepSetBuilder. Newly added
// transitive contents are to the right of any existing transitive contents.  The argument can
// be any slice of type that has depSet embedded in it.
func (b *depSetBuilder) Transitive(transitive interface{}) *depSetBuilder {
	depSets := sliceToDepSets(transitive, b.order)
	b.transitive = append(b.transitive, depSets...)
	return b
}

// Returns the depSet being built by this depSetBuilder.  The depSetBuilder retains its contents
// for creating more depSets.
func (b *depSetBuilder) Build() *depSet {
	return newDepSet(b.order, b.direct.Interface(), b.transitive)
}

// walk calls the visit method in depth-first order on a DepSet, preordered if d.preorder is set,
// otherwise postordered.
func (d *depSet) walk(visit func(interface{})) {
	visited := make(map[*depSet]bool)

	var dfs func(d *depSet)
	dfs = func(d *depSet) {
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

// ToList returns the depSet flattened to a list.  The order in the list is based on the order
// of the depSet.  POSTORDER and PREORDER orders return a postordered or preordered left to right
// flattened list.  TOPOLOGICAL returns a list that guarantees that elements of children are listed
// after all of their parents (unless there are duplicate direct elements in the DepSet or any of
// its transitive dependencies, in which case the ordering of the duplicated element is not
// guaranteed).
//
// This method uses a reflection-based implementation to find the unique elements in slice, which
// is around 3x slower than a concrete implementation.  Type-safe wrappers around depSet can
// provide their own implementation of ToList that calls depSet.toList with a method that
// uses a concrete implementation.
func (d *depSet) ToList() interface{} {
	return d.toList(firstUnique)
}

// toList returns the depSet flattened to a list.  The order in the list is based on the order
// of the depSet.  POSTORDER and PREORDER orders return a postordered or preordered left to right
// flattened list.  TOPOLOGICAL returns a list that guarantees that elements of children are listed
// after all of their parents (unless there are duplicate direct elements in the DepSet or any of
// its transitive dependencies, in which case the ordering of the duplicated element is not
// guaranteed).  The firstUniqueFunc is used to remove duplicates from the list.
func (d *depSet) toList(firstUniqueFunc func(interface{}) interface{}) interface{} {
	if d == nil {
		return nil
	}
	slice := reflect.Zero(reflect.TypeOf(d.direct))
	d.walk(func(paths interface{}) {
		slice = reflect.AppendSlice(slice, reflect.ValueOf(paths))
	})
	list := slice.Interface()
	list = firstUniqueFunc(list)
	if d.reverse {
		reverseSliceInPlace(list)
	}
	return list
}

// firstUnique returns all unique elements of a slice, keeping the first copy of each.  It
// modifies the slice contents in place, and returns a subslice of the original slice.  The
// argument must be a slice, but is not type-safe due to the lack of reflection in Go.
//
// Performance of the reflection-based firstUnique is up to 3x slower than a concrete type
// version such as FirstUniqueStrings.
func firstUnique(slice interface{}) interface{} {
	// 4 was chosen based on Benchmark_firstUnique results.
	if reflect.ValueOf(slice).Len() > 4 {
		return firstUniqueMap(slice)
	}
	return firstUniqueList(slice)
}

// firstUniqueList is an implementation of firstUnique using an O(N^2) list comparison to look for
// duplicates.
func firstUniqueList(in interface{}) interface{} {
	writeIndex := 0
	slice := reflect.ValueOf(in)
	length := slice.Len()
outer:
	for readIndex := 0; readIndex < length; readIndex++ {
		readValue := slice.Index(readIndex)
		for compareIndex := 0; compareIndex < writeIndex; compareIndex++ {
			compareValue := slice.Index(compareIndex)
			// These two Interface() calls seem to cause an allocation and significantly
			// slow down this list-based implementation.  The map implementation below doesn't
			// have this issue because reflect.Value.MapIndex takes a Value and appears to be
			// able to do the map lookup without an allocation.
			if readValue.Interface() == compareValue.Interface() {
				// The value at readIndex already exists somewhere in the output region
				// of the slice before writeIndex, skip it.
				continue outer
			}
		}
		if readIndex != writeIndex {
			writeValue := slice.Index(writeIndex)
			writeValue.Set(readValue)
		}
		writeIndex++
	}
	return slice.Slice(0, writeIndex).Interface()
}

var trueValue = reflect.ValueOf(true)

// firstUniqueList is an implementation of firstUnique using an O(N) hash set lookup to look for
// duplicates.
func firstUniqueMap(in interface{}) interface{} {
	writeIndex := 0
	slice := reflect.ValueOf(in)
	length := slice.Len()
	seen := reflect.MakeMapWithSize(reflect.MapOf(slice.Type().Elem(), trueValue.Type()), slice.Len())
	for readIndex := 0; readIndex < length; readIndex++ {
		readValue := slice.Index(readIndex)
		if seen.MapIndex(readValue).IsValid() {
			continue
		}
		seen.SetMapIndex(readValue, trueValue)
		if readIndex != writeIndex {
			writeValue := slice.Index(writeIndex)
			writeValue.Set(readValue)
		}
		writeIndex++
	}
	return slice.Slice(0, writeIndex).Interface()
}

// reverseSliceInPlace reverses the elements of a slice in place.  The argument must be a slice, but
// is not type-safe due to the lack of reflection in Go.
func reverseSliceInPlace(in interface{}) {
	swapper := reflect.Swapper(in)
	slice := reflect.ValueOf(in)
	length := slice.Len()
	for i, j := 0, length-1; i < j; i, j = i+1, j-1 {
		swapper(i, j)
	}
}

// reverseSlice returns a copy of a slice in reverse order.  The argument must be a slice, but is
// not type-safe due to the lack of reflection in Go.
func reverseSlice(in interface{}) interface{} {
	slice := reflect.ValueOf(in)
	if !slice.IsValid() || slice.IsNil() {
		return in
	}
	if slice.Kind() != reflect.Slice {
		panic(fmt.Errorf("%t is not a slice", in))
	}
	length := slice.Len()
	if length == 0 {
		return in
	}
	out := reflect.MakeSlice(slice.Type(), length, length)
	for i := 0; i < length; i++ {
		out.Index(i).Set(slice.Index(length - 1 - i))
	}
	return out.Interface()
}

// copySlice returns a copy of a slice.  The argument must be a slice, but is not type-safe due to
// the lack of reflection in Go.
func copySlice(in interface{}) interface{} {
	slice := reflect.ValueOf(in)
	if !slice.IsValid() || slice.IsNil() {
		return in
	}
	length := slice.Len()
	if length == 0 {
		return in
	}
	out := reflect.MakeSlice(slice.Type(), length, length)
	reflect.Copy(out, slice)
	return out.Interface()
}
