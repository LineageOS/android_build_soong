// Copyright 2019 Google Inc. All rights reserved.
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
	"testing"
	"time"
)

func TestOncePer_Once(t *testing.T) {
	once := OncePer{}
	key := NewOnceKey("key")

	a := once.Once(key, func() interface{} { return "a" }).(string)
	b := once.Once(key, func() interface{} { return "b" }).(string)

	if a != "a" {
		t.Errorf(`first call to Once should return "a": %q`, a)
	}

	if b != "a" {
		t.Errorf(`second call to Once with the same key should return "a": %q`, b)
	}
}

func TestOncePer_Once_wait(t *testing.T) {
	once := OncePer{}
	key := NewOnceKey("key")

	ch := make(chan bool)

	go once.Once(key, func() interface{} { close(ch); time.Sleep(100 * time.Millisecond); return "foo" })
	<-ch
	a := once.Once(key, func() interface{} { return "bar" }).(string)

	if a != "foo" {
		t.Errorf("expect %q, got %q", "foo", a)
	}
}

func TestOncePer_Get(t *testing.T) {
	once := OncePer{}
	key := NewOnceKey("key")

	a := once.Once(key, func() interface{} { return "a" }).(string)
	b := once.Get(key).(string)

	if a != "a" {
		t.Errorf(`first call to Once should return "a": %q`, a)
	}

	if b != "a" {
		t.Errorf(`Get with the same key should return "a": %q`, b)
	}
}

func TestOncePer_Get_panic(t *testing.T) {
	once := OncePer{}
	key := NewOnceKey("key")

	defer func() {
		p := recover()

		if p == nil {
			t.Error("call to Get for unused key should panic")
		}
	}()

	once.Get(key)
}

func TestOncePer_Get_wait(t *testing.T) {
	once := OncePer{}
	key := NewOnceKey("key")

	ch := make(chan bool)

	go once.Once(key, func() interface{} { close(ch); time.Sleep(100 * time.Millisecond); return "foo" })
	<-ch
	a := once.Get(key).(string)

	if a != "foo" {
		t.Errorf("expect %q, got %q", "foo", a)
	}
}

func TestOncePer_OnceStringSlice(t *testing.T) {
	once := OncePer{}
	key := NewOnceKey("key")

	a := once.OnceStringSlice(key, func() []string { return []string{"a"} })
	b := once.OnceStringSlice(key, func() []string { return []string{"a"} })

	if a[0] != "a" {
		t.Errorf(`first call to OnceStringSlice should return ["a"]: %q`, a)
	}

	if b[0] != "a" {
		t.Errorf(`second call to OnceStringSlice with the same key should return ["a"]: %q`, b)
	}
}

func TestOncePer_Once2StringSlice(t *testing.T) {
	once := OncePer{}
	key := NewOnceKey("key")

	a, b := once.Once2StringSlice(key, func() ([]string, []string) { return []string{"a"}, []string{"b"} })
	c, d := once.Once2StringSlice(key, func() ([]string, []string) { return []string{"c"}, []string{"d"} })

	if a[0] != "a" || b[0] != "b" {
		t.Errorf(`first call to Once2StringSlice should return ["a"], ["b"]: %q, %q`, a, b)
	}

	if c[0] != "a" || d[0] != "b" {
		t.Errorf(`second call to Once2StringSlice with the same key should return ["a"], ["b"]: %q, %q`, c, d)
	}
}

func TestNewOnceKey(t *testing.T) {
	once := OncePer{}
	key1 := NewOnceKey("key")
	key2 := NewOnceKey("key")

	a := once.Once(key1, func() interface{} { return "a" }).(string)
	b := once.Once(key2, func() interface{} { return "b" }).(string)

	if a != "a" {
		t.Errorf(`first call to Once should return "a": %q`, a)
	}

	if b != "b" {
		t.Errorf(`second call to Once with the NewOnceKey from same string should return "b": %q`, b)
	}
}

func TestNewCustomOnceKey(t *testing.T) {
	type key struct {
		key string
	}
	once := OncePer{}
	key1 := NewCustomOnceKey(key{"key"})
	key2 := NewCustomOnceKey(key{"key"})

	a := once.Once(key1, func() interface{} { return "a" }).(string)
	b := once.Once(key2, func() interface{} { return "b" }).(string)

	if a != "a" {
		t.Errorf(`first call to Once should return "a": %q`, a)
	}

	if b != "a" {
		t.Errorf(`second call to Once with the NewCustomOnceKey from equal key should return "a": %q`, b)
	}
}

func TestOncePerReentrant(t *testing.T) {
	once := OncePer{}
	key1 := NewOnceKey("key")
	key2 := NewOnceKey("key")

	a := once.Once(key1, func() interface{} { return once.Once(key2, func() interface{} { return "a" }) })
	if a != "a" {
		t.Errorf(`reentrant Once should return "a": %q`, a)
	}
}

// Test that a recovered panic in a Once function doesn't deadlock
func TestOncePerPanic(t *testing.T) {
	once := OncePer{}
	key := NewOnceKey("key")

	ch := make(chan interface{})

	var a interface{}

	go func() {
		defer func() {
			ch <- recover()
		}()

		a = once.Once(key, func() interface{} {
			panic("foo")
		})
	}()

	p := <-ch

	if p.(string) != "foo" {
		t.Errorf(`expected panic with "foo", got %#v`, p)
	}

	if a != nil {
		t.Errorf(`expected a to be nil, got %#v`, a)
	}

	// If the call to Once that panicked leaves the key in a bad state this will deadlock
	b := once.Once(key, func() interface{} {
		return "bar"
	})

	// The second call to Once should return nil inserted by the first call that panicked.
	if b != nil {
		t.Errorf(`expected b to be nil, got %#v`, b)
	}
}
