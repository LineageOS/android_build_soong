// Copyright 2021 Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package android

import (
	"sort"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

var (
	_ = pctx.HostBinToolVariable("licenseMetadataCmd", "build_license_metadata")

	licenseMetadataRule = pctx.AndroidStaticRule("licenseMetadataRule", blueprint.RuleParams{
		Command:        "${licenseMetadataCmd} -o $out @${out}.rsp",
		CommandDeps:    []string{"${licenseMetadataCmd}"},
		Rspfile:        "${out}.rsp",
		RspfileContent: "${args}",
	}, "args")
)

func buildLicenseMetadata(ctx ModuleContext, licenseMetadataFile WritablePath) {
	base := ctx.Module().base()

	if !base.Enabled() {
		return
	}

	if exemptFromRequiredApplicableLicensesProperty(ctx.Module()) {
		return
	}

	var outputFiles Paths
	if outputFileProducer, ok := ctx.Module().(OutputFileProducer); ok {
		outputFiles, _ = outputFileProducer.OutputFiles("")
		outputFiles = PathsIfNonNil(outputFiles...)
	}

	// Only pass the last installed file to isContainerFromFileExtensions so a *.zip file in test data
	// doesn't mark the whole module as a container.
	var installFiles InstallPaths
	if len(base.installFiles) > 0 {
		installFiles = InstallPaths{base.installFiles[len(base.installFiles)-1]}
	}

	isContainer := isContainerFromFileExtensions(installFiles, outputFiles)

	var allDepMetadataFiles Paths
	var allDepMetadataArgs []string
	var allDepOutputFiles Paths
	var allDepMetadataDepSets []*DepSet[Path]

	ctx.VisitDirectDepsBlueprint(func(bpdep blueprint.Module) {
		dep, _ := bpdep.(Module)
		if dep == nil {
			return
		}
		if !dep.Enabled() {
			return
		}

		// Defaults add properties and dependencies that get processed on their own.
		if ctx.OtherModuleDependencyTag(dep) == DefaultsDepTag {
			return
		}

		if ctx.OtherModuleHasProvider(dep, LicenseMetadataProvider) {
			info := ctx.OtherModuleProvider(dep, LicenseMetadataProvider).(*LicenseMetadataInfo)
			allDepMetadataFiles = append(allDepMetadataFiles, info.LicenseMetadataPath)
			if isContainer || isInstallDepNeeded(dep, ctx.OtherModuleDependencyTag(dep)) {
				allDepMetadataDepSets = append(allDepMetadataDepSets, info.LicenseMetadataDepSet)
			}

			depAnnotations := licenseAnnotationsFromTag(ctx.OtherModuleDependencyTag(dep))

			allDepMetadataArgs = append(allDepMetadataArgs, info.LicenseMetadataPath.String()+depAnnotations)

			if depInstallFiles := dep.base().installFiles; len(depInstallFiles) > 0 {
				allDepOutputFiles = append(allDepOutputFiles, depInstallFiles.Paths()...)
			} else if depOutputFiles, err := outputFilesForModule(ctx, dep, ""); err == nil {
				depOutputFiles = PathsIfNonNil(depOutputFiles...)
				allDepOutputFiles = append(allDepOutputFiles, depOutputFiles...)
			}
		}
	})

	allDepMetadataFiles = SortedUniquePaths(allDepMetadataFiles)
	sort.Strings(allDepMetadataArgs)
	allDepOutputFiles = SortedUniquePaths(allDepOutputFiles)

	var orderOnlyDeps Paths
	var args []string

	if n := ctx.ModuleName(); n != "" {
		args = append(args,
			"-mn "+proptools.NinjaAndShellEscape(n))
	}

	if t := ctx.ModuleType(); t != "" {
		args = append(args,
			"-mt "+proptools.NinjaAndShellEscape(t))
	}

	args = append(args,
		"-r "+proptools.NinjaAndShellEscape(ctx.ModuleDir()),
		"-mc UNKNOWN")

	if p := base.commonProperties.Effective_package_name; p != nil {
		args = append(args,
			`-p `+proptools.NinjaAndShellEscapeIncludingSpaces(*p))
	}

	args = append(args,
		JoinWithPrefix(proptools.NinjaAndShellEscapeListIncludingSpaces(base.commonProperties.Effective_license_kinds), "-k "))

	args = append(args,
		JoinWithPrefix(proptools.NinjaAndShellEscapeListIncludingSpaces(base.commonProperties.Effective_license_conditions), "-c "))

	args = append(args,
		JoinWithPrefix(proptools.NinjaAndShellEscapeListIncludingSpaces(base.commonProperties.Effective_license_text.Strings()), "-n "))

	if isContainer {
		transitiveDeps := Paths(NewDepSet[Path](TOPOLOGICAL, nil, allDepMetadataDepSets).ToList())
		args = append(args,
			JoinWithPrefix(proptools.NinjaAndShellEscapeListIncludingSpaces(transitiveDeps.Strings()), "-d "))
		orderOnlyDeps = append(orderOnlyDeps, transitiveDeps...)
	} else {
		args = append(args,
			JoinWithPrefix(proptools.NinjaAndShellEscapeListIncludingSpaces(allDepMetadataArgs), "-d "))
		orderOnlyDeps = append(orderOnlyDeps, allDepMetadataFiles...)
	}

	args = append(args,
		JoinWithPrefix(proptools.NinjaAndShellEscapeListIncludingSpaces(allDepOutputFiles.Strings()), "-s "))

	// Install map
	args = append(args,
		JoinWithPrefix(proptools.NinjaAndShellEscapeListIncludingSpaces(base.licenseInstallMap), "-m "))

	// Built files
	if len(outputFiles) > 0 {
		args = append(args,
			JoinWithPrefix(proptools.NinjaAndShellEscapeListIncludingSpaces(outputFiles.Strings()), "-t "))
	}

	// Installed files
	args = append(args,
		JoinWithPrefix(proptools.NinjaAndShellEscapeListIncludingSpaces(base.installFiles.Strings()), "-i "))

	if isContainer {
		args = append(args, "--is_container")
	}

	ctx.Build(pctx, BuildParams{
		Rule:        licenseMetadataRule,
		Output:      licenseMetadataFile,
		OrderOnly:   orderOnlyDeps,
		Description: "license metadata",
		Args: map[string]string{
			"args": strings.Join(args, " "),
		},
	})

	ctx.SetProvider(LicenseMetadataProvider, &LicenseMetadataInfo{
		LicenseMetadataPath:   licenseMetadataFile,
		LicenseMetadataDepSet: NewDepSet(TOPOLOGICAL, Paths{licenseMetadataFile}, allDepMetadataDepSets),
	})
}

func isContainerFromFileExtensions(installPaths InstallPaths, builtPaths Paths) bool {
	var paths Paths
	if len(installPaths) > 0 {
		paths = installPaths.Paths()
	} else {
		paths = builtPaths
	}

	for _, path := range paths {
		switch path.Ext() {
		case ".zip", ".tar", ".tgz", ".tar.gz", ".img", ".srcszip", ".apex":
			return true
		}
	}

	return false
}

// LicenseMetadataProvider is used to propagate license metadata paths between modules.
var LicenseMetadataProvider = blueprint.NewProvider(&LicenseMetadataInfo{})

// LicenseMetadataInfo stores the license metadata path for a module.
type LicenseMetadataInfo struct {
	LicenseMetadataPath   Path
	LicenseMetadataDepSet *DepSet[Path]
}

// licenseAnnotationsFromTag returns the LicenseAnnotations for a tag (if any) converted into
// a string, or an empty string if there are none.
func licenseAnnotationsFromTag(tag blueprint.DependencyTag) string {
	if annoTag, ok := tag.(LicenseAnnotationsDependencyTag); ok {
		annos := annoTag.LicenseAnnotations()
		if len(annos) > 0 {
			annoStrings := make([]string, len(annos))
			for i, s := range annos {
				annoStrings[i] = string(s)
			}
			return ":" + strings.Join(annoStrings, ",")
		}
	}
	return ""
}

// LicenseAnnotationsDependencyTag is implemented by dependency tags in order to provide a
// list of license dependency annotations.
type LicenseAnnotationsDependencyTag interface {
	LicenseAnnotations() []LicenseAnnotation
}

// LicenseAnnotation is an enum of annotations that can be applied to dependencies for propagating
// license information.
type LicenseAnnotation string

const (
	// LicenseAnnotationSharedDependency should be returned by LicenseAnnotations implementations
	// of dependency tags when the usage of the dependency is dynamic, for example a shared library
	// linkage for native modules or as a classpath library for java modules.
	//
	// Dependency tags that need to always return LicenseAnnotationSharedDependency
	// can embed LicenseAnnotationSharedDependencyTag to implement LicenseAnnotations.
	LicenseAnnotationSharedDependency LicenseAnnotation = "dynamic"

	// LicenseAnnotationToolchain should be returned by LicenseAnnotations implementations of
	// dependency tags when the dependency is used as a toolchain.
	//
	// Dependency tags that need to always return LicenseAnnotationToolchain
	// can embed LicenseAnnotationToolchainDependencyTag to implement LicenseAnnotations.
	LicenseAnnotationToolchain LicenseAnnotation = "toolchain"
)

// LicenseAnnotationSharedDependencyTag can be embedded in a dependency tag to implement
// LicenseAnnotations that always returns LicenseAnnotationSharedDependency.
type LicenseAnnotationSharedDependencyTag struct{}

func (LicenseAnnotationSharedDependencyTag) LicenseAnnotations() []LicenseAnnotation {
	return []LicenseAnnotation{LicenseAnnotationSharedDependency}
}

// LicenseAnnotationToolchainDependencyTag can be embedded in a dependency tag to implement
// LicenseAnnotations that always returns LicenseAnnotationToolchain.
type LicenseAnnotationToolchainDependencyTag struct{}

func (LicenseAnnotationToolchainDependencyTag) LicenseAnnotations() []LicenseAnnotation {
	return []LicenseAnnotation{LicenseAnnotationToolchain}
}
