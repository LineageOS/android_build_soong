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

	// Whether we use a linux-specific prebuilt for this tool. On Darwin,
	// we'll allow the host executable instead.
	LinuxOnlyPrebuilt bool
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

var Log = PathConfig{
	Symlink: true,
	Log:     true,
	Error:   false,
}

// The configuration used if the tool is not listed in the config below.
// Currently this will create the symlink, but log and error when it's used. In
// the future, I expect the symlink to be removed, and this will be equivalent
// to Forbidden.
var Missing = PathConfig{
	Symlink: true,
	Log:     true,
	Error:   true,
}

var LinuxOnlyPrebuilt = PathConfig{
	Symlink:           false,
	Log:               true,
	Error:             true,
	LinuxOnlyPrebuilt: true,
}

func GetConfig(name string) PathConfig {
	if config, ok := Configuration[name]; ok {
		return config
	}
	return Missing
}

var Configuration = map[string]PathConfig{
	"bash":     Allowed,
	"bc":       Allowed,
	"bzip2":    Allowed,
	"date":     Allowed,
	"dd":       Allowed,
	"diff":     Allowed,
	"egrep":    Allowed,
	"expr":     Allowed,
	"find":     Allowed,
	"fuser":    Allowed,
	"getopt":   Allowed,
	"git":      Allowed,
	"grep":     Allowed,
	"gzip":     Allowed,
	"hexdump":  Allowed,
	"jar":      Allowed,
	"java":     Allowed,
	"javap":    Allowed,
	"lsof":     Allowed,
	"m4":       Allowed,
	"nproc":    Allowed,
	"openssl":  Allowed,
	"patch":    Allowed,
	"pstree":   Allowed,
	"python3":  Allowed,
	"realpath": Allowed,
	"rsync":    Allowed,
	"sed":      Allowed,
	"sh":       Allowed,
	"tar":      Allowed,
	"timeout":  Allowed,
	"tr":       Allowed,
	"unzip":    Allowed,
	"xz":       Allowed,
	"zip":      Allowed,
	"zipinfo":  Allowed,

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

	// On Linux we'll use the toybox versions of these instead.
	"basename":  LinuxOnlyPrebuilt,
	"cat":       LinuxOnlyPrebuilt,
	"chmod":     LinuxOnlyPrebuilt,
	"cmp":       LinuxOnlyPrebuilt,
	"cp":        LinuxOnlyPrebuilt,
	"comm":      LinuxOnlyPrebuilt,
	"cut":       LinuxOnlyPrebuilt,
	"dirname":   LinuxOnlyPrebuilt,
	"du":        LinuxOnlyPrebuilt,
	"echo":      LinuxOnlyPrebuilt,
	"env":       LinuxOnlyPrebuilt,
	"head":      LinuxOnlyPrebuilt,
	"getconf":   LinuxOnlyPrebuilt,
	"hostname":  LinuxOnlyPrebuilt,
	"id":        LinuxOnlyPrebuilt,
	"ln":        LinuxOnlyPrebuilt,
	"ls":        LinuxOnlyPrebuilt,
	"md5sum":    LinuxOnlyPrebuilt,
	"mkdir":     LinuxOnlyPrebuilt,
	"mktemp":    LinuxOnlyPrebuilt,
	"mv":        LinuxOnlyPrebuilt,
	"od":        LinuxOnlyPrebuilt,
	"paste":     LinuxOnlyPrebuilt,
	"pgrep":     LinuxOnlyPrebuilt,
	"pkill":     LinuxOnlyPrebuilt,
	"ps":        LinuxOnlyPrebuilt,
	"pwd":       LinuxOnlyPrebuilt,
	"readlink":  LinuxOnlyPrebuilt,
	"rm":        LinuxOnlyPrebuilt,
	"rmdir":     LinuxOnlyPrebuilt,
	"seq":       LinuxOnlyPrebuilt,
	"setsid":    LinuxOnlyPrebuilt,
	"sha1sum":   LinuxOnlyPrebuilt,
	"sha256sum": LinuxOnlyPrebuilt,
	"sha512sum": LinuxOnlyPrebuilt,
	"sleep":     LinuxOnlyPrebuilt,
	"sort":      LinuxOnlyPrebuilt,
	"stat":      LinuxOnlyPrebuilt,
	"tail":      LinuxOnlyPrebuilt,
	"tee":       LinuxOnlyPrebuilt,
	"touch":     LinuxOnlyPrebuilt,
	"true":      LinuxOnlyPrebuilt,
	"uname":     LinuxOnlyPrebuilt,
	"uniq":      LinuxOnlyPrebuilt,
	"unix2dos":  LinuxOnlyPrebuilt,
	"wc":        LinuxOnlyPrebuilt,
	"whoami":    LinuxOnlyPrebuilt,
	"which":     LinuxOnlyPrebuilt,
	"xargs":     LinuxOnlyPrebuilt,
	"xxd":       LinuxOnlyPrebuilt,
}

func init() {
	if runtime.GOOS == "darwin" {
		Configuration["md5"] = Allowed
		Configuration["sw_vers"] = Allowed
		Configuration["xcrun"] = Allowed

		// We don't have darwin prebuilts for some tools (like toybox),
		// so allow the host versions.
		for name, config := range Configuration {
			if config.LinuxOnlyPrebuilt {
				Configuration[name] = Allowed
			}
		}
	}
}
