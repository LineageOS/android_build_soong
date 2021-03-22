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
	"fmt"
	"strings"
	"testing"
)

// Provides support for creating test fixtures on which tests can be run. Reduces duplication
// of test setup by allow tests to easily reuse setup code.
//
// Fixture
// =======
// These determine the environment within which a test can be run. Fixtures are mutable and are
// created by FixtureFactory instances and mutated by FixturePreparer instances. They are created by
// first creating a base Fixture (which is essentially empty) and then applying FixturePreparer
// instances to it to modify the environment.
//
// FixtureFactory (deprecated)
// ===========================
// These are responsible for creating fixtures. Factories are immutable and are intended to be
// initialized once and reused to create multiple fixtures. Each factory has a list of fixture
// preparers that prepare a fixture for running a test. Factories can also be used to create other
// factories by extending them with additional fixture preparers.
//
// FixturePreparer
// ===============
// These are responsible for modifying a Fixture in preparation for it to run a test. Preparers are
// intended to be immutable and able to prepare multiple Fixture objects simultaneously without
// them sharing any data.
//
// They provide the basic capabilities for running tests too.
//
// FixturePreparers are only ever applied once per test fixture. Prior to application the list of
// FixturePreparers are flattened and deduped while preserving the order they first appear in the
// list. This makes it easy to reuse, group and combine FixturePreparers together.
//
// Each small self contained piece of test setup should be their own FixturePreparer. e.g.
// * A group of related modules.
// * A group of related mutators.
// * A combination of both.
// * Configuration.
//
// They should not overlap, e.g. the same module type should not be registered by different
// FixturePreparers as using them both would cause a build error. In that case the preparer should
// be split into separate parts and combined together using FixturePreparers(...).
//
// e.g. attempting to use AllPreparers in preparing a Fixture would break as it would attempt to
// register module bar twice:
//   var Preparer1 = FixtureRegisterWithContext(RegisterModuleFooAndBar)
//   var Preparer2 = FixtureRegisterWithContext(RegisterModuleBarAndBaz)
//   var AllPreparers = GroupFixturePreparers(Preparer1, Preparer2)
//
// However, when restructured like this it would work fine:
//   var PreparerFoo = FixtureRegisterWithContext(RegisterModuleFoo)
//   var PreparerBar = FixtureRegisterWithContext(RegisterModuleBar)
//   var PreparerBaz = FixtureRegisterWithContext(RegisterModuleBaz)
//   var Preparer1 = GroupFixturePreparers(RegisterModuleFoo, RegisterModuleBar)
//   var Preparer2 = GroupFixturePreparers(RegisterModuleBar, RegisterModuleBaz)
//   var AllPreparers = GroupFixturePreparers(Preparer1, Preparer2)
//
// As after deduping and flattening AllPreparers would result in the following preparers being
// applied:
// 1. PreparerFoo
// 2. PreparerBar
// 3. PreparerBaz
//
// Preparers can be used for both integration and unit tests.
//
// Integration tests typically use all the module types, mutators and singletons that are available
// for that package to try and replicate the behavior of the runtime build as closely as possible.
// However, that realism comes at a cost of increased fragility (as they can be broken by changes in
// many different parts of the build) and also increased runtime, especially if they use lots of
// singletons and mutators.
//
// Unit tests on the other hand try and minimize the amount of code being tested which makes them
// less susceptible to changes elsewhere in the build and quick to run but at a cost of potentially
// not testing realistic scenarios.
//
// Supporting unit tests effectively require that preparers are available at the lowest granularity
// possible. Supporting integration tests effectively require that the preparers are organized into
// groups that provide all the functionality available.
//
// At least in terms of tests that check the behavior of build components via processing
// `Android.bp` there is no clear separation between a unit test and an integration test. Instead
// they vary from one end that tests a single module (e.g. filegroup) to the other end that tests a
// whole system of modules, mutators and singletons (e.g. apex + hiddenapi).
//
// TestResult
// ==========
// These are created by running tests in a Fixture and provide access to the Config and TestContext
// in which the tests were run.
//
// Example
// =======
//
// An exported preparer for use by other packages that need to use java modules.
//
// package java
// var PrepareForIntegrationTestWithJava = GroupFixturePreparers(
//    android.PrepareForIntegrationTestWithAndroid,
//    FixtureRegisterWithContext(RegisterAGroupOfRelatedModulesMutatorsAndSingletons),
//    FixtureRegisterWithContext(RegisterAnotherGroupOfRelatedModulesMutatorsAndSingletons),
//    ...
// )
//
// Some files to use in tests in the java package.
//
// var javaMockFS = android.MockFS{
//    "api/current.txt":        nil,
//    "api/removed.txt":        nil,
//    ...
// }
//
// A package private preparer for use for testing java within the java package.
//
// var prepareForJavaTest = android.GroupFixturePreparers(
//    PrepareForIntegrationTestWithJava,
//    FixtureRegisterWithContext(func(ctx android.RegistrationContext) {
//      ctx.RegisterModuleType("test_module", testModule)
//    }),
//    javaMockFS.AddToFixture(),
//    ...
// }
//
// func TestJavaStuff(t *testing.T) {
//   result := android.GroupFixturePreparers(t,
//       prepareForJavaTest,
//       android.FixtureWithRootAndroidBp(`java_library {....}`),
//       android.MockFS{...}.AddToFixture(),
//   ).RunTest(t)
//   ... test result ...
// }
//
// package cc
// var PrepareForTestWithCC = android.GroupFixturePreparers(
//    android.PrepareForArchMutator,
//    android.prepareForPrebuilts,
//    FixtureRegisterWithContext(RegisterRequiredBuildComponentsForTest),
//    ...
// )
//
// package apex
//
// var PrepareForApex = GroupFixturePreparers(
//    ...
// )
//
// Use modules and mutators from java, cc and apex. Any duplicate preparers (like
// android.PrepareForArchMutator) will be automatically deduped.
//
// var prepareForApexTest = android.GroupFixturePreparers(
//    PrepareForJava,
//    PrepareForCC,
//    PrepareForApex,
// )
//
// // FixtureFactory instances have been deprecated, this remains for informational purposes to
// // help explain some of the existing code but will be removed along with FixtureFactory.
//
// var javaFixtureFactory = android.NewFixtureFactory(
//    PrepareForIntegrationTestWithJava,
//    FixtureRegisterWithContext(func(ctx android.RegistrationContext) {
//      ctx.RegisterModuleType("test_module", testModule)
//    }),
//    javaMockFS.AddToFixture(),
//    ...
// }
//
// func TestJavaStuff(t *testing.T) {
//   result := javaFixtureFactory.RunTest(t,
//       android.FixtureWithRootAndroidBp(`java_library {....}`),
//       android.MockFS{...}.AddToFixture(),
//   )
//   ... test result ...
// }
//
// package cc
// var PrepareForTestWithCC = GroupFixturePreparers(
//    android.PrepareForArchMutator,
//    android.prepareForPrebuilts,
//    FixtureRegisterWithContext(RegisterRequiredBuildComponentsForTest),
//    ...
// )
//
// package apex
//
// var PrepareForApex = GroupFixturePreparers(
//    ...
// )
//
// Use modules and mutators from java, cc and apex. Any duplicate preparers (like
// android.PrepareForArchMutator) will be automatically deduped.
//
// var apexFixtureFactory = android.NewFixtureFactory(
//    PrepareForJava,
//    PrepareForCC,
//    PrepareForApex,
// )

