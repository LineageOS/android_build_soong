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

	"github.com/golang/protobuf/proto"

	"android/soong/ui/logger"
	"android/soong/ui/status/ninja_frontend"
)

// NinjaReader reads the protobuf frontend format from ninja and translates it
// into calls on the ToolStatus API.
func NinjaReader(ctx logger.Logger, status ToolStatus, fifo string) {
	os.Remove(fifo)

	err := syscall.Mkfifo(fifo, 0666)
	if err != nil {
		ctx.Fatalf("Failed to mkfifo(%q): %v", fifo, err)
	}

	go ninjaReader(ctx, status, fifo)
}

func ninjaReader(ctx logger.Logger, status ToolStatus, fifo string) {
	defer os.Remove(fifo)

	f, err := os.Open(fifo)
	if err != nil {
		ctx.Fatal("Failed to open fifo:", err)
	}
	defer f.Close()

	r := bufio.NewReader(f)

	running := map[uint32]*Action{}

	for {
		size, err := readVarInt(r)
		if err != nil {
			if err != io.EOF {
				ctx.Println("Got error reading from ninja:", err)
			}
			return
		}

		buf := make([]byte, size)
		_, err = io.ReadFull(r, buf)
		if err != nil {
			if err == io.EOF {
				ctx.Printf("Missing message of size %d from ninja\n", size)
			} else {
				ctx.Fatal("Got error reading from ninja:", err)
			}
			return
		}

		msg := &ninja_frontend.Status{}
		err = proto.Unmarshal(buf, msg)
		if err != nil {
			ctx.Printf("Error reading message from ninja: %v\n", err)
			continue
		}

		// Ignore msg.BuildStarted
		if msg.TotalEdges != nil {
			status.SetTotalActions(int(msg.TotalEdges.GetTotalEdges()))
		}
		if msg.EdgeStarted != nil {
			action := &Action{
				Description: msg.EdgeStarted.GetDesc(),
				Outputs:     msg.EdgeStarted.Outputs,
				Command:     msg.EdgeStarted.GetCommand(),
			}
			status.StartAction(action)
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

				status.FinishAction(ActionResult{
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
				status.Status(message)
			case ninja_frontend.Status_Message_WARNING:
				status.Print("warning: " + message)
			case ninja_frontend.Status_Message_ERROR:
				status.Error(message)
			default:
				status.Print(message)
			}
		}
		if msg.BuildFinished != nil {
			status.Finish()
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
