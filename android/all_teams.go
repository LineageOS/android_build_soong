package android

import (
	"android/soong/android/team_proto"
	"path/filepath"

	"google.golang.org/protobuf/proto"
)

const ownershipDirectory = "ownership"
const allTeamsFile = "all_teams.pb"

func AllTeamsFactory() Singleton {
	return &allTeamsSingleton{}
}

func init() {
	registerAllTeamBuildComponents(InitRegistrationContext)
}

func registerAllTeamBuildComponents(ctx RegistrationContext) {
	ctx.RegisterParallelSingletonType("all_teams", AllTeamsFactory)
}

// For each module, list the team or the bpFile the module is defined in.
type moduleTeamAndTestInfo struct {
	// Name field from bp file for the team
	teamName string
	// Blueprint file the module is located in.
	bpFile string
	// Is this module only used by tests.
	testOnly bool
	// Is this a directly testable target by running the module directly
	// or via tradefed.
	topLevelTestTarget bool
	// String name indicating the module, like `java_library` for reporting.
	kind string
}

type allTeamsSingleton struct {
	// Path where the collected metadata is stored after successful validation.
	outputPath OutputPath

	// Map of all package modules we visit during GenerateBuildActions
	packages map[string]packageProperties
	// Map of all team modules we visit during GenerateBuildActions
	teams map[string]teamProperties
	// Keeps track of team information or bp file for each module we visit.
	teams_for_mods map[string]moduleTeamAndTestInfo
}

// See if there is a package module for the given bpFilePath with a team defined, if so return the team.
// If not ascend up to the parent directory and do the same.
func (t *allTeamsSingleton) lookupDefaultTeam(bpFilePath string) (teamProperties, bool) {
	// return the Default_team listed in the package if is there.
	if p, ok := t.packages[bpFilePath]; ok {
		if defaultTeam := p.Default_team; defaultTeam != nil {
			return t.teams[*defaultTeam], true
		}
	}
	// Strip a directory and go up.
	// Does android/paths.go basePath,SourcePath help?
	current, base := filepath.Split(bpFilePath)
	current = filepath.Clean(current) // removes trailing slash, convert "" -> "."
	parent, _ := filepath.Split(current)
	if current == "." {
		return teamProperties{}, false
	}
	return t.lookupDefaultTeam(filepath.Join(parent, base))
}

// Visit all modules and collect all teams and use WriteFileRuleVerbatim
// to write it out.
func (t *allTeamsSingleton) GenerateBuildActions(ctx SingletonContext) {
	t.packages = make(map[string]packageProperties)
	t.teams = make(map[string]teamProperties)
	t.teams_for_mods = make(map[string]moduleTeamAndTestInfo)

	ctx.VisitAllModules(func(module Module) {
		bpFile := ctx.BlueprintFile(module)

		// Package Modules and Team Modules are stored in a map so we can look them up by name for
		// modules without a team.
		if pack, ok := module.(*packageModule); ok {
			// Packages don't have names, use the blueprint file as the key. we can't get qualifiedModuleId in t context.
			pkgKey := bpFile
			t.packages[pkgKey] = pack.properties
			return
		}
		if team, ok := module.(*teamModule); ok {
			t.teams[team.Name()] = team.properties
			return
		}

		testModInfo := TestModuleInformation{}
		if tmi, ok := SingletonModuleProvider(ctx, module, TestOnlyProviderKey); ok {
			testModInfo = tmi
		}

		// Some modules, like java_test_host don't set the provider when the module isn't enabled:
		//                                                test_only, top_level
		//     AVFHostTestCases{os:linux_glibc,arch:common} {true true}
		//     AVFHostTestCases{os:windows,arch:common} {false false}
		// Generally variant information of true override false or unset.
		if testModInfo.TestOnly == false {
			if prevValue, exists := t.teams_for_mods[module.Name()]; exists {
				if prevValue.testOnly == true {
					return
				}
			}
		}
		entry := moduleTeamAndTestInfo{
			bpFile:             bpFile,
			testOnly:           testModInfo.TestOnly,
			topLevelTestTarget: testModInfo.TopLevelTarget,
			kind:               ctx.ModuleType(module),
			teamName:           module.base().Team(),
		}
		t.teams_for_mods[module.Name()] = entry

	})

	// Visit all modules again and lookup the team name in the package or parent package if the team
	// isn't assignged at the module level.
	allTeams := t.lookupTeamForAllModules()

	t.outputPath = PathForOutput(ctx, ownershipDirectory, allTeamsFile)
	data, err := proto.Marshal(allTeams)
	if err != nil {
		ctx.Errorf("Unable to marshal team data. %s", err)
	}

	WriteFileRuleVerbatim(ctx, t.outputPath, string(data))
	ctx.Phony("all_teams", t.outputPath)
}

func (t *allTeamsSingleton) MakeVars(ctx MakeVarsContext) {
	ctx.DistForGoal("all_teams", t.outputPath)
}

// Visit every (non-package, non-team) module and write out a proto containing
// either the declared team data for that module or the package default team data for that module.
func (t *allTeamsSingleton) lookupTeamForAllModules() *team_proto.AllTeams {
	teamsProto := make([]*team_proto.Team, len(t.teams_for_mods))
	for i, moduleName := range SortedKeys(t.teams_for_mods) {
		m, _ := t.teams_for_mods[moduleName]
		teamName := m.teamName
		var teamProperties teamProperties
		found := false
		if teamName != "" {
			teamProperties, found = t.teams[teamName]
		} else {
			teamProperties, found = t.lookupDefaultTeam(m.bpFile)
		}

		trendy_team_id := ""
		if found {
			trendy_team_id = *teamProperties.Trendy_team_id
		}

		teamData := new(team_proto.Team)
		*teamData = team_proto.Team{
			TargetName:     proto.String(moduleName),
			Path:           proto.String(m.bpFile),
			TestOnly:       proto.Bool(m.testOnly),
			TopLevelTarget: proto.Bool(m.topLevelTestTarget),
			Kind:           proto.String(m.kind),
		}
		if trendy_team_id != "" {
			teamData.TrendyTeamId = proto.String(trendy_team_id)
		} else {
			// Clients rely on the TrendyTeamId optional field not being set.
		}
		teamsProto[i] = teamData
	}
	return &team_proto.AllTeams{Teams: teamsProto}
}