// Factory for Fixture objects.
//
// This is configured with a set of FixturePreparer objects that are used to
// initialize each Fixture instance this creates.
//
// deprecated: Use FixturePreparer instead.
type FixtureFactory interface {
	FixturePreparer
}

// Create a new FixtureFactory that will apply the supplied preparers.
//
// The buildDirSupplier is a pointer to the package level buildDir variable that is initialized by
// the package level setUp method. It has to be a pointer to the variable as the variable will not
// have been initialized at the time the factory is created. If it is nil then a test specific
// temporary directory will be created instead.
//
// deprecated: The functionality provided by FixtureFactory will be merged into FixturePreparer
func NewFixtureFactory(buildDirSupplier *string, preparers ...FixturePreparer) FixtureFactory {
	f := &fixtureFactory{
		buildDirSupplier: buildDirSupplier,
		compositeFixturePreparer: compositeFixturePreparer{
			preparers: dedupAndFlattenPreparers(nil, preparers),
		},
	}
	f.initBaseFixturePreparer(f)
	return f
}

// A set of mock files to add to the mock file system.
type MockFS map[string][]byte

// Merge adds the extra entries from the supplied map to this one.
//
// Fails if the supplied map files with the same paths are present in both of them.
func (fs MockFS) Merge(extra map[string][]byte) {
	for p, c := range extra {
		validateFixtureMockFSPath(p)
		if _, ok := fs[p]; ok {
			panic(fmt.Errorf("attempted to add file %s to the mock filesystem but it already exists", p))
		}
		fs[p] = c
	}
}

