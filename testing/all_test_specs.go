package testing

import (
	"android/soong/android"
)

const ownershipDirectory = "ownership"
const fileContainingFilePaths = "all_test_spec_paths.rsp"
const allTestSpecsFile = "all_test_specs.pb"

func AllTestSpecsFactory() android.Singleton {
	return &allTestSpecsSingleton{}
}

type allTestSpecsSingleton struct {
	// Path where the collected metadata is stored after successful validation.
	outputPath android.OutputPath
}

func (this *allTestSpecsSingleton) GenerateBuildActions(ctx android.SingletonContext) {
	var intermediateMetadataPaths android.Paths

	ctx.VisitAllModules(func(module android.Module) {
		if !ctx.ModuleHasProvider(module, TestSpecProviderKey) {
			return
		}
		intermediateMetadataPaths = append(intermediateMetadataPaths, ctx.ModuleProvider(module, TestSpecProviderKey).(TestSpecProviderData).IntermediatePath)
	})

	rspFile := android.PathForOutput(ctx, fileContainingFilePaths)
	this.outputPath = android.PathForOutput(ctx, ownershipDirectory, allTestSpecsFile)

	rule := android.NewRuleBuilder(pctx, ctx)
	cmd := rule.Command().
		BuiltTool("metadata").
		FlagWithArg("-rule ", "test_spec").
		FlagWithRspFileInputList("-inputFile ", rspFile, intermediateMetadataPaths)
	cmd.FlagWithOutput("-outputFile ", this.outputPath)
	rule.Build("all_test_specs_rule", "Generate all test specifications")
	ctx.Phony("all_test_specs", this.outputPath)
}

func (this *allTestSpecsSingleton) MakeVars(ctx android.MakeVarsContext) {
	ctx.DistForGoal("test_specs", this.outputPath)
}
