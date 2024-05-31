package tradefed

import (
	"android/soong/android"

	"github.com/google/blueprint"
)

// Output files we need from a base test that we derive from.
type BaseTestProviderData struct {
	// data files and apps for android_test
	InstalledFiles android.Paths
	// apk for android_test
	OutputFile android.Path
	// Either handwritten or generated TF xml.
	TestConfig android.Path
	// Other modules we require to be installed to run tests. We expect base to build them.
	HostRequiredModuleNames []string
	RequiredModuleNames     []string
	// List of test suites base uses.
	TestSuites []string
	// Used for bases that are Host
	IsHost bool
}

var BaseTestProviderKey = blueprint.NewProvider[BaseTestProviderData]()
