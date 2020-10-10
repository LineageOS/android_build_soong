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

package cc

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestGen(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		ctx := testCc(t, `
		cc_library_shared {
			name: "libfoo",
			srcs: [
				"foo.c",
				"b.aidl",
			],
		}`)

		aidl := ctx.ModuleForTests("libfoo", "android_arm_armv7-a-neon_shared").Rule("aidl")
		libfoo := ctx.ModuleForTests("libfoo", "android_arm_armv7-a-neon_shared").Module().(*Module)

		if !inList("-I"+filepath.Dir(aidl.Output.String()), libfoo.flags.Local.CommonFlags) {
			t.Errorf("missing aidl includes in global flags")
		}
	})

	t.Run("filegroup", func(t *testing.T) {
		ctx := testCc(t, `
		filegroup {
			name: "fg",
			srcs: ["sub/c.aidl"],
			path: "sub",
		}

		cc_library_shared {
			name: "libfoo",
			srcs: [
				"foo.c",
				":fg",
			],
		}`)

		aidl := ctx.ModuleForTests("libfoo", "android_arm_armv7-a-neon_shared").Rule("aidl")
		libfoo := ctx.ModuleForTests("libfoo", "android_arm_armv7-a-neon_shared").Module().(*Module)

		if !inList("-I"+filepath.Dir(aidl.Output.String()), libfoo.flags.Local.CommonFlags) {
			t.Errorf("missing aidl includes in global flags")
		}

		aidlCommand := aidl.RuleParams.Command
		if !strings.Contains(aidlCommand, "-Isub") {
			t.Errorf("aidl command for c.aidl should contain \"-Isub\", but was %q", aidlCommand)
		}

	})

}
