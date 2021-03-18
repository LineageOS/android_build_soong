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

package bpf

import (
	"os"
	"testing"

	"android/soong/android"
	"android/soong/cc"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

var bpfFactory = android.NewFixtureFactory(
	nil,
	cc.PrepareForTestWithCcDefaultModules,
	android.FixtureMergeMockFs(
		map[string][]byte{
			"bpf.c":       nil,
			"BpfTest.cpp": nil,
		},
	),
	PrepareForTestWithBpf,
)

func TestBpfDataDependency(t *testing.T) {
	bp := `
		bpf {
			name: "bpf.o",
			srcs: ["bpf.c"],
		}

		cc_test {
			name: "vts_test_binary_bpf_module",
			srcs: ["BpfTest.cpp"],
			data: [":bpf.o"],
			gtest: false,
		}
	`

	bpfFactory.RunTestWithBp(t, bp)

	// We only verify the above BP configuration is processed successfully since the data property
	// value is not available for testing from this package.
	// TODO(jungjw): Add a check for data or move this test to the cc package.
}
