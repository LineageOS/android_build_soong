// Copyright 2020 Google Inc. All rights reserved.
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

package remoteexec

import (
	"fmt"
	"testing"
)

func TestTemplate(t *testing.T) {
	tests := []struct {
		name   string
		params *REParams
		want   string
	}{
		{
			name: "basic",
			params: &REParams{
				Labels:      map[string]string{"type": "compile", "lang": "cpp", "compiler": "clang"},
				Inputs:      []string{"$in"},
				OutputFiles: []string{"$out"},
				Platform: map[string]string{
					ContainerImageKey: DefaultImage,
					PoolKey:           "default",
				},
			},
			want: fmt.Sprintf("${remoteexec.Wrapper} --labels=compiler=clang,lang=cpp,type=compile --platform=\"Pool=default,container-image=%s\" --exec_strategy=local --inputs=$in --output_files=$out -- ", DefaultImage),
		},
		{
			name: "all params",
			params: &REParams{
				Labels:          map[string]string{"type": "compile", "lang": "cpp", "compiler": "clang"},
				Inputs:          []string{"$in"},
				OutputFiles:     []string{"$out"},
				ExecStrategy:    "remote",
				RSPFile:         "$out.rsp",
				ToolchainInputs: []string{"clang++"},
				Platform: map[string]string{
					ContainerImageKey: DefaultImage,
					PoolKey:           "default",
				},
			},
			want: fmt.Sprintf("${remoteexec.Wrapper} --labels=compiler=clang,lang=cpp,type=compile --platform=\"Pool=default,container-image=%s\" --exec_strategy=remote --inputs=$in --input_list_paths=$out.rsp --output_files=$out --toolchain_inputs=clang++ -- ", DefaultImage),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.params.Template(); got != test.want {
				t.Errorf("Template() returned\n%s\nwant\n%s", got, test.want)
			}
		})
	}
}

func TestTemplateDeterminism(t *testing.T) {
	r := &REParams{
		Labels:      map[string]string{"type": "compile", "lang": "cpp", "compiler": "clang"},
		Inputs:      []string{"$in"},
		OutputFiles: []string{"$out"},
		Platform: map[string]string{
			ContainerImageKey: DefaultImage,
			PoolKey:           "default",
		},
	}
	want := fmt.Sprintf("${remoteexec.Wrapper} --labels=compiler=clang,lang=cpp,type=compile --platform=\"Pool=default,container-image=%s\" --exec_strategy=local --inputs=$in --output_files=$out -- ", DefaultImage)
	for i := 0; i < 1000; i++ {
		if got := r.Template(); got != want {
			t.Fatalf("Template() returned\n%s\nwant\n%s", got, want)
		}
	}
}
