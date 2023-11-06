package android

import (
	"encoding/json"
	"io"
	"slices"

	"github.com/google/blueprint"
)

type CoreModuleInfoJSON struct {
	RegisterName       string   `json:"-"`
	Path               []string `json:"path,omitempty"`                // $(sort $(ALL_MODULES.$(m).PATH))
	Installed          []string `json:"installed,omitempty"`           // $(sort $(ALL_MODULES.$(m).INSTALLED))
	ModuleName         string   `json:"module_name,omitempty"`         // $(ALL_MODULES.$(m).MODULE_NAME)
	SupportedVariants  []string `json:"supported_variants,omitempty"`  // $(sort $(ALL_MODULES.$(m).SUPPORTED_VARIANTS))
	HostDependencies   []string `json:"host_dependencies,omitempty"`   // $(sort $(ALL_MODULES.$(m).HOST_REQUIRED_FROM_TARGET))
	TargetDependencies []string `json:"target_dependencies,omitempty"` // $(sort $(ALL_MODULES.$(m).TARGET_REQUIRED_FROM_HOST))
	Data               []string `json:"data,omitempty"`                // $(sort $(ALL_MODULES.$(m).TEST_DATA))
}

type ModuleInfoJSON struct {
	core                CoreModuleInfoJSON
	SubName             string   `json:"-"`
	Uninstallable       bool     `json:"-"`
	Class               []string `json:"class,omitempty"`                 // $(sort $(ALL_MODULES.$(m).CLASS))
	Tags                []string `json:"tags,omitempty"`                  // $(sort $(ALL_MODULES.$(m).TAGS))
	Dependencies        []string `json:"dependencies,omitempty"`          // $(sort $(ALL_DEPS.$(m).ALL_DEPS))
	SharedLibs          []string `json:"shared_libs,omitempty"`           // $(sort $(ALL_MODULES.$(m).SHARED_LIBS))
	StaticLibs          []string `json:"static_libs,omitempty"`           // $(sort $(ALL_MODULES.$(m).STATIC_LIBS))
	SystemSharedLibs    []string `json:"system_shared_libs,omitempty"`    // $(sort $(ALL_MODULES.$(m).SYSTEM_SHARED_LIBS))
	Srcs                []string `json:"srcs,omitempty"`                  // $(sort $(ALL_MODULES.$(m).SRCS))
	SrcJars             []string `json:"srcjars,omitempty"`               // $(sort $(ALL_MODULES.$(m).SRCJARS))
	ClassesJar          []string `json:"classes_jar,omitempty"`           // $(sort $(ALL_MODULES.$(m).CLASSES_JAR))
	TestMainlineModules []string `json:"test_mainline_modules,omitempty"` // $(sort $(ALL_MODULES.$(m).TEST_MAINLINE_MODULES))
	IsUnitTest          bool     `json:"is_unit_test,omitempty"`          // $(ALL_MODULES.$(m).IS_UNIT_TEST)
	TestOptionsTags     []string `json:"test_options_tags,omitempty"`     // $(sort $(ALL_MODULES.$(m).TEST_OPTIONS_TAGS))
	RuntimeDependencies []string `json:"runtime_dependencies,omitempty"`  // $(sort $(ALL_MODULES.$(m).LOCAL_RUNTIME_LIBRARIES))
	StaticDependencies  []string `json:"static_dependencies,omitempty"`   // $(sort $(ALL_MODULES.$(m).LOCAL_STATIC_LIBRARIES))
	DataDependencies    []string `json:"data_dependencies,omitempty"`     // $(sort $(ALL_MODULES.$(m).TEST_DATA_BINS))

	CompatibilitySuites []string `json:"compatibility_suites,omitempty"` // $(sort $(ALL_MODULES.$(m).COMPATIBILITY_SUITES))
	AutoTestConfig      []string `json:"auto_test_config,omitempty"`     // $(ALL_MODULES.$(m).auto_test_config)
	TestConfig          []string `json:"test_config,omitempty"`          // $(strip $(ALL_MODULES.$(m).TEST_CONFIG) $(ALL_MODULES.$(m).EXTRA_TEST_CONFIGS)
}

//ALL_DEPS.$(LOCAL_MODULE).ALL_DEPS := $(sort \
//$(ALL_DEPS.$(LOCAL_MODULE).ALL_DEPS) \
//$(LOCAL_STATIC_LIBRARIES) \
//$(LOCAL_WHOLE_STATIC_LIBRARIES) \
//$(LOCAL_SHARED_LIBRARIES) \
//$(LOCAL_DYLIB_LIBRARIES) \
//$(LOCAL_RLIB_LIBRARIES) \
//$(LOCAL_PROC_MACRO_LIBRARIES) \
//$(LOCAL_HEADER_LIBRARIES) \
//$(LOCAL_STATIC_JAVA_LIBRARIES) \
//$(LOCAL_JAVA_LIBRARIES) \
//$(LOCAL_JNI_SHARED_LIBRARIES))

type combinedModuleInfoJSON struct {
	*CoreModuleInfoJSON
	*ModuleInfoJSON
}

func encodeModuleInfoJSON(w io.Writer, moduleInfoJSON *ModuleInfoJSON) error {
	moduleInfoJSONCopy := *moduleInfoJSON

	sortAndUnique := func(s *[]string) {
		*s = slices.Clone(*s)
		slices.Sort(*s)
		*s = slices.Compact(*s)
	}

	sortAndUnique(&moduleInfoJSONCopy.core.Path)
	sortAndUnique(&moduleInfoJSONCopy.core.Installed)
	sortAndUnique(&moduleInfoJSONCopy.core.SupportedVariants)
	sortAndUnique(&moduleInfoJSONCopy.core.HostDependencies)
	sortAndUnique(&moduleInfoJSONCopy.core.TargetDependencies)
	sortAndUnique(&moduleInfoJSONCopy.core.Data)

	sortAndUnique(&moduleInfoJSONCopy.Class)
	sortAndUnique(&moduleInfoJSONCopy.Tags)
	sortAndUnique(&moduleInfoJSONCopy.Dependencies)
	sortAndUnique(&moduleInfoJSONCopy.SharedLibs)
	sortAndUnique(&moduleInfoJSONCopy.StaticLibs)
	sortAndUnique(&moduleInfoJSONCopy.SystemSharedLibs)
	sortAndUnique(&moduleInfoJSONCopy.Srcs)
	sortAndUnique(&moduleInfoJSONCopy.SrcJars)
	sortAndUnique(&moduleInfoJSONCopy.ClassesJar)
	sortAndUnique(&moduleInfoJSONCopy.TestMainlineModules)
	sortAndUnique(&moduleInfoJSONCopy.TestOptionsTags)
	sortAndUnique(&moduleInfoJSONCopy.RuntimeDependencies)
	sortAndUnique(&moduleInfoJSONCopy.StaticDependencies)
	sortAndUnique(&moduleInfoJSONCopy.DataDependencies)
	sortAndUnique(&moduleInfoJSONCopy.CompatibilitySuites)
	sortAndUnique(&moduleInfoJSONCopy.AutoTestConfig)
	sortAndUnique(&moduleInfoJSONCopy.TestConfig)

	encoder := json.NewEncoder(w)
	return encoder.Encode(combinedModuleInfoJSON{&moduleInfoJSONCopy.core, &moduleInfoJSONCopy})
}

var ModuleInfoJSONProvider = blueprint.NewProvider[*ModuleInfoJSON]()
