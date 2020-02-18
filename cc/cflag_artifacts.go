package cc

import (
	"fmt"
	"sort"
	"strings"

	"github.com/google/blueprint/proptools"

	"android/soong/android"
)

func init() {
	android.RegisterSingletonType("cflag_artifacts_text", cflagArtifactsTextFactory)
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

const FileBP = 50

// Stores output files.
type cflagArtifactsText struct {
	interOutputs map[string]android.WritablePaths
	outputs      android.WritablePaths
}

// allowedDir verifies if the directory/project is part of the TrackedCFlagsDir
// filter.
func allowedDir(subdir string) bool {
	subdir += "/"
	return android.HasAnyPrefix(subdir, TrackedCFlagsDir)
}

func (s *cflagArtifactsText) genFlagFilename(flag string) string {
	return fmt.Sprintf("module_cflags%s.txt", flag)
}

// incrementFile is used to generate an output path object with the passed in flag
// and part number.
// e.g. FLAG + part # -> out/soong/cflags/module_cflags-FLAG.txt.0
func (s *cflagArtifactsText) incrementFile(ctx android.SingletonContext,
	flag string, part int) (string, android.OutputPath) {

	filename := fmt.Sprintf("%s.%d", s.genFlagFilename(flag), part)
	filepath := android.PathForOutput(ctx, "cflags", filename)
	s.interOutputs[flag] = append(s.interOutputs[flag], filepath)
	return filename, filepath
}

// GenCFlagArtifactParts is used to generate the build rules which produce the
// intermediary files for each desired C Flag artifact
// e.g. module_cflags-FLAG.txt.0, module_cflags-FLAG.txt.1, ...
func (s *cflagArtifactsText) GenCFlagArtifactParts(ctx android.SingletonContext,
	flag string, using bool, modules []string, part int) int {

	cleanedName := strings.Replace(flag, "=", "_", -1)
	filename, filepath := s.incrementFile(ctx, cleanedName, part)
	rule := android.NewRuleBuilder()
	rule.Command().Textf("rm -f %s", filepath.String())

	if using {
		rule.Command().
			Textf("echo '# Modules using %s'", flag).
			FlagWithOutput(">> ", filepath)
	} else {
		rule.Command().
			Textf("echo '# Modules not using %s'", flag).
			FlagWithOutput(">> ", filepath)
	}

	length := len(modules)

	if length == 0 {
		rule.Build(pctx, ctx, filename, "gen "+filename)
		part++
	}

	// Following loop splits the module list for each tracked C Flag into
	// chunks of length FileBP (file breakpoint) and generates a partial artifact
	// (intermediary file) build rule for each split.
	moduleShards := android.ShardStrings(modules, FileBP)
	for index, shard := range moduleShards {
		rule.Command().
			Textf("for m in %s; do echo $m",
				strings.Join(proptools.ShellEscapeList(shard), " ")).
			FlagWithOutput(">> ", filepath).
			Text("; done")
		rule.Build(pctx, ctx, filename, "gen "+filename)

		if index+1 != len(moduleShards) {
			filename, filepath = s.incrementFile(ctx, cleanedName, part+index+1)
			rule = android.NewRuleBuilder()
			rule.Command().Textf("rm -f %s", filepath.String())
		}
	}

	return part + len(moduleShards)
}

// GenCFlagArtifacts is used to generate build rules which combine the
// intermediary files of a specific tracked flag into a single C Flag artifact
// for each tracked flag.
// e.g. module_cflags-FLAG.txt.0 + module_cflags-FLAG.txt.1 = module_cflags-FLAG.txt
func (s *cflagArtifactsText) GenCFlagArtifacts(ctx android.SingletonContext) {
	// Scans through s.interOutputs and creates a build rule for each tracked C
	// Flag that concatenates the associated intermediary file into a single
	// artifact.
	for _, flag := range TrackedCFlags {
		// Generate build rule to combine related intermediary files into a
		// C Flag artifact
		rule := android.NewRuleBuilder()
		filename := s.genFlagFilename(flag)
		outputpath := android.PathForOutput(ctx, "cflags", filename)
		rule.Command().
			Text("cat").
			Inputs(s.interOutputs[flag].Paths()).
			FlagWithOutput("> ", outputpath)
		rule.Build(pctx, ctx, filename, "gen "+filename)
		s.outputs = append(s.outputs, outputpath)
	}
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
		part := s.GenCFlagArtifactParts(ctx, flag, true, modulesWithCFlag[flag], 0)
		sort.Strings(modulesWithCFlag["!"+flag])
		s.GenCFlagArtifactParts(ctx, flag, false, modulesWithCFlag["!"+flag], part)
	}

	// Combine intermediary files into a single C Flag artifact.
	s.GenCFlagArtifacts(ctx)
}

func cflagArtifactsTextFactory() android.Singleton {
	return &cflagArtifactsText{
		interOutputs: make(map[string]android.WritablePaths),
	}
}

func (s *cflagArtifactsText) MakeVars(ctx android.MakeVarsContext) {
	ctx.Strict("SOONG_MODULES_CFLAG_ARTIFACTS", strings.Join(s.outputs.Strings(), " "))
}
