// Copyright 2022 Google Inc. All rights reserved.
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
	"debug/elf"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
)

const gnuBuildID = "GNU\x00"

// elfIdentifier extracts the elf build ID from an elf file.  If allowMissing is true it returns
// an empty identifier if the file exists but the build ID note does not.
func elfIdentifier(filename string, allowMissing bool) (string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return "", fmt.Errorf("failed to open %s: %w", filename, err)
	}
	defer f.Close()

	return elfIdentifierFromReaderAt(f, filename, allowMissing)
}

// elfIdentifierFromReaderAt extracts the elf build ID from a ReaderAt.  If allowMissing is true it
// returns an empty identifier if the file exists but the build ID note does not.
func elfIdentifierFromReaderAt(r io.ReaderAt, filename string, allowMissing bool) (string, error) {
	f, err := elf.NewFile(r)
	if err != nil {
		if allowMissing {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return "", nil
			}
			if _, ok := err.(*elf.FormatError); ok {
				// The file was not an elf file.
				return "", nil
			}
		}
		return "", fmt.Errorf("failed to parse elf file %s: %w", filename, err)
	}
	defer f.Close()

	buildIDNote := f.Section(".note.gnu.build-id")
	if buildIDNote == nil {
		if allowMissing {
			return "", nil
		}
		return "", fmt.Errorf("failed to find .note.gnu.build-id in  %s", filename)
	}

	buildIDs, err := readNote(buildIDNote.Open(), f.ByteOrder)
	if err != nil {
		return "", fmt.Errorf("failed to read .note.gnu.build-id: %w", err)
	}

	for name, desc := range buildIDs {
		if name == gnuBuildID {
			return hex.EncodeToString(desc), nil
		}
	}

	return "", nil
}

// readNote reads the contents of a note section, returning it as a map from name to descriptor.
func readNote(note io.Reader, byteOrder binary.ByteOrder) (map[string][]byte, error) {
	var noteHeader struct {
		Namesz uint32
		Descsz uint32
		Type   uint32
	}

	notes := make(map[string][]byte)
	for {
		err := binary.Read(note, byteOrder, &noteHeader)
		if err != nil {
			if err == io.EOF {
				return notes, nil
			}
			return nil, fmt.Errorf("failed to read note header: %w", err)
		}

		nameBuf := make([]byte, align4(noteHeader.Namesz))
		err = binary.Read(note, byteOrder, &nameBuf)
		if err != nil {
			return nil, fmt.Errorf("failed to read note name: %w", err)
		}
		name := string(nameBuf[:noteHeader.Namesz])

		descBuf := make([]byte, align4(noteHeader.Descsz))
		err = binary.Read(note, byteOrder, &descBuf)
		if err != nil {
			return nil, fmt.Errorf("failed to read note desc: %w", err)
		}
		notes[name] = descBuf[:noteHeader.Descsz]
	}
}

// align4 rounds the input up to the next multiple of 4.
func align4(i uint32) uint32 {
	return (i + 3) &^ 3
}
