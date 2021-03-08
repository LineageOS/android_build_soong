package shared

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

var (
	isDebugging bool
)

// Finds the Delve binary to use. Either uses the SOONG_DELVE_PATH environment
// variable or if that is unset, looks at $PATH.
func ResolveDelveBinary() string {
	result := os.Getenv("SOONG_DELVE_PATH")
	if result == "" {
		result, _ = exec.LookPath("dlv")
	}

	return result
}

// Returns whether the current process is running under Delve due to
// ReexecWithDelveMaybe().
func IsDebugging() bool {
	return isDebugging
}

// Re-executes the binary in question under the control of Delve when
// delveListen is not the empty string. delvePath gives the path to the Delve.
func ReexecWithDelveMaybe(delveListen, delvePath string) {
	isDebugging = os.Getenv("SOONG_DELVE_REEXECUTED") == "true"
	if isDebugging || delveListen == "" {
		return
	}

	if delvePath == "" {
		fmt.Fprintln(os.Stderr, "Delve debugging requested but failed to find dlv")
		os.Exit(1)
	}

	soongDelveEnv := []string{}
	for _, env := range os.Environ() {
		idx := strings.IndexRune(env, '=')
		if idx != -1 {
			soongDelveEnv = append(soongDelveEnv, env)
		}
	}

	soongDelveEnv = append(soongDelveEnv, "SOONG_DELVE_REEXECUTED=true")

	dlvArgv := []string{
		delvePath,
		"--listen=:" + delveListen,
		"--headless=true",
		"--api-version=2",
		"exec",
		os.Args[0],
		"--",
	}

	dlvArgv = append(dlvArgv, os.Args[1:]...)
	syscall.Exec(delvePath, dlvArgv, soongDelveEnv)
	fmt.Fprintln(os.Stderr, "exec() failed while trying to reexec with Delve")
	os.Exit(1)
}