// Ensure that tests cannot add paths into the mock file system which would not be allowed in the
// runtime, e.g. absolute paths, paths relative to the 'out/' directory.
func validateFixtureMockFSPath(path string) {
	// This uses validateSafePath rather than validatePath because the latter prevents adding files
	// that include a $ but there are tests that allow files with a $ to be used, albeit only by
	// globbing.
	validatedPath, err := validateSafePath(path)
	if err != nil {
		panic(err)
	}

	// Make sure that the path is canonical.
	if validatedPath != path {
		panic(fmt.Errorf("path %q is not a canonical path, use %q instead", path, validatedPath))
	}

	if path == "out" || strings.HasPrefix(path, "out/") {
		panic(fmt.Errorf("cannot add output path %q to the mock file system", path))
	}
}

func (fs MockFS) AddToFixture() FixturePreparer {
	return FixtureMergeMockFs(fs)
}

// FixtureCustomPreparer allows for the modification of any aspect of the fixture.
//
// This should only be used if one of the other more specific preparers are not suitable.
func FixtureCustomPreparer(mutator func(fixture Fixture)) FixturePreparer {
	return newSimpleFixturePreparer(func(f *fixture) {
		mutator(f)
	})
}

// Modify the config
func FixtureModifyConfig(mutator func(config Config)) FixturePreparer {
	return newSimpleFixturePreparer(func(f *fixture) {
		mutator(f.config)
	})
}

// Modify the config and context
func FixtureModifyConfigAndContext(mutator func(config Config, ctx *TestContext)) FixturePreparer {
	return newSimpleFixturePreparer(func(f *fixture) {
		mutator(f.config, f.ctx)
	})
}

// Modify the context
func FixtureModifyContext(mutator func(ctx *TestContext)) FixturePreparer {
	return newSimpleFixturePreparer(func(f *fixture) {
		mutator(f.ctx)
	})
}

func FixtureRegisterWithContext(registeringFunc func(ctx RegistrationContext)) FixturePreparer {
	return FixtureModifyContext(func(ctx *TestContext) { registeringFunc(ctx) })
}

// Modify the mock filesystem
func FixtureModifyMockFS(mutator func(fs MockFS)) FixturePreparer {
	return newSimpleFixturePreparer(func(f *fixture) {
		mutator(f.mockFS)

		// Make sure that invalid paths were not added to the mock filesystem.
		for p, _ := range f.mockFS {
			validateFixtureMockFSPath(p)
		}
	})
}

// Merge the supplied file system into the mock filesystem.
//
// Paths that already exist in the mock file system are overridden.
func FixtureMergeMockFs(mockFS MockFS) FixturePreparer {
	return FixtureModifyMockFS(func(fs MockFS) {
		fs.Merge(mockFS)
	})
}

// Add a file to the mock filesystem
//
// Fail if the filesystem already contains a file with that path, use FixtureOverrideFile instead.
func FixtureAddFile(path string, contents []byte) FixturePreparer {
	return FixtureModifyMockFS(func(fs MockFS) {
		validateFixtureMockFSPath(path)
		if _, ok := fs[path]; ok {
			panic(fmt.Errorf("attempted to add file %s to the mock filesystem but it already exists, use FixtureOverride*File instead", path))
		}
		fs[path] = contents
	})
}

