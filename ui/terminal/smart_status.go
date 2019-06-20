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

package terminal

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"android/soong/ui/status"
)

const tableHeightEnVar = "SOONG_UI_TABLE_HEIGHT"

type actionTableEntry struct {
	action    *status.Action
	startTime time.Time
}

type smartStatusOutput struct {
	writer    io.Writer
	formatter formatter

	lock sync.Mutex

	haveBlankLine bool

	tableMode             bool
	tableHeight           int
	requestedTableHeight  int
	termWidth, termHeight int

	runningActions  []actionTableEntry
	ticker          *time.Ticker
	done            chan bool
	sigwinch        chan os.Signal
	sigwinchHandled chan bool
}

// NewSmartStatusOutput returns a StatusOutput that represents the
// current build status similarly to Ninja's built-in terminal
// output.
func NewSmartStatusOutput(w io.Writer, formatter formatter) status.StatusOutput {
	tableHeight, _ := strconv.Atoi(os.Getenv(tableHeightEnVar))

	s := &smartStatusOutput{
		writer:    w,
		formatter: formatter,

		haveBlankLine: true,

		tableMode:            tableHeight > 0,
		requestedTableHeight: tableHeight,

		done:     make(chan bool),
		sigwinch: make(chan os.Signal),
	}

	s.updateTermSize()

	if s.tableMode {
		// Add empty lines at the bottom of the screen to scroll back the existing history
		// and make room for the action table.
		// TODO: read the cursor position to see if the empty lines are necessary?
		for i := 0; i < s.tableHeight; i++ {
			fmt.Fprintln(w)
		}

		// Hide the cursor to prevent seeing it bouncing around
		fmt.Fprintf(s.writer, ansi.hideCursor())

		// Configure the empty action table
		s.actionTable()

		// Start a tick to update the action table periodically
		s.startActionTableTick()
	}

	s.startSigwinch()

	return s
}

func (s *smartStatusOutput) Message(level status.MsgLevel, message string) {
	if level < status.StatusLvl {
		return
	}

	str := s.formatter.message(level, message)

	s.lock.Lock()
	defer s.lock.Unlock()

	if level > status.StatusLvl {
		s.print(str)
	} else {
		s.statusLine(str)
	}
}

func (s *smartStatusOutput) StartAction(action *status.Action, counts status.Counts) {
	startTime := time.Now()

	str := action.Description
	if str == "" {
		str = action.Command
	}

	progress := s.formatter.progress(counts)

	s.lock.Lock()
	defer s.lock.Unlock()

	s.runningActions = append(s.runningActions, actionTableEntry{
		action:    action,
		startTime: startTime,
	})

	s.statusLine(progress + str)
}

func (s *smartStatusOutput) FinishAction(result status.ActionResult, counts status.Counts) {
	str := result.Description
	if str == "" {
		str = result.Command
	}

	progress := s.formatter.progress(counts) + str

	output := s.formatter.result(result)

	s.lock.Lock()
	defer s.lock.Unlock()

	for i, runningAction := range s.runningActions {
		if runningAction.action == result.Action {
			s.runningActions = append(s.runningActions[:i], s.runningActions[i+1:]...)
			break
		}
	}

	if output != "" {
		s.statusLine(progress)
		s.requestLine()
		s.print(output)
	} else {
		s.statusLine(progress)
	}
}

func (s *smartStatusOutput) Flush() {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.stopSigwinch()

	s.requestLine()

	s.runningActions = nil

	if s.tableMode {
		s.stopActionTableTick()

		// Update the table after clearing runningActions to clear it
		s.actionTable()

		// Reset the scrolling region to the whole terminal
		fmt.Fprintf(s.writer, ansi.resetScrollingMargins())
		_, height, _ := termSize(s.writer)
		// Move the cursor to the top of the now-blank, previously non-scrolling region
		fmt.Fprintf(s.writer, ansi.setCursor(height-s.tableHeight, 0))
		// Turn the cursor back on
		fmt.Fprintf(s.writer, ansi.showCursor())
	}
}

func (s *smartStatusOutput) Write(p []byte) (int, error) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.print(string(p))
	return len(p), nil
}

func (s *smartStatusOutput) requestLine() {
	if !s.haveBlankLine {
		fmt.Fprintln(s.writer)
		s.haveBlankLine = true
	}
}

func (s *smartStatusOutput) print(str string) {
	if !s.haveBlankLine {
		fmt.Fprint(s.writer, "\r", ansi.clearToEndOfLine())
		s.haveBlankLine = true
	}
	fmt.Fprint(s.writer, str)
	if len(str) == 0 || str[len(str)-1] != '\n' {
		fmt.Fprint(s.writer, "\n")
	}
}

func (s *smartStatusOutput) statusLine(str string) {
	idx := strings.IndexRune(str, '\n')
	if idx != -1 {
		str = str[0:idx]
	}

	// Limit line width to the terminal width, otherwise we'll wrap onto
	// another line and we won't delete the previous line.
	if s.termWidth > 0 {
		str = s.elide(str)
	}

	// Move to the beginning on the line, turn on bold, print the output,
	// turn off bold, then clear the rest of the line.
	start := "\r" + ansi.bold()
	end := ansi.regular() + ansi.clearToEndOfLine()
	fmt.Fprint(s.writer, start, str, end)
	s.haveBlankLine = false
}

