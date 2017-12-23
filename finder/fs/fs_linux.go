// Copyright 2017 Google Inc. All rights reserved.
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

package fs

import (
	"fmt"
	"os"
	"syscall"
	"time"
)

func (osFs) InodeNumber(info os.FileInfo) (number uint64, err error) {
	sys := info.Sys()
	linuxStats, ok := sys.(*syscall.Stat_t)
	if ok {
		return linuxStats.Ino, nil
	}
	return 0, fmt.Errorf("%v is not a *syscall.Stat_t", sys)
}

func (osFs) DeviceNumber(info os.FileInfo) (number uint64, err error) {
	sys := info.Sys()
	linuxStats, ok := sys.(*syscall.Stat_t)
	if ok {
		return linuxStats.Dev, nil
	}
	return 0, fmt.Errorf("%v is not a *syscall.Stat_t", sys)
}

func (osFs) PermTime(info os.FileInfo) (when time.Time, err error) {
	sys := info.Sys()
	linuxStats, ok := sys.(*syscall.Stat_t)
	if ok {
		return time.Unix(linuxStats.Ctim.Sec, linuxStats.Ctim.Nsec), nil
	}
	return time.Time{}, fmt.Errorf("%v is not a *syscall.Stat_t", sys)
}
