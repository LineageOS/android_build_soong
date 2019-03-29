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

package status

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"android/soong/ui/logger"
)

// Tests that closing the ninja reader when nothing has opened the other end of the fifo is fast.
func TestNinjaReader_Close(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "ninja_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	stat := &Status{}
	nr := NewNinjaReader(logger.New(ioutil.Discard), stat.StartTool(), filepath.Join(tempDir, "fifo"))

	start := time.Now()

	nr.Close()

	if g, w := time.Since(start), NINJA_READER_CLOSE_TIMEOUT; g >= w {
		t.Errorf("nr.Close timed out, %s > %s", g, w)
	}
}
