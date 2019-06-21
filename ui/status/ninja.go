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

package status

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"syscall"
	"time"

	"github.com/golang/protobuf/proto"

	"android/soong/ui/logger"
	"android/soong/ui/status/ninja_frontend"
)

// NewNinjaReader reads the protobuf frontend format from ninja and translates it
// into calls on the ToolStatus API.
func NewNinjaReader(ctx logger.Logger, status ToolStatus, fifo string) *NinjaReader {
	os.Remove(fifo)

	err := syscall.Mkfifo(fifo, 0666)
	if err != nil {
		ctx.Fatalf("Failed to mkfifo(%q): %v", fifo, err)
	}

	n := &NinjaReader{
		status: status,
		fifo:   fifo,
		done:   make(chan bool),
		cancel: make(chan bool),
	}

	go n.run()

	return n
}

type NinjaReader struct {
	status ToolStatus
	fifo   string
	done   chan bool
	cancel chan bool
}

const NINJA_READER_CLOSE_TIMEOUT = 5 * time.Second

// Close waits for NinjaReader to finish reading from the fifo, or 5 seconds.
func (n *NinjaReader) Close() {
	// Signal the goroutine to stop if it is blocking opening the fifo.
	close(n.cancel)

	timeoutCh := time.After(NINJA_READER_CLOSE_TIMEOUT)

	select {
	case <-n.done:
		// Nothing
	case <-timeoutCh:
		n.status.Error(fmt.Sprintf("ninja fifo didn't finish after %s", NINJA_READER_CLOSE_TIMEOUT.String()))
	}

	return
}

func (n *NinjaReader) run() {
	defer close(n.done)

	// Opening the fifo can block forever if ninja never opens the write end, do it in a goroutine so this
	// method can exit on cancel.
	fileCh := make(chan *os.File)
	go func() {
		f, err := os.Open(n.fifo)
		if err != nil {
			n.status.Error(fmt.Sprintf("Failed to open fifo: %v", err))
			close(fileCh)
			return
		}
		fileCh <- f
	}()

	var f *os.File

	select {
	case f = <-fileCh:
		// Nothing
	case <-n.cancel:
		return
	}

	defer f.Close()

	r := bufio.NewReader(f)

	running := map[uint32]*Action{}

	for {
		size, err := readVarInt(r)
		if err != nil {
			if err != io.EOF {
				n.status.Error(fmt.Sprintf("Got error reading from ninja: %s", err))
			}
			return
		}

		buf := make([]byte, size)
		_, err = io.ReadFull(r, buf)
		if err != nil {
			if err == io.EOF {
				n.status.Print(fmt.Sprintf("Missing message of size %d from ninja\n", size))
			} else {
				n.status.Error(fmt.Sprintf("Got error reading from ninja: %s", err))
			}
			return
		}

		msg := &ninja_frontend.Status{}
		err = proto.Unmarshal(buf, msg)
		if err != nil {
			n.status.Print(fmt.Sprintf("Error reading message from ninja: %v", err))
			continue
		}

		// Ignore msg.BuildStarted
		if msg.TotalEdges != nil {
			n.status.SetTotalActions(int(msg.TotalEdges.GetTotalEdges()))
		}
		if msg.EdgeStarted != nil {
			action := &Action{
				Description: msg.EdgeStarted.GetDesc(),
				Outputs:     msg.EdgeStarted.Outputs,
				Inputs:      msg.EdgeStarted.Inputs,
				Command:     msg.EdgeStarted.GetCommand(),
			}
			n.status.StartAction(action)
			running[msg.EdgeStarted.GetId()] = action
		}
		if msg.EdgeFinished != nil {
			if started, ok := running[msg.EdgeFinished.GetId()]; ok {
				delete(running, msg.EdgeFinished.GetId())

				var err error
				exitCode := int(msg.EdgeFinished.GetStatus())
				if exitCode != 0 {
					err = fmt.Errorf("exited with code: %d", exitCode)
				}

				n.status.FinishAction(ActionResult{
					Action: started,
					Output: msg.EdgeFinished.GetOutput(),
					Error:  err,
				})
			}
		}
		if msg.Message != nil {
			message := "ninja: " + msg.Message.GetMessage()
			switch msg.Message.GetLevel() {
			case ninja_frontend.Status_Message_INFO:
				n.status.Status(message)
			case ninja_frontend.Status_Message_WARNING:
				n.status.Print("warning: " + message)
			case ninja_frontend.Status_Message_ERROR:
				n.status.Error(message)
			default:
				n.status.Print(message)
			}
		}
		if msg.BuildFinished != nil {
			n.status.Finish()
		}
	}
}

func readVarInt(r *bufio.Reader) (int, error) {
	ret := 0
	shift := uint(0)

	for {
		b, err := r.ReadByte()
		if err != nil {
			return 0, err
		}

		ret += int(b&0x7f) << (shift * 7)
		if b&0x80 == 0 {
			break
		}
		shift += 1
		if shift > 4 {
			return 0, fmt.Errorf("Expected varint32 length-delimited message")
		}
	}

	return ret, nil
}
