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

package java

import (
	"strings"
	"testing"

	"android/soong/android"
)

const protoModules = `
java_library_static {
    name: "libprotobuf-java-lite",
}
`

func TestProtoStream(t *testing.T) {
	bp := `
		java_library {
			name: "java-stream-protos",
			proto: {
				type: "stream",
			},
			srcs: [
				"a.proto",
				"b.proto",
			],
		}
	`

	ctx := android.GroupFixturePreparers(
		PrepareForIntegrationTestWithJava,
	).RunTestWithBp(t, protoModules+bp)

	proto0 := ctx.ModuleForTests("java-stream-protos", "android_common").Output("proto/proto0.srcjar")

	if cmd := proto0.RuleParams.Command; !strings.Contains(cmd, "--javastream_out=") {
		t.Errorf("expected '--javastream_out' in %q", cmd)
	}
}