// Add a text file to the mock filesystem
//
// Fail if the filesystem already contains a file with that path.
func FixtureAddTextFile(path string, contents string) FixturePreparer {
	return FixtureAddFile(path, []byte(contents))
}

// Override a file in the mock filesystem
//
// If the file does not exist this behaves as FixtureAddFile.
func FixtureOverrideFile(path string, contents []byte) FixturePreparer {
	return FixtureModifyMockFS(func(fs MockFS) {
		fs[path] = contents
	})
}

// Override a text file in the mock filesystem
//
// If the file does not exist this behaves as FixtureAddTextFile.
func FixtureOverrideTextFile(path string, contents string) FixturePreparer {
	return FixtureOverrideFile(path, []byte(contents))
}

// Add the root Android.bp file with the supplied contents.
func FixtureWithRootAndroidBp(contents string) FixturePreparer {
	return FixtureAddTextFile("Android.bp", contents)
}

// Merge some environment variables into the fixture.
func FixtureMergeEnv(env map[string]string) FixturePreparer {
	return FixtureModifyConfig(func(config Config) {
		for k, v := range env {
			if k == "PATH" {
				panic("Cannot set PATH environment variable")
			}
			config.env[k] = v
		}
	})
}

// Modify the env.
//
// Will panic if the mutator changes the PATH environment variable.
func FixtureModifyEnv(mutator func(env map[string]string)) FixturePreparer {
	return FixtureModifyConfig(func(config Config) {
		oldPath := config.env["PATH"]
		mutator(config.env)
		newPath := config.env["PATH"]
		if newPath != oldPath {
			panic(fmt.Errorf("Cannot change PATH environment variable from %q to %q", oldPath, newPath))
		}
	})
}

// Allow access to the product variables when preparing the fixture.
type FixtureProductVariables struct {
	*productVariables
}

// Modify product variables.
func FixtureModifyProductVariables(mutator func(variables FixtureProductVariables)) FixturePreparer {
	return FixtureModifyConfig(func(config Config) {
		productVariables := FixtureProductVariables{&config.productVariables}
		mutator(productVariables)
	})
}

// GroupFixturePreparers creates a composite FixturePreparer that is equivalent to applying each of
// the supplied FixturePreparer instances in order.
//
// Before preparing the fixture the list of preparers is flattened by replacing each
// instance of GroupFixturePreparers with its contents.
func GroupFixturePreparers(preparers ...FixturePreparer) FixturePreparer {
	all := dedupAndFlattenPreparers(nil, preparers)
	return newFixturePreparer(all)
}

// NullFixturePreparer is a preparer that does nothing.
var NullFixturePreparer = GroupFixturePreparers()

// OptionalFixturePreparer will return the supplied preparer if it is non-nil, otherwise it will
// return the NullFixturePreparer
func OptionalFixturePreparer(preparer FixturePreparer) FixturePreparer {
	if preparer == nil {
		return NullFixturePreparer
	} else {
		return preparer
	}
}

