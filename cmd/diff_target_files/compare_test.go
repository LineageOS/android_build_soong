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

package main

import (
	"archive/zip"
	"reflect"
	"testing"
)

func TestDiffTargetFilesLists(t *testing.T) {
	zipArtifactFile := func(name string, crc32 uint32, size uint64) *ZipArtifactFile {
		return &ZipArtifactFile{
			File: &zip.File{
				FileHeader: zip.FileHeader{
					Name:               name,
					CRC32:              crc32,
					UncompressedSize64: size,
				},
			},
		}
	}
	x0 := zipArtifactFile("x", 0, 0)
	x1 := zipArtifactFile("x", 1, 0)
	x2 := zipArtifactFile("x", 0, 2)
	y0 := zipArtifactFile("y", 0, 0)
	//y1 := zipArtifactFile("y", 1, 0)
	//y2 := zipArtifactFile("y", 1, 2)
	z0 := zipArtifactFile("z", 0, 0)
	z1 := zipArtifactFile("z", 1, 0)
	//z2 := zipArtifactFile("z", 1, 2)

	testCases := []struct {
		name string
		a, b []*ZipArtifactFile
		diff zipDiff
	}{
		{
			name: "same",
			a:    []*ZipArtifactFile{x0, y0, z0},
			b:    []*ZipArtifactFile{x0, y0, z0},
			diff: zipDiff{nil, nil, nil},
		},
		{
			name: "first only in a",
			a:    []*ZipArtifactFile{x0, y0, z0},
			b:    []*ZipArtifactFile{y0, z0},
			diff: zipDiff{nil, []*ZipArtifactFile{x0}, nil},
		},
		{
			name: "middle only in a",
			a:    []*ZipArtifactFile{x0, y0, z0},
			b:    []*ZipArtifactFile{x0, z0},
			diff: zipDiff{nil, []*ZipArtifactFile{y0}, nil},
		},
		{
			name: "last only in a",
			a:    []*ZipArtifactFile{x0, y0, z0},
			b:    []*ZipArtifactFile{x0, y0},
			diff: zipDiff{nil, []*ZipArtifactFile{z0}, nil},
		},

		{
			name: "first only in b",
			a:    []*ZipArtifactFile{y0, z0},
			b:    []*ZipArtifactFile{x0, y0, z0},
			diff: zipDiff{nil, nil, []*ZipArtifactFile{x0}},
		},
		{
			name: "middle only in b",
			a:    []*ZipArtifactFile{x0, z0},
			b:    []*ZipArtifactFile{x0, y0, z0},
			diff: zipDiff{nil, nil, []*ZipArtifactFile{y0}},
		},
		{
			name: "last only in b",
			a:    []*ZipArtifactFile{x0, y0},
			b:    []*ZipArtifactFile{x0, y0, z0},
			diff: zipDiff{nil, nil, []*ZipArtifactFile{z0}},
		},

		{
			name: "diff",
			a:    []*ZipArtifactFile{x0},
			b:    []*ZipArtifactFile{x1},
			diff: zipDiff{[][2]*ZipArtifactFile{{x0, x1}}, nil, nil},
		},
		{
			name: "diff plus unique last",
			a:    []*ZipArtifactFile{x0, y0},
			b:    []*ZipArtifactFile{x1, z0},
			diff: zipDiff{[][2]*ZipArtifactFile{{x0, x1}}, []*ZipArtifactFile{y0}, []*ZipArtifactFile{z0}},
		},
		{
			name: "diff plus unique first",
			a:    []*ZipArtifactFile{x0, z0},
			b:    []*ZipArtifactFile{y0, z1},
			diff: zipDiff{[][2]*ZipArtifactFile{{z0, z1}}, []*ZipArtifactFile{x0}, []*ZipArtifactFile{y0}},
		},
		{
			name: "diff size",
			a:    []*ZipArtifactFile{x0},
			b:    []*ZipArtifactFile{x2},
			diff: zipDiff{[][2]*ZipArtifactFile{{x0, x2}}, nil, nil},
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			diff := diffTargetFilesLists(test.a, test.b)

			if !reflect.DeepEqual(diff, test.diff) {

				t.Errorf("diffTargetFilesLists = %v, %v, %v", diff.modified, diff.onlyInA, diff.onlyInB)
				t.Errorf("                  want %v, %v, %v", test.diff.modified, test.diff.onlyInA, test.diff.onlyInB)
			}
		})
	}
}
