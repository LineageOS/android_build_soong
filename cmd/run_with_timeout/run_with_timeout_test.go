// Copyright 2021 Google Inc. All rights reserved.
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

package main

import (
	"bytes"
	"io"
	"testing"
	"time"
)

func Test_runWithTimeout(t *testing.T) {
	type args struct {
		command      string
		args         []string
		timeout      time.Duration
		onTimeoutCmd string
		stdin        io.Reader
	}
	tests := []struct {
		name       string
		args       args
		wantStdout string
		wantStderr string
		wantErr    bool
	}{
		{
			name: "no timeout",
			args: args{
				command: "echo",
				args:    []string{"foo"},
			},
			wantStdout: "foo\n",
		},
		{
			name: "timeout not reached",
			args: args{
				command: "echo",
				args:    []string{"foo"},
				timeout: 1 * time.Second,
			},
			wantStdout: "foo\n",
		},
		{
			name: "timed out",
			args: args{
				command: "sh",
				args:    []string{"-c", "sleep 1 && echo foo"},
				timeout: 1 * time.Millisecond,
			},
			wantErr: true,
		},
		{
			name: "on_timeout command",
			args: args{
				command:      "sh",
				args:         []string{"-c", "sleep 1 && echo foo"},
				timeout:      1 * time.Millisecond,
				onTimeoutCmd: "echo bar",
			},
			wantStdout: "bar\n",
			wantErr:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}
			err := runWithTimeout(tt.args.command, tt.args.args, tt.args.timeout, tt.args.onTimeoutCmd, tt.args.stdin, stdout, stderr)
			if (err != nil) != tt.wantErr {
				t.Errorf("runWithTimeout() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotStdout := stdout.String(); gotStdout != tt.wantStdout {
				t.Errorf("runWithTimeout() gotStdout = %v, want %v", gotStdout, tt.wantStdout)
			}
			if gotStderr := stderr.String(); gotStderr != tt.wantStderr {
				t.Errorf("runWithTimeout() gotStderr = %v, want %v", gotStderr, tt.wantStderr)
			}
		})
	}
}
