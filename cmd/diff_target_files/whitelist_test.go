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
	"bytes"
	"reflect"
	"testing"
)

func bytesToZipArtifactFile(name string, data []byte) *ZipArtifactFile {
	buf := &bytes.Buffer{}
	w := zip.NewWriter(buf)
	f, err := w.Create(name)
	if err != nil {
		panic(err)
	}
	_, err = f.Write(data)
	if err != nil {
		panic(err)
	}

	w.Close()

	r, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		panic(err)
	}

	return &ZipArtifactFile{r.File[0]}
}

var f1a = bytesToZipArtifactFile("dir/f1", []byte(`
a
foo: bar
c
`))

var f1b = bytesToZipArtifactFile("dir/f1", []byte(`
a
foo: baz
c
`))

var f2 = bytesToZipArtifactFile("dir/f2", nil)

func Test_applyWhitelists(t *testing.T) {
	type args struct {
		diff       zipDiff
		whitelists []whitelist
	}
	tests := []struct {
		name    string
		args    args
		want    zipDiff
		wantErr bool
	}{
		{
			name: "simple",
			args: args{
				diff: zipDiff{
					onlyInA: []*ZipArtifactFile{f1a, f2},
				},
				whitelists: []whitelist{{path: "dir/f1"}},
			},
			want: zipDiff{
				onlyInA: []*ZipArtifactFile{f2},
			},
		},
		{
			name: "glob",
			args: args{
				diff: zipDiff{
					onlyInA: []*ZipArtifactFile{f1a, f2},
				},
				whitelists: []whitelist{{path: "dir/*"}},
			},
			want: zipDiff{},
		},
		{
			name: "modified",
			args: args{
				diff: zipDiff{
					modified: [][2]*ZipArtifactFile{{f1a, f1b}},
				},
				whitelists: []whitelist{{path: "dir/*"}},
			},
			want: zipDiff{},
		},
		{
			name: "matching lines",
			args: args{
				diff: zipDiff{
					modified: [][2]*ZipArtifactFile{{f1a, f1b}},
				},
				whitelists: []whitelist{{path: "dir/*", ignoreMatchingLines: []string{"foo: .*"}}},
			},
			want: zipDiff{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := applyWhitelists(tt.args.diff, tt.args.whitelists)
			if (err != nil) != tt.wantErr {
				t.Errorf("applyWhitelists() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("applyWhitelists() = %v, want %v", got, tt.want)
			}
		})
	}
}
