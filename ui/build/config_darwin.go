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

package build

import (
	"encoding/binary"
	"syscall"
)

func detectTotalRAM(ctx Context) uint64 {
	s, err := syscall.Sysctl("hw.memsize")
	if err != nil {
		ctx.Printf("Failed to get system memory size: %s")
		return 0
	}

	// syscall.Sysctl assumes that the return value is a string and trims the last byte if it is 0.
	if len(s) == 7 {
		s += "\x00"
	}

	if len(s) != 8 {
		ctx.Printf("Failed to get system memory size, returned %d bytes, 8", len(s))
		return 0
	}

	return binary.LittleEndian.Uint64([]byte(s))
}
