// Copyright 2017 Google Inc. All rights reserved.
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

package config

import (
	"strings"
)

func init() {
	pctx.SourcePathVariable("ErrorProneJavacJar", "external/error_prone/javac/javac-9-dev-r3297-4.jar")
	pctx.SourcePathVariable("ErrorProneJar", "external/error_prone/error_prone/error_prone_core-2.0.19-with-dependencies.jar")
	pctx.SourcePathsVariable("ErrorProneClasspath", ":",
		"external/error_prone/error_prone/error_prone_annotations-2.0.19.jar",
		"external/error_prone/checkerframework/dataflow-1.8.10.jar",
		"external/error_prone/checkerframework/javacutil-1.8.10.jar",
		"external/error_prone/jFormatString/jFormatString-3.0.0.jar")

	// The checks that are fatal to the build.
	pctx.StaticVariable("ErrorProneChecksError", strings.Join([]string{
		"-Xep:AsyncCallableReturnsNull:ERROR",
		"-Xep:AsyncFunctionReturnsNull:ERROR",
		"-Xep:BundleDeserializationCast:ERROR",
		"-Xep:CompatibleWithAnnotationMisuse:ERROR",
		"-Xep:CompileTimeConstant:ERROR",
		"-Xep:DaggerProvidesNull:ERROR",
		"-Xep:DoNotCall:ERROR",
		"-Xep:ForOverride:ERROR",
		"-Xep:FunctionalInterfaceMethodChanged:ERROR",
		"-Xep:FuturesGetCheckedIllegalExceptionType:ERROR",
		"-Xep:GuiceAssistedInjectScoping:ERROR",
		"-Xep:GuiceAssistedParameters:ERROR",
		"-Xep:GuiceInjectOnFinalField:ERROR",
		"-Xep:Immutable:ERROR",
		"-Xep:ImmutableModification:ERROR",
		"-Xep:IncompatibleArgumentType:ERROR",
		"-Xep:IndexOfChar:ERROR",
		"-Xep:InjectMoreThanOneScopeAnnotationOnClass:ERROR",
		"-Xep:JavaxInjectOnAbstractMethod:ERROR",
		"-Xep:JUnit4SetUpNotRun:ERROR",
		"-Xep:JUnit4TearDownNotRun:ERROR",
		"-Xep:JUnit4TestNotRun:ERROR",
		"-Xep:JUnitAssertSameCheck:ERROR",
		"-Xep:LiteByteStringUtf8:ERROR",
		"-Xep:LoopConditionChecker:ERROR",
		"-Xep:MockitoCast:ERROR",
		"-Xep:MockitoUsage:ERROR",
		"-Xep:MoreThanOneInjectableConstructor:ERROR",
		"-Xep:MustBeClosedChecker:ERROR",
		"-Xep:NonCanonicalStaticImport:ERROR",
		"-Xep:NonFinalCompileTimeConstant:ERROR",
		"-Xep:OptionalEquality:ERROR",
		"-Xep:OverlappingQualifierAndScopeAnnotation:ERROR",
		"-Xep:PackageInfo:ERROR",
		"-Xep:PreconditionsCheckNotNull:ERROR",
		"-Xep:PreconditionsCheckNotNullPrimitive:ERROR",
		"-Xep:ProtoFieldNullComparison:ERROR",
		"-Xep:ProvidesMethodOutsideOfModule:ERROR",
		"-Xep:RestrictedApiChecker:ERROR",
		"-Xep:SelfAssignment:ERROR",
		"-Xep:StreamToString:ERROR",
		"-Xep:SuppressWarningsDeprecated:ERROR",
		"-Xep:ThrowIfUncheckedKnownChecked:ERROR",
		"-Xep:ThrowNull:ERROR",
		"-Xep:TypeParameterQualifier:ERROR",
		"-Xep:UnnecessaryTypeArgument:ERROR",
		"-Xep:UnusedAnonymousClass:ERROR",
	}, " "))

	pctx.StaticVariable("ErrorProneFlags", strings.Join([]string{
		"com.google.errorprone.ErrorProneCompiler",
		"-Xdiags:verbose",
		"-XDcompilePolicy=simple",
		"-XDallowBetterNullChecks=false",
		"-XDusePolyAttribution=true",
		"-XDuseStrictMethodClashCheck=true",
		"-XDuseStructuralMostSpecificResolution=true",
		"-XDuseGraphInference=true",
		"-Xmaxwarns 100000",
		"-XDandroidCompatible=true",
		"-XepAllErrorsAsWarnings",
	}, " "))

	pctx.StaticVariable("ErrorProneCmd",
		"${JavaCmd} -Xbootclasspath/p:${ErrorProneJavacJar} -cp ${ErrorProneJar}:${ErrorProneClasspath} ${ErrorProneFlags} ${ErrorProneChecksError}")
}
