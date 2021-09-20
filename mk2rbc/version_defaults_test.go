package mk2rbc

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseVersionDefaults(t *testing.T) {
	testDir := getTestDirectory()
	abspath := func(relPath string) string { return filepath.Join(testDir, relPath) }
	actualProducts, err := ParseVersionDefaults(abspath("version_defaults.mk.test"))
	if err != nil {
		t.Fatal(err)
	}
	expectedProducts := map[string]string{
		"DEFAULT_PLATFORM_VERSION":            "TP1A",
		"MAX_PLATFORM_VERSION":                "TP1A",
		"MIN_PLATFORM_VERSION":                "TP1A",
		"PLATFORM_BASE_SDK_EXTENSION_VERSION": "0",
		"PLATFORM_SDK_EXTENSION_VERSION":      "1",
		"PLATFORM_SDK_VERSION":                "31",
		"PLATFORM_SECURITY_PATCH":             "2021-10-05",
		"PLATFORM_VERSION_LAST_STABLE":        "12",
		"PLATFORM_VERSION_CODENAME.SP2A":      "Sv2",
		"PLATFORM_VERSION_CODENAME.TP1A":      "Tiramisu",
	}
	if !reflect.DeepEqual(actualProducts, expectedProducts) {
		t.Errorf("\nExpected: %v\n  Actual: %v", expectedProducts, actualProducts)
	}
}

func TestVersionDefaults(t *testing.T) {
	testDir := getTestDirectory()
	abspath := func(relPath string) string { return filepath.Join(testDir, relPath) }
	actualProducts, err := ParseVersionDefaults(abspath("version_defaults.mk.test"))
	if err != nil {
		t.Fatal(err)
	}
	expectedString := `version_defaults = struct(
    default_platform_version = "TP1A",
    max_platform_version = "TP1A",
    min_platform_version = "TP1A",
    platform_base_sdk_extension_version = 0,
    platform_sdk_extension_version = 1,
    platform_sdk_version = 31,
    platform_security_patch = "2021-10-05",
    platform_version_last_stable = 12,
    codenames = { "SP2A": "Sv2", "TP1A": "Tiramisu" }
)
`
	actualString := VersionDefaults(actualProducts)
	if !reflect.DeepEqual(actualString, expectedString) {
		t.Errorf("\nExpected: %v\nActual:\n%v",
			strings.ReplaceAll(expectedString, "\n", "␤\n"),
			strings.ReplaceAll(actualString, "\n", "␤\n"))
	}

}
