package shared

import (
	"os"
	"os/exec"
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
