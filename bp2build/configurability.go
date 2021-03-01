package bp2build

import "android/soong/android"

// Configurability support for bp2build.

var (
	// A map of architectures to the Bazel label of the constraint_value.
	platformArchMap = map[android.ArchType]string{
		android.Arm:    "@bazel_tools//platforms:arm",
		android.Arm64:  "@bazel_tools//platforms:aarch64",
		android.X86:    "@bazel_tools//platforms:x86_32",
		android.X86_64: "@bazel_tools//platforms:x86_64",
	}
)
