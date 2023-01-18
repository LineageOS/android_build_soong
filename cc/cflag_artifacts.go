package cc

import (
	"fmt"
	"sort"
	"strings"

	"android/soong/android"
)

func init() {
	android.RegisterSingletonType("cflag_artifacts_text", cflagArtifactsTextFactory)
}

func cflagArtifactsTextFactory() android.Singleton {
	return &cflagArtifactsText{}
}

var (
	TrackedCFlags = []string{
		"-Wall",
		"-Werror",
		"-Wextra",
		"-Wthread-safety",
		"-O3",
	}

	TrackedCFlagsDir = []string{
		"device/google/",
		"vendor/google/",
	}
)

// Stores output files.
type cflagArtifactsText struct {
	outputs android.WritablePaths
}

// allowedDir verifies if the directory/project is part of the TrackedCFlagsDir
// filter.
func allowedDir(subdir string) bool {
	subdir += "/"
	return android.HasAnyPrefix(subdir, TrackedCFlagsDir)
}

// GenCFlagArtifact is used to generate the build rules which produce a file
// that contains a list of all modules using/not using a particular cflag
func (s *cflagArtifactsText) GenCFlagArtifact(ctx android.SingletonContext,
	flag string, modulesUsing, modulesNotUsing []string) {

	filename := "module_cflags" + flag + ".txt"
	filepath := android.PathForOutput(ctx, "cflags", filename)

	lines := make([]string, 0, 2+len(modulesUsing)+len(modulesNotUsing))
	lines = append(lines, "# Modules using "+flag)
	lines = append(lines, modulesUsing...)
	lines = append(lines, "# Modules not using "+flag)
	lines = append(lines, modulesNotUsing...)

	android.WriteFileRule(ctx, filepath, strings.Join(lines, "\n"))
	s.outputs = append(s.outputs, filepath)
}

func (s *cflagArtifactsText) GenerateBuildActions(ctx android.SingletonContext) {
	modulesWithCFlag := make(map[string][]string)

	// Scan through all modules, selecting the ones that are part of the filter,
	// and then storing into a map which tracks whether or not tracked C flag is
	// used or not.
	ctx.VisitAllModules(func(module android.Module) {
		if ccModule, ok := module.(*Module); ok {
			if allowedDir(ctx.ModuleDir(ccModule)) {
				cflags := ccModule.flags.Local.CFlags
				cppflags := ccModule.flags.Local.CppFlags
				module := fmt.Sprintf("%s:%s (%s)",
					ctx.BlueprintFile(ccModule),
					ctx.ModuleName(ccModule),
					ctx.ModuleSubDir(ccModule))
				for _, flag := range TrackedCFlags {
					if inList(flag, cflags) || inList(flag, cppflags) {
						modulesWithCFlag[flag] = append(modulesWithCFlag[flag], module)
					} else {
						modulesWithCFlag["!"+flag] = append(modulesWithCFlag["!"+flag], module)
					}
				}
			}
		}
	})

	// Traversing map and setting up rules to produce intermediary files which
	// contain parts of each expected C Flag artifact.
	for _, flag := range TrackedCFlags {
		sort.Strings(modulesWithCFlag[flag])
		sort.Strings(modulesWithCFlag["!"+flag])
		s.GenCFlagArtifact(ctx, flag, modulesWithCFlag[flag], modulesWithCFlag["!"+flag])
	}
}

func (s *cflagArtifactsText) MakeVars(ctx android.MakeVarsContext) {
	ctx.Strict("SOONG_MODULES_CFLAG_ARTIFACTS", strings.Join(s.outputs.Strings(), " "))
}
