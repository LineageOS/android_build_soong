package bazel

type RunName string

// Below is a list bazel execution run names used through out the
// Platform Build systems. Each run name represents an unique key
// to query the bazel metrics.
const (
	// Perform a bazel build of the phony root to generate symlink forests
	// for dependencies of the bazel build.
	BazelBuildPhonyRootRunName = RunName("bazel-build-phony-root")

	// Perform aquery of the bazel build root to retrieve action information.
	AqueryBuildRootRunName = RunName("aquery-buildroot")

	// Perform cquery of the Bazel build root and its dependencies.
	CqueryBuildRootRunName = RunName("cquery-buildroot")

	// Run bazel as a ninja executer
	BazelNinjaExecRunName = RunName("bazel-ninja-exec")
)

// String returns the name of the run.
func (c RunName) String() string {
	return string(c)
}