// FixturePreparer provides the ability to create, modify and then run tests within a fixture.
type FixturePreparer interface {
	// Return the flattened and deduped list of simpleFixturePreparer pointers.
	list() []*simpleFixturePreparer

	// Creates a copy of this instance and adds some additional preparers.
	//
	// Before the preparers are used they are combined with the preparers provided when the factory
	// was created, any groups of preparers are flattened, and the list is deduped so that each
	// preparer is only used once. See the file documentation in android/fixture.go for more details.
	//
	// deprecated: Use GroupFixturePreparers() instead.
	Extend(preparers ...FixturePreparer) FixturePreparer

	// Create a Fixture.
	Fixture(t *testing.T, preparers ...FixturePreparer) Fixture

	// ExtendWithErrorHandler creates a new FixturePreparer that will use the supplied error handler
	// to check the errors (may be 0) reported by the test.
	//
	// The default handlers is FixtureExpectsNoErrors which will fail the go test immediately if any
	// errors are reported.
	ExtendWithErrorHandler(errorHandler FixtureErrorHandler) FixturePreparer

	// Run the test, checking any errors reported and returning a TestResult instance.
	//
	// Shorthand for Fixture(t, preparers...).RunTest()
	RunTest(t *testing.T, preparers ...FixturePreparer) *TestResult

	// Run the test with the supplied Android.bp file.
	//
	// Shorthand for RunTest(t, android.FixtureWithRootAndroidBp(bp))
	RunTestWithBp(t *testing.T, bp string) *TestResult

	// RunTestWithConfig is a temporary method added to help ease the migration of existing tests to
	// the test fixture.
	//
	// In order to allow the Config object to be customized separately to the TestContext a lot of
	// existing test code has `test...WithConfig` funcs that allow the Config object to be supplied
	// from the test and then have the TestContext created and configured automatically. e.g.
	// testCcWithConfig, testCcErrorWithConfig, testJavaWithConfig, etc.
	//
	// This method allows those methods to be migrated to use the test fixture pattern without
	// requiring that every test that uses those methods be migrated at the same time. That allows
	// those tests to benefit from correctness in the order of registration quickly.
	//
	// This method discards the config (along with its mock file system, product variables,
	// environment, etc.) that may have been set up by FixturePreparers.
	//
	// deprecated
	RunTestWithConfig(t *testing.T, config Config) *TestResult
}

// dedupAndFlattenPreparers removes any duplicates and flattens any composite FixturePreparer
// instances.
//
// base      - a list of already flattened and deduped preparers that will be applied first before
//             the list of additional preparers. Any duplicates of these in the additional preparers
//             will be ignored.
//
// preparers - a list of additional unflattened, undeduped preparers that will be applied after the
//             base preparers.
//
// Returns a deduped and flattened list of the preparers starting with the ones in base with any
// additional ones from the preparers list added afterwards.
func dedupAndFlattenPreparers(base []*simpleFixturePreparer, preparers []FixturePreparer) []*simpleFixturePreparer {
	if len(preparers) == 0 {
		return base
	}

	list := make([]*simpleFixturePreparer, len(base))
	visited := make(map[*simpleFixturePreparer]struct{})

	// Mark the already flattened and deduped preparers, if any, as having been seen so that
	// duplicates of these in the additional preparers will be discarded. Add them to the output
	// list.
	for i, s := range base {
		visited[s] = struct{}{}
		list[i] = s
	}

	for _, p := range preparers {
		for _, s := range p.list() {
			if _, seen := visited[s]; !seen {
				visited[s] = struct{}{}
				list = append(list, s)
			}
		}
	}

	return list
}

// compositeFixturePreparer is a FixturePreparer created from a list of fixture preparers.
type compositeFixturePreparer struct {
	baseFixturePreparer
	// The flattened and deduped list of simpleFixturePreparer pointers encapsulated within this
	// composite preparer.
	preparers []*simpleFixturePreparer
}

func (c *compositeFixturePreparer) list() []*simpleFixturePreparer {
	return c.preparers
}

func newFixturePreparer(preparers []*simpleFixturePreparer) FixturePreparer {
	if len(preparers) == 1 {
		return preparers[0]
	}
	p := &compositeFixturePreparer{
		preparers: preparers,
	}
	p.initBaseFixturePreparer(p)
	return p
}

// simpleFixturePreparer is a FixturePreparer that applies a function to a fixture.
type simpleFixturePreparer struct {
	baseFixturePreparer
	function func(fixture *fixture)
}

func (s *simpleFixturePreparer) list() []*simpleFixturePreparer {
	return []*simpleFixturePreparer{s}
}

func newSimpleFixturePreparer(preparer func(fixture *fixture)) FixturePreparer {
	p := &simpleFixturePreparer{function: preparer}
	p.initBaseFixturePreparer(p)
	return p
}

