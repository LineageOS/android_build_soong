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
	"net"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
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
	ctx, _ := context.WithTimeout(context.Background(), 2*timeoutDuration)

	recv, err := logListener(ctx, socket, lookup)
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		for i := 0; i < 10; i++ {
			sendLog(socket, lookup, &LogEntry{
				Basename: "test",
				Args:     []string{"foo", "bar"},
			}, make(chan interface{}))
		}
	}()

	count := 0
	for {
		select {
		case entry := <-recv:
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
		case <-ctx.Done():
			t.Error("Hit timeout before receiving all logs")
		}
	}
}

func TestSendLogError(t *testing.T) {
	d, err := ioutil.TempDir("", "log_socket")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(d)

	t.Run("Missing file", func(t *testing.T) {
		start := time.Now()
		SendLog(filepath.Join(d, "missing"), &LogEntry{}, make(chan interface{}))
		elapsed := time.Since(start)
		if elapsed > timeoutDuration {
			t.Errorf("Should have been << timeout (%s), but was %s", timeoutDuration, elapsed)
		}
	})

	t.Run("Regular file", func(t *testing.T) {
		f := filepath.Join(d, "file")
		if fp, err := os.Create(f); err == nil {
			fp.Close()
		} else {
			t.Fatal(err)
		}

		start := time.Now()
		SendLog(f, &LogEntry{}, make(chan interface{}))
		elapsed := time.Since(start)
		if elapsed > timeoutDuration {
			t.Errorf("Should have been << timeout (%s), but was %s", timeoutDuration, elapsed)
		}
	})

	t.Run("Reader not reading", func(t *testing.T) {
		f := filepath.Join(d, "sock1")

		ln, err := net.Listen("unix", f)
		if err != nil {
			t.Fatal(err)
		}
		defer ln.Close()

		done := make(chan bool, 1)
		go func() {
			for i := 0; i < 10; i++ {
				SendLog(f, &LogEntry{
					// Ensure a relatively large payload
					Basename: strings.Repeat(" ", 100000),
				}, make(chan interface{}))
			}
			done <- true
		}()

		select {
		case <-done:
			break
		case <-time.After(12 * timeoutDuration):
			t.Error("Should have finished")
		}
	})
}
