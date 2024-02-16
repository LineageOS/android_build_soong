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
type moduleTeamInfo struct {
	teamName string
	bpFile   string
}

type allTeamsSingleton struct {
	// Path where the collected metadata is stored after successful validation.
	outputPath OutputPath

	// Map of all package modules we visit during GenerateBuildActions
	packages map[string]packageProperties
	// Map of all team modules we visit during GenerateBuildActions
	teams map[string]teamProperties
	// Keeps track of team information or bp file for each module we visit.
	teams_for_mods map[string]moduleTeamInfo
}

// See if there is a package module for the given bpFilePath with a team defined, if so return the team.
// If not ascend up to the parent directory and do the same.
func (this *allTeamsSingleton) lookupDefaultTeam(bpFilePath string) (teamProperties, bool) {
	// return the Default_team listed in the package if is there.
	if p, ok := this.packages[bpFilePath]; ok {
		if t := p.Default_team; t != nil {
			return this.teams[*p.Default_team], true
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
	return this.lookupDefaultTeam(filepath.Join(parent, base))
}

// Create a rule to run a tool to collect all the intermediate files
// which list the team per module into one proto file.
func (this *allTeamsSingleton) GenerateBuildActions(ctx SingletonContext) {
	this.packages = make(map[string]packageProperties)
	this.teams = make(map[string]teamProperties)
	this.teams_for_mods = make(map[string]moduleTeamInfo)

	ctx.VisitAllModules(func(module Module) {
		bpFile := ctx.BlueprintFile(module)

		// Package Modules and Team Modules are stored in a map so we can look them up by name for
		// modules without a team.
		if pack, ok := module.(*packageModule); ok {
			// Packages don't have names, use the blueprint file as the key. we can't get qualifiedModuleId in this context.
			pkgKey := bpFile
			this.packages[pkgKey] = pack.properties
			return
		}
		if team, ok := module.(*teamModule); ok {
			this.teams[team.Name()] = team.properties
			return
		}

		// If a team name is given for a module, store it.
		// Otherwise store the bpFile so we can do a package walk later.
		if module.base().Team() != "" {
			this.teams_for_mods[module.Name()] = moduleTeamInfo{teamName: module.base().Team(), bpFile: bpFile}
		} else {
			this.teams_for_mods[module.Name()] = moduleTeamInfo{bpFile: bpFile}
		}
	})

	// Visit all modules again and lookup the team name in the package or parent package if the team
	// isn't assignged at the module level.
	allTeams := this.lookupTeamForAllModules()

	this.outputPath = PathForOutput(ctx, ownershipDirectory, allTeamsFile)
	data, err := proto.Marshal(allTeams)
	if err != nil {
		ctx.Errorf("Unable to marshal team data. %s", err)
	}

	WriteFileRuleVerbatim(ctx, this.outputPath, string(data))
	ctx.Phony("all_teams", this.outputPath)
}

func (this *allTeamsSingleton) MakeVars(ctx MakeVarsContext) {
	ctx.DistForGoal("all_teams", this.outputPath)
}

// Visit every (non-package, non-team) module and write out a proto containing
// either the declared team data for that module or the package default team data for that module.
func (this *allTeamsSingleton) lookupTeamForAllModules() *team_proto.AllTeams {
	teamsProto := make([]*team_proto.Team, len(this.teams_for_mods))
	for i, moduleName := range SortedKeys(this.teams_for_mods) {
		m, _ := this.teams_for_mods[moduleName]
		teamName := m.teamName
		var teamProperties teamProperties
		found := false
		if teamName != "" {
			teamProperties, found = this.teams[teamName]
		} else {
			teamProperties, found = this.lookupDefaultTeam(m.bpFile)
		}

		trendy_team_id := ""
		if found {
			trendy_team_id = *teamProperties.Trendy_team_id
		}

		var files []string
		teamData := new(team_proto.Team)
		if trendy_team_id != "" {
			*teamData = team_proto.Team{
				TrendyTeamId: proto.String(trendy_team_id),
				TargetName:   proto.String(moduleName),
				Path:         proto.String(m.bpFile),
				File:         files,
			}
		} else {
			// Clients rely on the TrendyTeamId optional field not being set.
			*teamData = team_proto.Team{
				TargetName: proto.String(moduleName),
				Path:       proto.String(m.bpFile),
				File:       files,
			}
		}
		teamsProto[i] = teamData
	}
	return &team_proto.AllTeams{Teams: teamsProto}
}
