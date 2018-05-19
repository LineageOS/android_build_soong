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
	"net"
	"os"
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

const timeoutDuration = time.Duration(250) * time.Millisecond

func SendLog(logSocket string, entry *LogEntry, done chan interface{}) {
	defer close(done)

	dialer := &net.Dialer{}
	conn, err := dialer.Dial("unix", logSocket)
	if err != nil {
		return
	}
	defer conn.Close()

	if err := conn.SetDeadline(dialer.Deadline); err != nil {
		return
	}

	enc := gob.NewEncoder(conn)
	enc.Encode(entry)
}

func LogListener(ctx context.Context, logSocket string) (chan *LogEntry, error) {
	ret := make(chan *LogEntry, 5)

	if err := os.Remove(logSocket); err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	ln, err := net.Listen("unix", logSocket)
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
		defer close(ret)

		for {
			conn, err := ln.Accept()
			if err != nil {
				ln.Close()
				break
			}
			conn.SetDeadline(time.Now().Add(timeoutDuration))

			go func() {
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
