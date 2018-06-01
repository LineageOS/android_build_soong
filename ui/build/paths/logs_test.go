// Copyright 2018 Google Inc. All rights reserved.
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

package paths

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestSendLog(t *testing.T) {
	t.Run("Short name", func(t *testing.T) {
		d, err := ioutil.TempDir("", "s")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(d)
		f := filepath.Join(d, "s")

		testSendLog(t, f, getSocketAddr)
	})

	testLongName := func(t *testing.T, lookup socketAddrFunc) {
		d, err := ioutil.TempDir("", strings.Repeat("s", 150))
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(d)
		f := filepath.Join(d, strings.Repeat("s", 10))

		testSendLog(t, f, lookup)
	}

	// Using a name longer than the ~100 limit of the underlying calls to bind, etc
	t.Run("Long name", func(t *testing.T) {
		testLongName(t, getSocketAddr)
	})

	if runtime.GOOS == "linux" {
		t.Run("Long name proc fallback", func(t *testing.T) {
			testLongName(t, procFallback)
		})
	}

	t.Run("Long name tmp fallback", func(t *testing.T) {
		testLongName(t, tmpFallback)
	})
}

func testSendLog(t *testing.T, socket string, lookup socketAddrFunc) {
	recv, err := logListener(context.Background(), socket, lookup)
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		for i := 0; i < 10; i++ {
			sendLog(socket, lookup, 0, &LogEntry{
				Basename: "test",
				Args:     []string{"foo", "bar"},
			}, make(chan interface{}))
		}
	}()

	count := 0
	for {
		entry := <-recv
		if entry == nil {
			if count != 10 {
				t.Errorf("Expected 10 logs, got %d", count)
			}
			return
		}

		ref := LogEntry{
			Basename: "test",
			Args:     []string{"foo", "bar"},
		}
		if !reflect.DeepEqual(ref, *entry) {
			t.Fatalf("Bad log entry: %v", entry)
		}
		count++

		if count == 10 {
			return
		}
	}
}

func TestSendLogError(t *testing.T) {
	d, err := ioutil.TempDir("", "log_socket")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(d)

	// Missing log sockets should not block waiting for the timeout to elapse
	t.Run("Missing file", func(t *testing.T) {
		sendLog(filepath.Join(d, "missing"), getSocketAddr, 0, &LogEntry{}, make(chan interface{}))
	})

	// Non-sockets should not block waiting for the timeout to elapse
	t.Run("Regular file", func(t *testing.T) {
		f := filepath.Join(d, "file")
		if fp, err := os.Create(f); err == nil {
			fp.Close()
		} else {
			t.Fatal(err)
		}

		sendLog(f, getSocketAddr, 0, &LogEntry{}, make(chan interface{}))
	})

	// If the reader is stuck, we should be able to make progress
	t.Run("Reader not reading", func(t *testing.T) {
		f := filepath.Join(d, "sock1")

		ln, err := listen(f, getSocketAddr)
		if err != nil {
			t.Fatal(err)
		}
		defer ln.Close()

		done := make(chan bool, 1)
		go func() {
			for i := 0; i < 10; i++ {
				sendLog(f, getSocketAddr, timeoutDuration, &LogEntry{
					// Ensure a relatively large payload
					Basename: strings.Repeat(" ", 100000),
				}, make(chan interface{}))
			}
			done <- true
		}()

		<-done
	})
}