// FixtureErrorHandler determines how to respond to errors reported by the code under test.
//
// Some possible responses:
// * Fail the test if any errors are reported, see FixtureExpectsNoErrors.
// * Fail the test if at least one error that matches a pattern is not reported see
//   FixtureExpectsAtLeastOneErrorMatchingPattern
// * Fail the test if any unexpected errors are reported.
//
// Although at the moment all the error handlers are implemented as simply a wrapper around a
// function this is defined as an interface to allow future enhancements, e.g. provide different
// ways other than patterns to match an error and to combine handlers together.
type FixtureErrorHandler interface {
	// CheckErrors checks the errors reported.
	//
	// The supplied result can be used to access the state of the code under test just as the main
	// body of the test would but if any errors other than ones expected are reported the state may
	// be indeterminate.
	CheckErrors(t *testing.T, result *TestResult)
}

type simpleErrorHandler struct {
	function func(t *testing.T, result *TestResult)
}

func (h simpleErrorHandler) CheckErrors(t *testing.T, result *TestResult) {
	t.Helper()
	h.function(t, result)
}

// The default fixture error handler.
//
// Will fail the test immediately if any errors are reported.
//
// If the test fails this handler will call `result.FailNow()` which will exit the goroutine within
// which the test is being run which means that the RunTest() method will not return.
var FixtureExpectsNoErrors = FixtureCustomErrorHandler(
	func(t *testing.T, result *TestResult) {
		t.Helper()
		FailIfErrored(t, result.Errs)
	},
)

// FixtureIgnoreErrors ignores any errors.
//
// If this is used then it is the responsibility of the test to check the TestResult.Errs does not
// contain any unexpected errors.
var FixtureIgnoreErrors = FixtureCustomErrorHandler(func(t *testing.T, result *TestResult) {
	// Ignore the errors
})

// FixtureExpectsAtLeastOneMatchingError returns an error handler that will cause the test to fail
// if at least one error that matches the regular expression is not found.
//
// The test will be failed if:
// * No errors are reported.
// * One or more errors are reported but none match the pattern.
//
// The test will not fail if:
// * Multiple errors are reported that do not match the pattern as long as one does match.
//
// If the test fails this handler will call `result.FailNow()` which will exit the goroutine within
// which the test is being run which means that the RunTest() method will not return.
func FixtureExpectsAtLeastOneErrorMatchingPattern(pattern string) FixtureErrorHandler {
	return FixtureCustomErrorHandler(func(t *testing.T, result *TestResult) {
		t.Helper()
		if !FailIfNoMatchingErrors(t, pattern, result.Errs) {
			t.FailNow()
		}
	})
}

// FixtureExpectsOneErrorToMatchPerPattern returns an error handler that will cause the test to fail
// if there are any unexpected errors.
//
// The test will be failed if:
// * The number of errors reported does not exactly match the patterns.
// * One or more of the reported errors do not match a pattern.
// * No patterns are provided and one or more errors are reported.
//
// The test will not fail if:
// * One or more of the patterns does not match an error.
//
// If the test fails this handler will call `result.FailNow()` which will exit the goroutine within
// which the test is being run which means that the RunTest() method will not return.
func FixtureExpectsAllErrorsToMatchAPattern(patterns []string) FixtureErrorHandler {
	return FixtureCustomErrorHandler(func(t *testing.T, result *TestResult) {
		t.Helper()
		CheckErrorsAgainstExpectations(t, result.Errs, patterns)
	})
}

// FixtureCustomErrorHandler creates a custom error handler
func FixtureCustomErrorHandler(function func(t *testing.T, result *TestResult)) FixtureErrorHandler {
	return simpleErrorHandler{
		function: function,
	}
}

// Fixture defines the test environment.
type Fixture interface {
	// Config returns the fixture's configuration.
	Config() Config

	// Context returns the fixture's test context.
	Context() *TestContext

	// MockFS returns the fixture's mock filesystem.
	MockFS() MockFS

	// Run the test, checking any errors reported and returning a TestResult instance.
	RunTest() *TestResult
}

// Struct to allow TestResult to embed a *TestContext and allow call forwarding to its methods.
type testContext struct {
	*TestContext
}

// The result of running a test.
type TestResult struct {
	testContext

	fixture *fixture
	Config  Config

	// The errors that were reported during the test.
	Errs []error

	// The ninja deps is a list of the ninja files dependencies that were added by the modules and
	// singletons via the *.AddNinjaFileDeps() methods.
	NinjaDeps []string
}

