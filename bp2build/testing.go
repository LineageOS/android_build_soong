package bp2build

import (
	"android/soong/android"
	"android/soong/bazel"
)

type nestedProps struct {
	Nested_prop string
}

type customProps struct {
	Bool_prop     bool
	Bool_ptr_prop *bool
	// Ensure that properties tagged `blueprint:mutated` are omitted
	Int_prop         int `blueprint:"mutated"`
	Int64_ptr_prop   *int64
	String_prop      string
	String_ptr_prop  *string
	String_list_prop []string

	Nested_props     nestedProps
	Nested_props_ptr *nestedProps
}

type customModule struct {
	android.ModuleBase
	android.BazelModuleBase

	props customProps
}

// OutputFiles is needed because some instances of this module use dist with a
// tag property which requires the module implements OutputFileProducer.
func (m *customModule) OutputFiles(tag string) (android.Paths, error) {
	return android.PathsForTesting("path" + tag), nil
}

func (m *customModule) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	// nothing for now.
}

func customModuleFactoryBase() android.Module {
	module := &customModule{}
	module.AddProperties(&module.props)
	android.InitBazelModule(module)
	return module
}

func customModuleFactory() android.Module {
	m := customModuleFactoryBase()
	android.InitAndroidModule(m)
	return m
}

type testProps struct {
	Test_prop struct {
		Test_string_prop string
	}
}

type customTestModule struct {
	android.ModuleBase

	props      customProps
	test_props testProps
}

func (m *customTestModule) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	// nothing for now.
}

func customTestModuleFactoryBase() android.Module {
	m := &customTestModule{}
	m.AddProperties(&m.props)
	m.AddProperties(&m.test_props)
	return m
}

func customTestModuleFactory() android.Module {
	m := customTestModuleFactoryBase()
	android.InitAndroidModule(m)
	return m
}

type customDefaultsModule struct {
	android.ModuleBase
	android.DefaultsModuleBase
}

func customDefaultsModuleFactoryBase() android.DefaultsModule {
	module := &customDefaultsModule{}
	module.AddProperties(&customProps{})
	return module
}

func customDefaultsModuleFactoryBasic() android.Module {
	return customDefaultsModuleFactoryBase()
}

func customDefaultsModuleFactory() android.Module {
	m := customDefaultsModuleFactoryBase()
	android.InitDefaultsModule(m)
	return m
}

type customBazelModuleAttributes struct {
	String_prop      string
	String_list_prop []string
}

type customBazelModule struct {
	android.BazelTargetModuleBase
	customBazelModuleAttributes
}

func customBazelModuleFactory() android.Module {
	module := &customBazelModule{}
	module.AddProperties(&module.customBazelModuleAttributes)
	android.InitBazelTargetModule(module)
	return module
}

func (m *customBazelModule) Name() string                                          { return m.BaseModuleName() }
func (m *customBazelModule) GenerateAndroidBuildActions(ctx android.ModuleContext) {}

func customBp2BuildMutator(ctx android.TopDownMutatorContext) {
	if m, ok := ctx.Module().(*customModule); ok {
		if !m.ConvertWithBp2build(ctx) {
			return
		}

		attrs := &customBazelModuleAttributes{
			String_prop:      m.props.String_prop,
			String_list_prop: m.props.String_list_prop,
		}

		props := bazel.BazelTargetModuleProperties{
			Rule_class: "custom",
		}

		ctx.CreateBazelTargetModule(customBazelModuleFactory, m.Name(), props, attrs)
	}
}

// A bp2build mutator that uses load statements and creates a 1:M mapping from
// module to target.
func customBp2BuildMutatorFromStarlark(ctx android.TopDownMutatorContext) {
	if m, ok := ctx.Module().(*customModule); ok {
		if !m.ConvertWithBp2build(ctx) {
			return
		}

		baseName := m.Name()
		attrs := &customBazelModuleAttributes{}

		myLibraryProps := bazel.BazelTargetModuleProperties{
			Rule_class:        "my_library",
			Bzl_load_location: "//build/bazel/rules:rules.bzl",
		}
		ctx.CreateBazelTargetModule(customBazelModuleFactory, baseName, myLibraryProps, attrs)

		protoLibraryProps := bazel.BazelTargetModuleProperties{
			Rule_class:        "proto_library",
			Bzl_load_location: "//build/bazel/rules:proto.bzl",
		}
		ctx.CreateBazelTargetModule(customBazelModuleFactory, baseName+"_proto_library_deps", protoLibraryProps, attrs)

		myProtoLibraryProps := bazel.BazelTargetModuleProperties{
			Rule_class:        "my_proto_library",
			Bzl_load_location: "//build/bazel/rules:proto.bzl",
		}
		ctx.CreateBazelTargetModule(customBazelModuleFactory, baseName+"_my_proto_library_deps", myProtoLibraryProps, attrs)
	}
}

// Helper method for tests to easily access the targets in a dir.
func generateBazelTargetsForDir(codegenCtx *CodegenContext, dir string) BazelTargets {
	buildFileToTargets, _ := GenerateBazelTargets(codegenCtx)
	return buildFileToTargets[dir]
}
