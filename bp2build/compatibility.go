package bp2build

import (
	"fmt"
)

// Data from the code generation process that is used to improve compatibility
// between build systems.
type CodegenCompatLayer struct {
	// A map from the original module name to the generated/handcrafted Bazel
	// label for legacy build systems to be able to build a fully-qualified
	// Bazel target from an unique module name.
	NameToLabelMap map[string]string
}

// Log an entry of module name -> Bazel target label.
func (compatLayer CodegenCompatLayer) AddNameToLabelEntry(name, label string) {
	if existingLabel, ok := compatLayer.NameToLabelMap[name]; ok {
		panic(fmt.Errorf(
			"Module '%s' maps to more than one Bazel target label: %s, %s. "+
				"This shouldn't happen. It probably indicates a bug with the bp2build internals.",
			name,
			existingLabel,
			label))
	}
	compatLayer.NameToLabelMap[name] = label
}
