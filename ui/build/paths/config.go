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

package paths

import "runtime"

type PathConfig struct {
	// Whether to create the symlink in the new PATH for this tool.
	Symlink bool

	// Whether to log about usages of this tool to the soong.log
	Log bool

	// Whether to exit with an error instead of invoking the underlying tool.
	Error bool
}

var Allowed = PathConfig{
	Symlink: true,
	Log:     false,
	Error:   false,
}

var Forbidden = PathConfig{
	Symlink: false,
	Log:     true,
	Error:   true,
}

// The configuration used if the tool is not listed in the config below.
// Currently this will create the symlink, but log a warning. In the future,
// I expect this to move closer to Forbidden.
var Missing = PathConfig{
	Symlink: true,
	Log:     true,
	Error:   false,
}

func GetConfig(name string) PathConfig {
	if config, ok := Configuration[name]; ok {
		return config
	}
	return Missing
}

var Configuration = map[string]PathConfig{
	"awk":       Allowed,
	"basename":  Allowed,
	"bash":      Allowed,
	"bc":        Allowed,
	"bzip2":     Allowed,
	"cat":       Allowed,
	"chmod":     Allowed,
	"cmp":       Allowed,
	"comm":      Allowed,
	"cp":        Allowed,
	"cut":       Allowed,
	"date":      Allowed,
	"dd":        Allowed,
	"diff":      Allowed,
	"dirname":   Allowed,
	"echo":      Allowed,
	"egrep":     Allowed,
	"env":       Allowed,
	"expr":      Allowed,
	"find":      Allowed,
	"getconf":   Allowed,
	"getopt":    Allowed,
	"git":       Allowed,
	"grep":      Allowed,
	"gzip":      Allowed,
	"head":      Allowed,
	"hexdump":   Allowed,
	"hostname":  Allowed,
	"id":        Allowed,
	"jar":       Allowed,
	"java":      Allowed,
	"javap":     Allowed,
	"ln":        Allowed,
	"ls":        Allowed,
	"m4":        Allowed,
	"make":      Allowed,
	"md5sum":    Allowed,
	"mkdir":     Allowed,
	"mktemp":    Allowed,
	"mv":        Allowed,
	"openssl":   Allowed,
	"patch":     Allowed,
	"perl":      Allowed,
	"pgrep":     Allowed,
	"pkill":     Allowed,
	"pstree":    Allowed,
	"pwd":       Allowed,
	"python":    Allowed,
	"python2.7": Allowed,
	"python3":   Allowed,
	"readlink":  Allowed,
	"realpath":  Allowed,
	"rm":        Allowed,
	"rmdir":     Allowed,
	"rsync":     Allowed,
	"runalarm":  Allowed,
	"sed":       Allowed,
	"setsid":    Allowed,
	"sh":        Allowed,
	"sha1sum":   Allowed,
	"sha256sum": Allowed,
	"sha512sum": Allowed,
	"sleep":     Allowed,
	"sort":      Allowed,
	"stat":      Allowed,
	"sum":       Allowed,
	"tar":       Allowed,
	"tail":      Allowed,
	"touch":     Allowed,
	"tr":        Allowed,
	"true":      Allowed,
	"uname":     Allowed,
	"uniq":      Allowed,
	"unzip":     Allowed,
	"wc":        Allowed,
	"which":     Allowed,
	"whoami":    Allowed,
	"xargs":     Allowed,
	"xmllint":   Allowed,
	"xz":        Allowed,
	"zip":       Allowed,
	"zipinfo":   Allowed,

	// Host toolchain is removed. In-tree toolchain should be used instead.
	// GCC also can't find cc1 with this implementation.
	"ar":         Forbidden,
	"as":         Forbidden,
	"cc":         Forbidden,
	"clang":      Forbidden,
	"clang++":    Forbidden,
	"gcc":        Forbidden,
	"g++":        Forbidden,
	"ld":         Forbidden,
	"ld.bfd":     Forbidden,
	"ld.gold":    Forbidden,
	"pkg-config": Forbidden,

	// We've got prebuilts of these
	//"dtc":  Forbidden,
	//"lz4":  Forbidden,
	//"lz4c": Forbidden,
}

func init() {
	if runtime.GOOS == "darwin" {
		Configuration["md5"] = Allowed
		Configuration["sw_vers"] = Allowed
		Configuration["xcrun"] = Allowed
	}
}
