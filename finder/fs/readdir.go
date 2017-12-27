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

// This is based on the readdir implementation from Go 1.9:
// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

import (
	"os"
	"syscall"
	"unsafe"
)

const (
	blockSize = 4096
)

func readdir(path string) ([]DirEntryInfo, error) {
	f, err := os.Open(path)
	defer f.Close()

	if err != nil {
		return nil, err
	}
	// This implicitly switches the fd to non-blocking mode, which is less efficient than what
	// file.ReadDir does since it will keep a thread blocked and not just a goroutine.
	fd := int(f.Fd())

	buf := make([]byte, blockSize)
	entries := make([]*dirEntryInfo, 0, 100)

	for {
		n, errno := syscall.ReadDirent(fd, buf)
		if errno != nil {
			err = os.NewSyscallError("readdirent", errno)
			break
		}
		if n <= 0 {
			break // EOF
		}

		entries = parseDirent(buf[:n], entries)
	}

	ret := make([]DirEntryInfo, 0, len(entries))

	for _, entry := range entries {
		if !entry.modeExists {
			mode, lerr := lstatFileMode(path + "/" + entry.name)
			if os.IsNotExist(lerr) {
				// File disappeared between readdir + stat.
				// Just treat it as if it didn't exist.
				continue
			}
			if lerr != nil {
				return ret, lerr
			}
			entry.mode = mode
			entry.modeExists = true
		}
		ret = append(ret, entry)
	}

	return ret, err
}

func parseDirent(buf []byte, entries []*dirEntryInfo) []*dirEntryInfo {
	for len(buf) > 0 {
		reclen, ok := direntReclen(buf)
		if !ok || reclen > uint64(len(buf)) {
			return entries
		}
		rec := buf[:reclen]
		buf = buf[reclen:]
		ino, ok := direntIno(rec)
		if !ok {
			break
		}
		if ino == 0 { // File absent in directory.
			continue
		}
		typ, ok := direntType(rec)
		if !ok {
			break
		}
		const namoff = uint64(unsafe.Offsetof(syscall.Dirent{}.Name))
		namlen, ok := direntNamlen(rec)
		if !ok || namoff+namlen > uint64(len(rec)) {
			break
		}
		name := rec[namoff : namoff+namlen]

		for i, c := range name {
			if c == 0 {
				name = name[:i]
				break
			}
		}
		// Check for useless names before allocating a string.
		if string(name) == "." || string(name) == ".." {
			continue
		}

		mode, modeExists := direntTypeToFileMode(typ)

		entries = append(entries, &dirEntryInfo{string(name), mode, modeExists})
	}
	return entries
}

func direntIno(buf []byte) (uint64, bool) {
	return readInt(buf, unsafe.Offsetof(syscall.Dirent{}.Ino), unsafe.Sizeof(syscall.Dirent{}.Ino))
}

func direntType(buf []byte) (uint64, bool) {
	return readInt(buf, unsafe.Offsetof(syscall.Dirent{}.Type), unsafe.Sizeof(syscall.Dirent{}.Type))
}

func direntReclen(buf []byte) (uint64, bool) {
	return readInt(buf, unsafe.Offsetof(syscall.Dirent{}.Reclen), unsafe.Sizeof(syscall.Dirent{}.Reclen))
}

func direntNamlen(buf []byte) (uint64, bool) {
	reclen, ok := direntReclen(buf)
	if !ok {
		return 0, false
	}
	return reclen - uint64(unsafe.Offsetof(syscall.Dirent{}.Name)), true
}

// readInt returns the size-bytes unsigned integer in native byte order at offset off.
func readInt(b []byte, off, size uintptr) (u uint64, ok bool) {
	if len(b) < int(off+size) {
		return 0, false
	}
	return readIntLE(b[off:], size), true
}

func readIntLE(b []byte, size uintptr) uint64 {
	switch size {
	case 1:
		return uint64(b[0])
	case 2:
		_ = b[1] // bounds check hint to compiler; see golang.org/issue/14808
		return uint64(b[0]) | uint64(b[1])<<8
	case 4:
		_ = b[3] // bounds check hint to compiler; see golang.org/issue/14808
		return uint64(b[0]) | uint64(b[1])<<8 | uint64(b[2])<<16 | uint64(b[3])<<24
	case 8:
		_ = b[7] // bounds check hint to compiler; see golang.org/issue/14808
		return uint64(b[0]) | uint64(b[1])<<8 | uint64(b[2])<<16 | uint64(b[3])<<24 |
			uint64(b[4])<<32 | uint64(b[5])<<40 | uint64(b[6])<<48 | uint64(b[7])<<56
	default:
		panic("syscall: readInt with unsupported size")
	}
}

// If the directory entry doesn't specify the type, fall back to using lstat to get the type.
func lstatFileMode(name string) (os.FileMode, error) {
	stat, err := os.Lstat(name)
	if err != nil {
		return 0, err
	}

	return stat.Mode() & (os.ModeType | os.ModeCharDevice), nil
}

// from Linux and Darwin dirent.h
const (
	DT_UNKNOWN = 0
	DT_FIFO    = 1
	DT_CHR     = 2
	DT_DIR     = 4
	DT_BLK     = 6
	DT_REG     = 8
	DT_LNK     = 10
	DT_SOCK    = 12
)

func direntTypeToFileMode(typ uint64) (os.FileMode, bool) {
	exists := true
	var mode os.FileMode
	switch typ {
	case DT_UNKNOWN:
		exists = false
	case DT_FIFO:
		mode = os.ModeNamedPipe
	case DT_CHR:
		mode = os.ModeDevice | os.ModeCharDevice
	case DT_DIR:
		mode = os.ModeDir
	case DT_BLK:
		mode = os.ModeDevice
	case DT_REG:
		mode = 0
	case DT_LNK:
		mode = os.ModeSymlink
	case DT_SOCK:
		mode = os.ModeSocket
	default:
		exists = false
	}

	return mode, exists
}
