package android

import "github.com/google/blueprint/proptools"

// CreateSelectOsToBool is a utility function that makes it easy to create a
// Configurable property value that maps from os to a bool. Use an empty string
// to indicate a "default" case.
func CreateSelectOsToBool(cases map[string]*bool) proptools.Configurable[bool] {
	var resultCases []proptools.ConfigurableCase[bool]
	for pattern, value := range cases {
		if pattern == "" {
			resultCases = append(resultCases, proptools.NewConfigurableCase(
				[]proptools.ConfigurablePattern{proptools.NewDefaultConfigurablePattern()},
				value,
			))
		} else {
			resultCases = append(resultCases, proptools.NewConfigurableCase(
				[]proptools.ConfigurablePattern{proptools.NewStringConfigurablePattern(pattern)},
				value,
			))
		}
	}

	return proptools.NewConfigurable(
		[]proptools.ConfigurableCondition{proptools.NewConfigurableCondition("os", nil)},
		resultCases,
	)
}