func createFixture(t *testing.T, buildDir string, base []*simpleFixturePreparer, extra []FixturePreparer) Fixture {
	all := dedupAndFlattenPreparers(base, extra)

	config := TestConfig(buildDir, nil, "", nil)
	ctx := NewTestContext(config)
	fixture := &fixture{
		preparers: all,
		t:         t,
		config:    config,
		ctx:       ctx,
		mockFS:    make(MockFS),
		// Set the default error handler.
		errorHandler: FixtureExpectsNoErrors,
	}

	for _, preparer := range all {
		preparer.function(fixture)
	}

	return fixture
}

type baseFixturePreparer struct {
	self FixturePreparer
}

func (b *baseFixturePreparer) initBaseFixturePreparer(self FixturePreparer) {
	b.self = self
}

func (b *baseFixturePreparer) Extend(preparers ...FixturePreparer) FixturePreparer {
	all := dedupAndFlattenPreparers(b.self.list(), preparers)
	return newFixturePreparer(all)
}

func (b *baseFixturePreparer) Fixture(t *testing.T, preparers ...FixturePreparer) Fixture {
	return createFixture(t, t.TempDir(), b.self.list(), preparers)
}

func (b *baseFixturePreparer) ExtendWithErrorHandler(errorHandler FixtureErrorHandler) FixturePreparer {
	return b.self.Extend(newSimpleFixturePreparer(func(fixture *fixture) {
		fixture.errorHandler = errorHandler
	}))
}

func (b *baseFixturePreparer) RunTest(t *testing.T, preparers ...FixturePreparer) *TestResult {
	t.Helper()
	fixture := b.self.Fixture(t, preparers...)
	return fixture.RunTest()
}

func (b *baseFixturePreparer) RunTestWithBp(t *testing.T, bp string) *TestResult {
	t.Helper()
	return b.RunTest(t, FixtureWithRootAndroidBp(bp))
}

func (b *baseFixturePreparer) RunTestWithConfig(t *testing.T, config Config) *TestResult {
	t.Helper()
	// Create the fixture as normal.
	fixture := b.self.Fixture(t).(*fixture)

	// Discard the mock filesystem as otherwise that will override the one in the config.
	fixture.mockFS = nil

	// Replace the config with the supplied one in the fixture.
	fixture.config = config

	// Ditto with config derived information in the TestContext.
	ctx := fixture.ctx
	ctx.config = config
	ctx.SetFs(ctx.config.fs)
	if ctx.config.mockBpList != "" {
		ctx.SetModuleListFile(ctx.config.mockBpList)
	}

	return fixture.RunTest()
}

var _ FixtureFactory = (*fixtureFactory)(nil)

type fixtureFactory struct {
	compositeFixturePreparer

	buildDirSupplier *string
}

// Override to preserve the buildDirSupplier.
func (f *fixtureFactory) Extend(preparers ...FixturePreparer) FixturePreparer {
	// If there is no buildDirSupplier then just use the default implementation.
	if f.buildDirSupplier == nil {
		return f.baseFixturePreparer.Extend(preparers...)
	}

	all := dedupAndFlattenPreparers(f.preparers, preparers)

	// Create a new factory which uses the same buildDirSupplier as the previous one.
	extendedFactory := &fixtureFactory{
		buildDirSupplier: f.buildDirSupplier,
		compositeFixturePreparer: compositeFixturePreparer{
			preparers: all,
		},
	}
	extendedFactory.initBaseFixturePreparer(extendedFactory)
	return extendedFactory
}

func (f *fixtureFactory) Fixture(t *testing.T, preparers ...FixturePreparer) Fixture {
	// If there is no buildDirSupplier then just use the default implementation.
	if f.buildDirSupplier == nil {
		return f.baseFixturePreparer.Fixture(t, preparers...)
	}

	// Retrieve the buildDir from the supplier.
	buildDir := *f.buildDirSupplier

	return createFixture(t, buildDir, f.preparers, preparers)
}

