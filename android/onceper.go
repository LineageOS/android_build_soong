// Copyright 2016 Google Inc. All rights reserved.
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
	"sync"
)

type OncePer struct {
	values sync.Map
}

type onceValueWaiter chan bool

func (once *OncePer) maybeWaitFor(key OnceKey, value interface{}) interface{} {
	if wait, isWaiter := value.(onceValueWaiter); isWaiter {
		// The entry in the map is a placeholder waiter because something else is constructing the value
		// wait until the waiter is signalled, then load the real value.
		<-wait
		value, _ = once.values.Load(key)
		if _, isWaiter := value.(onceValueWaiter); isWaiter {
			panic(fmt.Errorf("Once() waiter completed but key is still not valid"))
		}
	}

	return value
}

// Once computes a value the first time it is called with a given key per OncePer, and returns the
// value without recomputing when called with the same key.  key must be hashable.  If value panics
// the panic will be propagated but the next call to Once with the same key will return nil.
func (once *OncePer) Once(key OnceKey, value func() interface{}) interface{} {
	// Fast path: check if the key is already in the map
	if v, ok := once.values.Load(key); ok {
		return once.maybeWaitFor(key, v)
	}

	// Slow path: create a OnceValueWrapper and attempt to insert it
	waiter := make(onceValueWaiter)
	if v, loaded := once.values.LoadOrStore(key, waiter); loaded {
		// Got a value, something else inserted its own waiter or a constructed value
		return once.maybeWaitFor(key, v)
	}

	// The waiter is inserted, call the value constructor, store it, and signal the waiter.  Use defer in case
	// the function panics.
	var v interface{}
	defer func() {
		once.values.Store(key, v)
		close(waiter)
	}()

	v = value()

	return v
}

// Get returns the value previously computed with Once for a given key.  If Once has not been called for the given
// key Get will panic.
func (once *OncePer) Get(key OnceKey) interface{} {
	v, ok := once.values.Load(key)
	if !ok {
		panic(fmt.Errorf("Get() called before Once()"))
	}

	return once.maybeWaitFor(key, v)
}

// OnceStringSlice is the same as Once, but returns the value cast to a []string
func (once *OncePer) OnceStringSlice(key OnceKey, value func() []string) []string {
	return once.Once(key, func() interface{} { return value() }).([]string)
}

// OnceStringSlice is the same as Once, but returns two values cast to []string
func (once *OncePer) Once2StringSlice(key OnceKey, value func() ([]string, []string)) ([]string, []string) {
	type twoStringSlice [2][]string
	s := once.Once(key, func() interface{} {
		var s twoStringSlice
		s[0], s[1] = value()
		return s
	}).(twoStringSlice)
	return s[0], s[1]
}

// OncePath is the same as Once, but returns the value cast to a Path
func (once *OncePer) OncePath(key OnceKey, value func() Path) Path {
	return once.Once(key, func() interface{} { return value() }).(Path)
}

// OncePath is the same as Once, but returns the value cast to a SourcePath
func (once *OncePer) OnceSourcePath(key OnceKey, value func() SourcePath) SourcePath {
	return once.Once(key, func() interface{} { return value() }).(SourcePath)
}

// OnceKey is an opaque type to be used as the key in calls to Once.
type OnceKey struct {
	key interface{}
}

// NewOnceKey returns an opaque OnceKey object for the provided key.  Two calls to NewOnceKey with the same key string
// DO NOT produce the same OnceKey object.
func NewOnceKey(key string) OnceKey {
	return OnceKey{&key}
}

// NewCustomOnceKey returns an opaque OnceKey object for the provided key.  The key can be any type that is valid as the
// key in a map, i.e. comparable.  Two calls to NewCustomOnceKey with key values that compare equal will return OnceKey
// objects that access the same value stored with Once.
func NewCustomOnceKey(key interface{}) OnceKey {
	return OnceKey{key}
}
