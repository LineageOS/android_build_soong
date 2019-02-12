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
	values     sync.Map
	valuesLock sync.Mutex
}

// Once computes a value the first time it is called with a given key per OncePer, and returns the
// value without recomputing when called with the same key.  key must be hashable.
func (once *OncePer) Once(key OnceKey, value func() interface{}) interface{} {
	// Fast path: check if the key is already in the map
	if v, ok := once.values.Load(key); ok {
		return v
	}

	// Slow path: lock so that we don't call the value function twice concurrently
	once.valuesLock.Lock()
	defer once.valuesLock.Unlock()

	// Check again with the lock held
	if v, ok := once.values.Load(key); ok {
		return v
	}

	// Still not in the map, call the value function and store it
	v := value()
	once.values.Store(key, v)

	return v
}

// Get returns the value previously computed with Once for a given key.  If Once has not been called for the given
// key Get will panic.
func (once *OncePer) Get(key OnceKey) interface{} {
	v, ok := once.values.Load(key)
	if !ok {
		panic(fmt.Errorf("Get() called before Once()"))
	}

	return v
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
