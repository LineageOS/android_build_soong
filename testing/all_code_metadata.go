package testing

import (
	"android/soong/android"
)

const fileContainingCodeMetadataFilePaths = "all_code_metadata_paths.rsp"
const allCodeMetadataFile = "all_code_metadata.pb"

func AllCodeMetadataFactory() android.Singleton {
	return &allCodeMetadataSingleton{}
}

type allCodeMetadataSingleton struct {
	// Path where the collected metadata is stored after successful validation.
	outputPath android.OutputPath
}

func (this *allCodeMetadataSingleton) GenerateBuildActions(ctx android.SingletonContext) {
	var intermediateMetadataPaths android.Paths

	ctx.VisitAllModules(
		func(module android.Module) {
			if metadata, ok := android.SingletonModuleProvider(ctx, module, CodeMetadataProviderKey); ok {
				intermediateMetadataPaths = append(intermediateMetadataPaths, metadata.IntermediatePath)
			}
		},
	)

	rspFile := android.PathForOutput(ctx, fileContainingCodeMetadataFilePaths)
	this.outputPath = android.PathForOutput(ctx, ownershipDirectory, allCodeMetadataFile)

	rule := android.NewRuleBuilder(pctx, ctx)
	cmd := rule.Command().
		BuiltTool("metadata").
		FlagWithArg("-rule ", "code_metadata").
		FlagWithRspFileInputList("-inputFile ", rspFile, intermediateMetadataPaths)
	cmd.FlagWithOutput("-outputFile ", this.outputPath)
	rule.Build("all_code_metadata_rule", "Generate all code metadata")

	ctx.Phony("all_code_metadata", this.outputPath)
}

func (this *allCodeMetadataSingleton) MakeVars(ctx android.MakeVarsContext) {
	ctx.DistForGoal("code_metadata", this.outputPath)
}
