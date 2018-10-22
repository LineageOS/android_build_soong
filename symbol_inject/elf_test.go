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

package symbol_inject

import (
	"strconv"
	"testing"
)

func TestElfSymbolTable(t *testing.T) {
	testCases := []struct {
		file         *mockElfFile
		symbol       string
		offset, size uint64
	}{
		{
			file:   elfSymbolTable1,
			symbol: "soong_build_number",
			offset: 0x1030,
			size:   128,
		},
		{
			file:   elfSymbolTable2,
			symbol: "symbol1",
			offset: 0x1030,
			size:   128,
		},
		{
			file:   elfSymbolTable2,
			symbol: "symbol2",
			offset: 0x10b0,
			size:   128,
		},
	}

	for i, testCase := range testCases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			file, err := extractElfSymbols(testCase.file)
			if err != nil {
				t.Error(err.Error())
			}
			offset, size, err := findSymbol(file, testCase.symbol)
			if err != nil {
				t.Error(err.Error())
			}
			if offset != testCase.offset || size != testCase.size {
				t.Errorf("expected %x:%x, got %x:%x", testCase.offset, testCase.size, offset, size)
			}
		})
	}
}