type fixture struct {
	// The preparers used to create this fixture.
	preparers []*simpleFixturePreparer

	// The gotest state of the go test within which this was created.
	t *testing.T

	// The configuration prepared for this fixture.
	config Config

	// The test context prepared for this fixture.
	ctx *TestContext

	// The mock filesystem prepared for this fixture.
	mockFS MockFS

	// The error handler used to check the errors, if any, that are reported.
	errorHandler FixtureErrorHandler
}

func (f *fixture) Config() Config {
	return f.config
}

func (f *fixture) Context() *TestContext {
	return f.ctx
}

func (f *fixture) MockFS() MockFS {
	return f.mockFS
}

func (f *fixture) RunTest() *TestResult {
	f.t.Helper()

	ctx := f.ctx

	// Do not use the fixture's mockFS to initialize the config's mock file system if it has been
	// cleared by RunTestWithConfig.
	if f.mockFS != nil {
		// The TestConfig() method assumes that the mock filesystem is available when creating so
		// creates the mock file system immediately. Similarly, the NewTestContext(Config) method
		// assumes that the supplied Config's FileSystem has been properly initialized before it is
		// called and so it takes its own reference to the filesystem. However, fixtures create the
		// Config and TestContext early so they can be modified by preparers at which time the mockFS
		// has not been populated (because it too is modified by preparers). So, this reinitializes the
		// Config and TestContext's FileSystem using the now populated mockFS.
		f.config.mockFileSystem("", f.mockFS)

		ctx.SetFs(ctx.config.fs)
		if ctx.config.mockBpList != "" {
			ctx.SetModuleListFile(ctx.config.mockBpList)
		}
	}

	ctx.Register()
	var ninjaDeps []string
	extraNinjaDeps, errs := ctx.ParseBlueprintsFiles("ignored")
	if len(errs) == 0 {
		ninjaDeps = append(ninjaDeps, extraNinjaDeps...)
		extraNinjaDeps, errs = ctx.PrepareBuildActions(f.config)
		if len(errs) == 0 {
			ninjaDeps = append(ninjaDeps, extraNinjaDeps...)
		}
	}

	result := &TestResult{
		testContext: testContext{ctx},
		fixture:     f,
		Config:      f.config,
		Errs:        errs,
		NinjaDeps:   ninjaDeps,
	}

	f.errorHandler.CheckErrors(f.t, result)

	return result
}

// NormalizePathForTesting removes the test invocation specific build directory from the supplied
// path.
//
// If the path is within the build directory (e.g. an OutputPath) then this returns the relative
// path to avoid tests having to deal with the dynamically generated build directory.
//
// Otherwise, this returns the supplied path as it is almost certainly a source path that is
// relative to the root of the source tree.
//
// Even though some information is removed from some paths and not others it should be possible to
// differentiate between them by the paths themselves, e.g. output paths will likely include
// ".intermediates" but source paths won't.
func (r *TestResult) NormalizePathForTesting(path Path) string {
	pathContext := PathContextForTesting(r.Config)
	pathAsString := path.String()
	if rel, isRel := MaybeRel(pathContext, r.Config.BuildDir(), pathAsString); isRel {
		return rel
	}
	return pathAsString
}

// NormalizePathsForTesting normalizes each path in the supplied list and returns their normalized
// forms.
func (r *TestResult) NormalizePathsForTesting(paths Paths) []string {
	var result []string
	for _, path := range paths {
		result = append(result, r.NormalizePathForTesting(path))
	}
	return result
}

// Preparer will return a FixturePreparer encapsulating all the preparers used to create the fixture
// that produced this result.
//
// e.g. assuming that this result was created by running:
//     factory.Extend(preparer1, preparer2).RunTest(t, preparer3, preparer4)
//
// Then this method will be equivalent to running:
//     GroupFixturePreparers(preparer1, preparer2, preparer3, preparer4)
//
// This is intended for use by tests whose output is Android.bp files to verify that those files
// are valid, e.g. tests of the snapshots produced by the sdk module type.
func (r *TestResult) Preparer() FixturePreparer {
	return newFixturePreparer(r.fixture.preparers)
}

// Module returns the module with the specific name and of the specified variant.
func (r *TestResult) Module(name string, variant string) Module {
	return r.ModuleForTests(name, variant).Module()
}