func (s *smartStatusOutput) elide(str string) string {
	if len(str) > s.termWidth {
		// TODO: Just do a max. Ninja elides the middle, but that's
		// more complicated and these lines aren't that important.
		str = str[:s.termWidth]
	}

	return str
}

func (s *smartStatusOutput) startActionTableTick() {
	s.ticker = time.NewTicker(time.Second)
	go func() {
		for {
			select {
			case <-s.ticker.C:
				s.lock.Lock()
				s.actionTable()
				s.lock.Unlock()
			case <-s.done:
				return
			}
		}
	}()
}

func (s *smartStatusOutput) stopActionTableTick() {
	s.ticker.Stop()
	s.done <- true
}

func (s *smartStatusOutput) startSigwinch() {
	signal.Notify(s.sigwinch, syscall.SIGWINCH)
	go func() {
		for _ = range s.sigwinch {
			s.lock.Lock()
			s.updateTermSize()
			if s.tableMode {
				s.actionTable()
			}
			s.lock.Unlock()
			if s.sigwinchHandled != nil {
				s.sigwinchHandled <- true
			}
		}
	}()
}

func (s *smartStatusOutput) stopSigwinch() {
	signal.Stop(s.sigwinch)
	close(s.sigwinch)
}

func (s *smartStatusOutput) updateTermSize() {
	if w, h, ok := termSize(s.writer); ok {
		firstUpdate := s.termHeight == 0 && s.termWidth == 0
		oldScrollingHeight := s.termHeight - s.tableHeight

		s.termWidth, s.termHeight = w, h

		if s.tableMode {
			tableHeight := s.requestedTableHeight
			if tableHeight > s.termHeight-1 {
				tableHeight = s.termHeight - 1
			}
			s.tableHeight = tableHeight

			scrollingHeight := s.termHeight - s.tableHeight

			if !firstUpdate {
				// If the scrolling region has changed, attempt to pan the existing text so that it is
				// not overwritten by the table.
				if scrollingHeight < oldScrollingHeight {
					pan := oldScrollingHeight - scrollingHeight
					if pan > s.tableHeight {
						pan = s.tableHeight
					}
					fmt.Fprint(s.writer, ansi.panDown(pan))
				}
			}
		}
	}
}

func (s *smartStatusOutput) actionTable() {
	scrollingHeight := s.termHeight - s.tableHeight

	// Update the scrolling region in case the height of the terminal changed
	fmt.Fprint(s.writer, ansi.setScrollingMargins(0, scrollingHeight))
	// Move the cursor to the first line of the non-scrolling region
	fmt.Fprint(s.writer, ansi.setCursor(scrollingHeight+1, 0))

	// Write as many status lines as fit in the table
	var tableLine int
	var runningAction actionTableEntry
	for tableLine, runningAction = range s.runningActions {
		if tableLine >= s.tableHeight {
			break
		}

		seconds := int(time.Since(runningAction.startTime).Round(time.Second).Seconds())

		desc := runningAction.action.Description
		if desc == "" {
			desc = runningAction.action.Command
		}

		str := fmt.Sprintf("   %2d:%02d %s", seconds/60, seconds%60, desc)
		str = s.elide(str)
		fmt.Fprint(s.writer, str, ansi.clearToEndOfLine())
		if tableLine < s.tableHeight-1 {
			fmt.Fprint(s.writer, "\n")
		}
	}

	// Clear any remaining lines in the table
	for ; tableLine < s.tableHeight; tableLine++ {
		fmt.Fprint(s.writer, ansi.clearToEndOfLine())
		if tableLine < s.tableHeight-1 {
			fmt.Fprint(s.writer, "\n")
		}
	}

	// Move the cursor back to the last line of the scrolling region
	fmt.Fprint(s.writer, ansi.setCursor(scrollingHeight, 0))
}

var ansi = ansiImpl{}

type ansiImpl struct{}

func (ansiImpl) clearToEndOfLine() string {
	return "\x1b[K"
}

func (ansiImpl) setCursor(row, column int) string {
	// Direct cursor address
	return fmt.Sprintf("\x1b[%d;%dH", row, column)
}

func (ansiImpl) setScrollingMargins(top, bottom int) string {
	// Set Top and Bottom Margins DECSTBM
	return fmt.Sprintf("\x1b[%d;%dr", top, bottom)
}

func (ansiImpl) resetScrollingMargins() string {
	// Set Top and Bottom Margins DECSTBM
	return fmt.Sprintf("\x1b[r")
}

func (ansiImpl) bold() string {
	return "\x1b[1m"
}

func (ansiImpl) regular() string {
	return "\x1b[0m"
}

func (ansiImpl) showCursor() string {
	return "\x1b[?25h"
}

func (ansiImpl) hideCursor() string {
	return "\x1b[?25l"
}

func (ansiImpl) panDown(lines int) string {
	return fmt.Sprintf("\x1b[%dS", lines)
}

func (ansiImpl) panUp(lines int) string {
	return fmt.Sprintf("\x1b[%dT", lines)
}
