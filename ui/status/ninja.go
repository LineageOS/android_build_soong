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
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"time"

	"google.golang.org/protobuf/proto"

	"android/soong/ui/logger"
	"android/soong/ui/status/ninja_frontend"
)

// NewNinjaReader reads the protobuf frontend format from ninja and translates it
// into calls on the ToolStatus API.
func NewNinjaReader(ctx logger.Logger, status ToolStatus, fifo string) *NinjaReader {
	os.Remove(fifo)

	if err := syscall.Mkfifo(fifo, 0666); err != nil {
		ctx.Fatalf("Failed to mkfifo(%q): %v", fifo, err)
	}

	n := &NinjaReader{
		status:     status,
		fifo:       fifo,
		forceClose: make(chan bool),
		done:       make(chan bool),
		cancelOpen: make(chan bool),
	}

	go n.run()

	return n
}

type NinjaReader struct {
	status     ToolStatus
	fifo       string
	forceClose chan bool
	done       chan bool
	cancelOpen chan bool
}

const NINJA_READER_CLOSE_TIMEOUT = 5 * time.Second

// Close waits for NinjaReader to finish reading from the fifo, or 5 seconds.
func (n *NinjaReader) Close() {
	// Signal the goroutine to stop if it is blocking opening the fifo.
	close(n.cancelOpen)

	// Ninja should already have exited or been killed, wait 5 seconds for the FIFO to be closed and any
	// remaining messages to be processed through the NinjaReader.run goroutine.
	timeoutCh := time.After(NINJA_READER_CLOSE_TIMEOUT)
	select {
	case <-n.done:
		return
	case <-timeoutCh:
		// Channel is not closed yet
	}

	n.status.Error(fmt.Sprintf("ninja fifo didn't finish after %s", NINJA_READER_CLOSE_TIMEOUT.String()))

	// Force close the reader even if the FIFO didn't close.
	close(n.forceClose)

	// Wait again for the reader thread to acknowledge the close before giving up and assuming it isn't going
	// to send anything else.
	timeoutCh = time.After(NINJA_READER_CLOSE_TIMEOUT)
	select {
	case <-n.done:
		return
	case <-timeoutCh:
		// Channel is not closed yet
	}

	n.status.Verbose(fmt.Sprintf("ninja fifo didn't finish even after force closing after %s", NINJA_READER_CLOSE_TIMEOUT.String()))
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
	case <-n.cancelOpen:
		return
	}

	defer f.Close()

	r := bufio.NewReader(f)

	running := map[uint32]*Action{}

	msgChan := make(chan *ninja_frontend.Status)

	// Read from the ninja fifo and decode the protobuf in a goroutine so the main NinjaReader.run goroutine
	// can listen
	go func() {
		defer close(msgChan)
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

			msgChan <- msg
		}
	}()

	for {
		var msg *ninja_frontend.Status
		var msgOk bool
		select {
		case <-n.forceClose:
			// Close() has been called, but the reader goroutine didn't get EOF after 5 seconds
			break
		case msg, msgOk = <-msgChan:
			// msg is ready or closed
		}

		if !msgOk {
			// msgChan is closed
			break
		}

		if msg.BuildStarted != nil {
			parallelism := uint32(runtime.NumCPU())
			if msg.BuildStarted.GetParallelism() > 0 {
				parallelism = msg.BuildStarted.GetParallelism()
			}
			// It is estimated from total time / parallelism assumming the build is packing enough.
			estimatedDurationFromTotal := time.Duration(msg.BuildStarted.GetEstimatedTotalTime()/parallelism) * time.Millisecond
			// It is estimated from critical path time which is useful for small size build.
			estimatedDurationFromCriticalPath := time.Duration(msg.BuildStarted.GetCriticalPathTime()) * time.Millisecond
			// Select the longer one.
			estimatedDuration := max(estimatedDurationFromTotal, estimatedDurationFromCriticalPath)

			if estimatedDuration > 0 {
				n.status.SetEstimatedTime(time.Now().Add(estimatedDuration))
				n.status.Verbose(fmt.Sprintf("parallelism: %d, estimated from total time: %s, critical path time: %s",
					parallelism,
					estimatedDurationFromTotal,
					estimatedDurationFromCriticalPath))

			}
		}
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

				outputWithErrorHint := errorHintGenerator.GetOutputWithErrorHint(msg.EdgeFinished.GetOutput(), exitCode)
				n.status.FinishAction(ActionResult{
					Action: started,
					Output: outputWithErrorHint,
					Error:  err,
					Stats: ActionResultStats{
						UserTime:                   msg.EdgeFinished.GetUserTime(),
						SystemTime:                 msg.EdgeFinished.GetSystemTime(),
						MaxRssKB:                   msg.EdgeFinished.GetMaxRssKb(),
						MinorPageFaults:            msg.EdgeFinished.GetMinorPageFaults(),
						MajorPageFaults:            msg.EdgeFinished.GetMajorPageFaults(),
						IOInputKB:                  msg.EdgeFinished.GetIoInputKb(),
						IOOutputKB:                 msg.EdgeFinished.GetIoOutputKb(),
						VoluntaryContextSwitches:   msg.EdgeFinished.GetVoluntaryContextSwitches(),
						InvoluntaryContextSwitches: msg.EdgeFinished.GetInvoluntaryContextSwitches(),
						Tags:                       msg.EdgeFinished.GetTags(),
					},
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
			case ninja_frontend.Status_Message_DEBUG:
				n.status.Verbose(message)
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

// key is pattern in stdout/stderr
// value is error hint
var allErrorHints = map[string]string{
	"Read-only file system": `\nWrite to a read-only file system detected. Possible fixes include
1. Generate file directly to out/ which is ReadWrite, #recommend solution
2. BUILD_BROKEN_SRC_DIR_RW_ALLOWLIST := <my/path/1> <my/path/2> #discouraged, subset of source tree will be RW
3. BUILD_BROKEN_SRC_DIR_IS_WRITABLE := true #highly discouraged, entire source tree will be RW
`,
}
var errorHintGenerator = *newErrorHintGenerator(allErrorHints)

type ErrorHintGenerator struct {
	allErrorHints                map[string]string
	allErrorHintPatternsCompiled *regexp.Regexp
}

func newErrorHintGenerator(allErrorHints map[string]string) *ErrorHintGenerator {
	var allErrorHintPatterns []string
	for errorHintPattern, _ := range allErrorHints {
		allErrorHintPatterns = append(allErrorHintPatterns, errorHintPattern)
	}
	allErrorHintPatternsRegex := strings.Join(allErrorHintPatterns[:], "|")
	re := regexp.MustCompile(allErrorHintPatternsRegex)
	return &ErrorHintGenerator{
		allErrorHints:                allErrorHints,
		allErrorHintPatternsCompiled: re,
	}
}

func (errorHintGenerator *ErrorHintGenerator) GetOutputWithErrorHint(rawOutput string, buildExitCode int) string {
	if buildExitCode == 0 {
		return rawOutput
	}
	errorHint := errorHintGenerator.getErrorHint(rawOutput)
	if errorHint == nil {
		return rawOutput
	}
	return rawOutput + *errorHint
}

// Returns the error hint corresponding to the FIRST match in raw output
func (errorHintGenerator *ErrorHintGenerator) getErrorHint(rawOutput string) *string {
	firstMatch := errorHintGenerator.allErrorHintPatternsCompiled.FindString(rawOutput)
	if _, found := errorHintGenerator.allErrorHints[firstMatch]; found {
		errorHint := errorHintGenerator.allErrorHints[firstMatch]
		return &errorHint
	}
	return nil
}
