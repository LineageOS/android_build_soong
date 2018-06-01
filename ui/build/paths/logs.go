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
	"encoding/gob"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"
)

type LogProcess struct {
	Pid     int
	Command string
}

type LogEntry struct {
	Basename string
	Args     []string
	Parents  []LogProcess
}

const timeoutDuration = time.Duration(100) * time.Millisecond

type socketAddrFunc func(string) (string, func(), error)

func procFallback(name string) (string, func(), error) {
	d, err := os.Open(filepath.Dir(name))
	if err != nil {
		return "", func() {}, err
	}

	return fmt.Sprintf("/proc/self/fd/%d/%s", d.Fd(), filepath.Base(name)), func() {
		d.Close()
	}, nil
}

func tmpFallback(name string) (addr string, cleanup func(), err error) {
	d, err := ioutil.TempDir("/tmp", "log_sock")
	if err != nil {
		cleanup = func() {}
		return
	}
	cleanup = func() {
		os.RemoveAll(d)
	}

	dir := filepath.Dir(name)

	absDir, err := filepath.Abs(dir)
	if err != nil {
		return
	}

	err = os.Symlink(absDir, filepath.Join(d, "d"))
	if err != nil {
		return
	}

	addr = filepath.Join(d, "d", filepath.Base(name))

	return
}

func getSocketAddr(name string) (string, func(), error) {
	maxNameLen := len(syscall.RawSockaddrUnix{}.Path)

	if len(name) < maxNameLen {
		return name, func() {}, nil
	}

	if runtime.GOOS == "linux" {
		addr, cleanup, err := procFallback(name)
		if err == nil {
			if len(addr) < maxNameLen {
				return addr, cleanup, nil
			}
		}
		cleanup()
	}

	addr, cleanup, err := tmpFallback(name)
	if err == nil {
		if len(addr) < maxNameLen {
			return addr, cleanup, nil
		}
	}
	cleanup()

	return name, func() {}, fmt.Errorf("Path to socket is still over size limit, fallbacks failed.")
}

func dial(name string, lookup socketAddrFunc, timeout time.Duration) (net.Conn, error) {
	socket, cleanup, err := lookup(name)
	defer cleanup()
	if err != nil {
		return nil, err
	}

	dialer := &net.Dialer{
		Timeout: timeout,
	}
	return dialer.Dial("unix", socket)
}

func listen(name string, lookup socketAddrFunc) (net.Listener, error) {
	socket, cleanup, err := lookup(name)
	defer cleanup()
	if err != nil {
		return nil, err
	}

	return net.Listen("unix", socket)
}

func SendLog(logSocket string, entry *LogEntry, done chan interface{}) {
	sendLog(logSocket, getSocketAddr, timeoutDuration, entry, done)
}

func sendLog(logSocket string, lookup socketAddrFunc, timeout time.Duration, entry *LogEntry, done chan interface{}) {
	defer close(done)

	conn, err := dial(logSocket, lookup, timeout)
	if err != nil {
		return
	}
	defer conn.Close()

	if timeout != 0 {
		conn.SetDeadline(time.Now().Add(timeout))
	}

	enc := gob.NewEncoder(conn)
	enc.Encode(entry)
}

func LogListener(ctx context.Context, logSocket string) (chan *LogEntry, error) {
	return logListener(ctx, logSocket, getSocketAddr)
}

func logListener(ctx context.Context, logSocket string, lookup socketAddrFunc) (chan *LogEntry, error) {
	ret := make(chan *LogEntry, 5)

	if err := os.Remove(logSocket); err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	ln, err := listen(logSocket, lookup)
	if err != nil {
		return nil, err
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				ln.Close()
			}
		}
	}()

	go func() {
		var wg sync.WaitGroup
		defer func() {
			wg.Wait()
			close(ret)
		}()

		for {
			conn, err := ln.Accept()
			if err != nil {
				ln.Close()
				break
			}
			conn.SetDeadline(time.Now().Add(timeoutDuration))
			wg.Add(1)

			go func() {
				defer wg.Done()
				defer conn.Close()

				dec := gob.NewDecoder(conn)
				entry := &LogEntry{}
				if err := dec.Decode(entry); err != nil {
					return
				}
				ret <- entry
			}()
		}
	}()
	return ret, nil
}
