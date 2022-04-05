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
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"android/soong/cmd/symbols_map/symbols_map_proto"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
)

func Test_mergeProtos(t *testing.T) {
	type testFile struct {
		filename string
		contents *symbols_map_proto.Mapping
		missing  bool
	}

	tests := []struct {
		name               string
		inputs             []testFile
		stripPrefix        string
		writeIfChanged     bool
		ignoreMissingFiles bool

		error  string
		output *symbols_map_proto.Mappings
	}{
		{
			name:   "empty",
			output: &symbols_map_proto.Mappings{},
		},
		{
			name: "merge",
			inputs: []testFile{
				{
					filename: "foo",
					contents: &symbols_map_proto.Mapping{
						Identifier: proto.String("foo"),
						Location:   proto.String("symbols/foo"),
						Type:       symbols_map_proto.Mapping_ELF.Enum(),
					},
				},
				{
					filename: "bar",
					contents: &symbols_map_proto.Mapping{
						Identifier: proto.String("bar"),
						Location:   proto.String("symbols/bar"),
						Type:       symbols_map_proto.Mapping_R8.Enum(),
					},
				},
			},
			output: &symbols_map_proto.Mappings{
				Mappings: []*symbols_map_proto.Mapping{
					{
						Identifier: proto.String("foo"),
						Location:   proto.String("symbols/foo"),
						Type:       symbols_map_proto.Mapping_ELF.Enum(),
					},
					{
						Identifier: proto.String("bar"),
						Location:   proto.String("symbols/bar"),
						Type:       symbols_map_proto.Mapping_R8.Enum(),
					},
				},
			},
		},
		{
			name: "strip prefix",
			inputs: []testFile{
				{
					filename: "foo",
					contents: &symbols_map_proto.Mapping{
						Identifier: proto.String("foo"),
						Location:   proto.String("symbols/foo"),
						Type:       symbols_map_proto.Mapping_ELF.Enum(),
					},
				},
				{
					filename: "bar",
					contents: &symbols_map_proto.Mapping{
						Identifier: proto.String("bar"),
						Location:   proto.String("symbols/bar"),
						Type:       symbols_map_proto.Mapping_R8.Enum(),
					},
				},
			},
			stripPrefix: "symbols/",
			output: &symbols_map_proto.Mappings{
				Mappings: []*symbols_map_proto.Mapping{
					{
						Identifier: proto.String("foo"),
						Location:   proto.String("foo"),
						Type:       symbols_map_proto.Mapping_ELF.Enum(),
					},
					{
						Identifier: proto.String("bar"),
						Location:   proto.String("bar"),
						Type:       symbols_map_proto.Mapping_R8.Enum(),
					},
				},
			},
		},
		{
			name: "missing",
			inputs: []testFile{
				{
					filename: "foo",
					contents: &symbols_map_proto.Mapping{
						Identifier: proto.String("foo"),
						Location:   proto.String("symbols/foo"),
						Type:       symbols_map_proto.Mapping_ELF.Enum(),
					},
				},
				{
					filename: "bar",
					missing:  true,
				},
			},
			error: "no such file or directory",
		},
		{
			name: "ignore missing",
			inputs: []testFile{
				{
					filename: "foo",
					contents: &symbols_map_proto.Mapping{
						Identifier: proto.String("foo"),
						Location:   proto.String("symbols/foo"),
						Type:       symbols_map_proto.Mapping_ELF.Enum(),
					},
				},
				{
					filename: "bar",
					missing:  true,
				},
			},
			ignoreMissingFiles: true,
			output: &symbols_map_proto.Mappings{
				Mappings: []*symbols_map_proto.Mapping{
					{
						Identifier: proto.String("foo"),
						Location:   proto.String("symbols/foo"),
						Type:       symbols_map_proto.Mapping_ELF.Enum(),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, err := os.MkdirTemp("", "test_mergeProtos")
			if err != nil {
				t.Fatalf("failed to create temporary directory: %s", err)
			}
			defer os.RemoveAll(dir)

			var inputs []string
			for _, in := range tt.inputs {
				path := filepath.Join(dir, in.filename)
				inputs = append(inputs, path)
				if !in.missing {
					err := writeTextProto(path, in.contents, false)
					if err != nil {
						t.Fatalf("failed to create input file %s: %s", path, err)
					}
				}
			}
			output := filepath.Join(dir, "out")

			err = mergeProtos(output, inputs, tt.stripPrefix, tt.writeIfChanged, tt.ignoreMissingFiles)
			if err != nil {
				if tt.error != "" {
					if !strings.Contains(err.Error(), tt.error) {
						t.Fatalf("expected error %q, got %s", tt.error, err.Error())
					}
				} else {
					t.Fatalf("unexpected error %q", err)
				}
			} else if tt.error != "" {
				t.Fatalf("missing error %q", tt.error)
			} else {
				data, err := ioutil.ReadFile(output)
				if err != nil {
					t.Fatalf("failed to read output file %s: %s", output, err)
				}
				var got symbols_map_proto.Mappings
				err = prototext.Unmarshal(data, &got)
				if err != nil {
					t.Fatalf("failed to unmarshal textproto %s: %s", output, err)
				}

				if !proto.Equal(tt.output, &got) {
					t.Fatalf("expected output %q, got %q", tt.output.String(), got.String())
				}
			}
		})
	}
}
