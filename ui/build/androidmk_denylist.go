// Copyright 2024 Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package build

import (
	"strings"
)

var androidmk_denylist []string = []string{
	"chained_build_config/",
	"cts/",
	"dalvik/",
	"developers/",
	// Do not block other directories in kernel/, see b/319658303.
	"kernel/configs/",
	"kernel/prebuilts/",
	"kernel/tests/",
	"libcore/",
	"libnativehelper/",
	"pdk/",
	// Add back toolchain/ once defensive Android.mk files are removed
	//"toolchain/",
}

func blockAndroidMks(ctx Context, androidMks []string) {
	for _, mkFile := range androidMks {
		for _, d := range androidmk_denylist {
			if strings.HasPrefix(mkFile, d) {
				ctx.Fatalf("Found blocked Android.mk file: %s. "+
					"Please see androidmk_denylist.go for the blocked directories and contact build system team if the file should not be blocked.", mkFile)
			}
		}
	}
}
