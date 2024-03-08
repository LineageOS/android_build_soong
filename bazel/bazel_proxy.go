// Copyright 2023 Google Inc. All rights reserved.
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

package bazel

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"net"
	os_lib "os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Logs events of ProxyServer.
type ServerLogger interface {
	Fatal(v ...interface{})
	Fatalf(format string, v ...interface{})
	Println(v ...interface{})
}

// CmdRequest is a request to the Bazel Proxy server.
type CmdRequest struct {
	// Args to the Bazel command.
	Argv []string
	// Environment variables to pass to the Bazel invocation. Strings should be of
	// the form "KEY=VALUE".
	Env []string
}

// CmdResponse is a response from the Bazel Proxy server.
type CmdResponse struct {
	Stdout      string
	Stderr      string
	ErrorString string
}

// ProxyClient is a client which can issue Bazel commands to the Bazel
// proxy server. Requests are issued (and responses received) via a unix socket.
// See ProxyServer for more details.
type ProxyClient struct {
	outDir string
}

// ProxyServer is a server which runs as a background goroutine. Each
// request to the server describes a Bazel command which the server should run.
// The server then issues the Bazel command, and returns a response describing
// the stdout/stderr of the command.
// Client-server communication is done via a unix socket under the output
// directory.
// The server is intended to circumvent sandboxing for subprocesses of the
// build. The build orchestrator (soong_ui) can launch a server to exist outside
// of sandboxing, and sandboxed processes (such as soong_build) can issue
// bazel commands through this socket tunnel. This allows a sandboxed process
// to issue bazel requests to a bazel that resides outside of sandbox. This
// is particularly useful to maintain a persistent Bazel server which lives
// past the duration of a single build.
// The ProxyServer will only live as long as soong_ui does; the
// underlying Bazel server will live past the duration of the build.
type ProxyServer struct {
	logger          ServerLogger
	outDir          string
	workspaceDir    string
	bazeliskVersion string
	// The server goroutine will listen on this channel and stop handling requests
	// once it is written to.
	done chan struct{}
}

// NewProxyClient is a constructor for a ProxyClient.
func NewProxyClient(outDir string) *ProxyClient {
	return &ProxyClient{
		outDir: outDir,
	}
}

func unixSocketPath(outDir string) string {
	return filepath.Join(outDir, "bazelsocket.sock")
}

// IssueCommand issues a request to the Bazel Proxy Server to issue a Bazel
// request. Returns a response describing the output from the Bazel process
// (if the Bazel process had an error, then the response will include an error).
// Returns an error if there was an issue with the connection to the Bazel Proxy
// server.
func (b *ProxyClient) IssueCommand(req CmdRequest) (CmdResponse, error) {
	var resp CmdResponse
	var err error
	// Check for connections every 1 second. This is chosen to be a relatively
	// short timeout, because the proxy server should accept requests quite
	// quickly.
	d := net.Dialer{Timeout: 1 * time.Second}
	var conn net.Conn
	conn, err = d.Dial("unix", unixSocketPath(b.outDir))
	if err != nil {
		return resp, err
	}
	defer conn.Close()

	enc := gob.NewEncoder(conn)
	if err = enc.Encode(req); err != nil {
		return resp, err
	}
	dec := gob.NewDecoder(conn)
	err = dec.Decode(&resp)
	return resp, err
}

// NewProxyServer is a constructor for a ProxyServer.
func NewProxyServer(logger ServerLogger, outDir string, workspaceDir string, bazeliskVersion string) *ProxyServer {
	if len(bazeliskVersion) > 0 {
		logger.Println("** Using Bazelisk for this build, due to env var USE_BAZEL_VERSION=" + bazeliskVersion + " **")
	}

	return &ProxyServer{
		logger:          logger,
		outDir:          outDir,
		workspaceDir:    workspaceDir,
		done:            make(chan struct{}),
		bazeliskVersion: bazeliskVersion,
	}
}

func ExecBazel(bazelPath string, workspaceDir string, request CmdRequest) (stdout []byte, stderr []byte, cmdErr error) {
	bazelCmd := exec.Command(bazelPath, request.Argv...)
	bazelCmd.Dir = workspaceDir
	bazelCmd.Env = request.Env

	stderrBuffer := &bytes.Buffer{}
	bazelCmd.Stderr = stderrBuffer

	if output, err := bazelCmd.Output(); err != nil {
		cmdErr = fmt.Errorf("bazel command failed: %s\n---command---\n%s\n---env---\n%s\n---stderr---\n%s---",
			err, bazelCmd, strings.Join(bazelCmd.Env, "\n"), stderrBuffer)
	} else {
		stdout = output
	}
	stderr = stderrBuffer.Bytes()
	return
}

func (b *ProxyServer) handleRequest(conn net.Conn) error {
	defer conn.Close()

	dec := gob.NewDecoder(conn)
	var req CmdRequest
	if err := dec.Decode(&req); err != nil {
		return fmt.Errorf("Error decoding request: %s", err)
	}

	if len(b.bazeliskVersion) > 0 {
		req.Env = append(req.Env, "USE_BAZEL_VERSION="+b.bazeliskVersion)
	}
	stdout, stderr, cmdErr := ExecBazel("./build/bazel/bin/bazel", b.workspaceDir, req)
	errorString := ""
	if cmdErr != nil {
		errorString = cmdErr.Error()
	}

	resp := CmdResponse{string(stdout), string(stderr), errorString}
	enc := gob.NewEncoder(conn)
	if err := enc.Encode(&resp); err != nil {
		return fmt.Errorf("Error encoding response: %s", err)
	}
	return nil
}

func (b *ProxyServer) listenUntilClosed(listener net.Listener) error {
	for {
		// Check for connections every 1 second. This is a blocking operation, so
		// if the server is closed, the goroutine will not fully close until this
		// deadline is reached. Thus, this deadline is short (but not too short
		// so that the routine churns).
		listener.(*net.UnixListener).SetDeadline(time.Now().Add(time.Second))
		conn, err := listener.Accept()

		select {
		case <-b.done:
			return nil
		default:
		}

		if err != nil {
			if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() {
				// Timeout is normal and expected while waiting for client to establish
				// a connection.
				continue
			} else {
				b.logger.Fatalf("Listener error: %s", err)
			}
		}

		err = b.handleRequest(conn)
		if err != nil {
			b.logger.Fatal(err)
		}
	}
}

// Start initializes the server unix socket and (in a separate goroutine)
// handles requests on the socket until the server is closed. Returns an error
// if a failure occurs during initialization. Will log any post-initialization
// errors to the server's logger.
func (b *ProxyServer) Start() error {
	unixSocketAddr := unixSocketPath(b.outDir)
	if err := os_lib.RemoveAll(unixSocketAddr); err != nil {
		return fmt.Errorf("couldn't remove socket '%s': %s", unixSocketAddr, err)
	}
	listener, err := net.Listen("unix", unixSocketAddr)

	if err != nil {
		return fmt.Errorf("error listening on socket '%s': %s", unixSocketAddr, err)
	}

	go b.listenUntilClosed(listener)
	return nil
}

// Close shuts down the server. This will stop the server from listening for
// additional requests.
func (b *ProxyServer) Close() {
	b.done <- struct{}{}
}
