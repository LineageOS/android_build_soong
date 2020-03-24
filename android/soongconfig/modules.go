// Copyright 2020 Google Inc. All rights reserved.
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

package soongconfig

import (
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/parser"
	"github.com/google/blueprint/proptools"
)

var soongConfigProperty = proptools.FieldNameForProperty("soong_config_variables")

// loadSoongConfigModuleTypeDefinition loads module types from an Android.bp file.  It caches the
// result so each file is only parsed once.
func Parse(r io.Reader, from string) (*SoongConfigDefinition, []error) {
	scope := parser.NewScope(nil)
	file, errs := parser.ParseAndEval(from, r, scope)

	if len(errs) > 0 {
		return nil, errs
	}

	mtDef := &SoongConfigDefinition{
		ModuleTypes: make(map[string]*ModuleType),
		variables:   make(map[string]soongConfigVariable),
	}

	for _, def := range file.Defs {
		switch def := def.(type) {
		case *parser.Module:
			newErrs := processImportModuleDef(mtDef, def)

			if len(newErrs) > 0 {
				errs = append(errs, newErrs...)
			}

		case *parser.Assignment:
			// Already handled via Scope object
		default:
			panic("unknown definition type")
		}
	}

	if len(errs) > 0 {
		return nil, errs
	}

	for name, moduleType := range mtDef.ModuleTypes {
		for _, varName := range moduleType.variableNames {
			if v, ok := mtDef.variables[varName]; ok {
				moduleType.Variables = append(moduleType.Variables, v)
			} else {
				return nil, []error{
					fmt.Errorf("unknown variable %q in module type %q", varName, name),
				}
			}
		}
	}

	return mtDef, nil
}

func processImportModuleDef(v *SoongConfigDefinition, def *parser.Module) (errs []error) {
	switch def.Type {
	case "soong_config_module_type":
		return processModuleTypeDef(v, def)
	case "soong_config_string_variable":
		return processStringVariableDef(v, def)
	case "soong_config_bool_variable":
		return processBoolVariableDef(v, def)
	default:
		// Unknown module types will be handled when the file is parsed as a normal
		// Android.bp file.
	}

	return nil
}

type ModuleTypeProperties struct {
	// the name of the new module type.  Unlike most modules, this name does not need to be unique,
	// although only one module type with any name will be importable into an Android.bp file.
	Name string

	// the module type that this module type will extend.
	Module_type string

	// the SOONG_CONFIG_NAMESPACE value from a BoardConfig.mk that this module type will read
	// configuration variables from.
	Config_namespace string

	// the list of SOONG_CONFIG variables that this module type will read
	Variables []string

	// the list of boolean SOONG_CONFIG variables that this module type will read
	Bool_variables []string

	// the list of SOONG_CONFIG variables that this module type will read. The value will be
	// inserted into the properties with %s substitution.
	Value_variables []string

	// the list of properties that this module type will extend.
	Properties []string
}

func processModuleTypeDef(v *SoongConfigDefinition, def *parser.Module) (errs []error) {

	props := &ModuleTypeProperties{}

	_, errs = proptools.UnpackProperties(def.Properties, props)
	if len(errs) > 0 {
		return errs
	}

	if props.Name == "" {
		errs = append(errs, fmt.Errorf("name property must be set"))
	}

	if props.Config_namespace == "" {
		errs = append(errs, fmt.Errorf("config_namespace property must be set"))
	}

	if props.Module_type == "" {
		errs = append(errs, fmt.Errorf("module_type property must be set"))
	}

	if len(errs) > 0 {
		return errs
	}

	mt := &ModuleType{
		affectableProperties: props.Properties,
		ConfigNamespace:      props.Config_namespace,
		BaseModuleType:       props.Module_type,
		variableNames:        props.Variables,
	}
	v.ModuleTypes[props.Name] = mt

	for _, name := range props.Bool_variables {
		if name == "" {
			return []error{fmt.Errorf("bool_variable name must not be blank")}
		}

		mt.Variables = append(mt.Variables, &boolVariable{
			baseVariable: baseVariable{
				variable: name,
			},
		})
	}

	for _, name := range props.Value_variables {
		if name == "" {
			return []error{fmt.Errorf("value_variables entry must not be blank")}
		}

		mt.Variables = append(mt.Variables, &valueVariable{
			baseVariable: baseVariable{
				variable: name,
			},
		})
	}

	return nil
}

type VariableProperties struct {
	Name string
}

type StringVariableProperties struct {
	Values []string
}

func processStringVariableDef(v *SoongConfigDefinition, def *parser.Module) (errs []error) {
	stringProps := &StringVariableProperties{}

	base, errs := processVariableDef(def, stringProps)
	if len(errs) > 0 {
		return errs
	}

	if len(stringProps.Values) == 0 {
		return []error{fmt.Errorf("values property must be set")}
	}

	v.variables[base.variable] = &stringVariable{
		baseVariable: base,
		values:       CanonicalizeToProperties(stringProps.Values),
	}

	return nil
}

func processBoolVariableDef(v *SoongConfigDefinition, def *parser.Module) (errs []error) {
	base, errs := processVariableDef(def)
	if len(errs) > 0 {
		return errs
	}

	v.variables[base.variable] = &boolVariable{
		baseVariable: base,
	}

	return nil
}

func processVariableDef(def *parser.Module,
	extraProps ...interface{}) (cond baseVariable, errs []error) {

	props := &VariableProperties{}

	allProps := append([]interface{}{props}, extraProps...)

	_, errs = proptools.UnpackProperties(def.Properties, allProps...)
	if len(errs) > 0 {
		return baseVariable{}, errs
	}

	if props.Name == "" {
		return baseVariable{}, []error{fmt.Errorf("name property must be set")}
	}

	return baseVariable{
		variable: props.Name,
	}, nil
}

type SoongConfigDefinition struct {
	ModuleTypes map[string]*ModuleType

	variables map[string]soongConfigVariable
}

// CreateProperties returns a reflect.Value of a newly constructed type that contains the desired
// property layout for the Soong config variables, with each possible value an interface{} that
// contains a nil pointer to another newly constructed type that contains the affectable properties.
// The reflect.Value will be cloned for each call to the Soong config module type's factory method.
//
// For example, the acme_cc_defaults example above would
// produce a reflect.Value whose type is:
// *struct {
//     Soong_config_variables struct {
//         Board struct {
//             Soc_a interface{}
//             Soc_b interface{}
//         }
//     }
// }
// And whose value is:
// &{
//     Soong_config_variables: {
//         Board: {
//             Soc_a: (*struct{ Cflags []string })(nil),
//             Soc_b: (*struct{ Cflags []string })(nil),
//         },
//     },
// }
func CreateProperties(factory blueprint.ModuleFactory, moduleType *ModuleType) reflect.Value {
	var fields []reflect.StructField

	_, factoryProps := factory()
	affectablePropertiesType := createAffectablePropertiesType(moduleType.affectableProperties, factoryProps)
	if affectablePropertiesType == nil {
		return reflect.Value{}
	}

	for _, c := range moduleType.Variables {
		fields = append(fields, reflect.StructField{
			Name: proptools.FieldNameForProperty(c.variableProperty()),
			Type: c.variableValuesType(),
		})
	}

	typ := reflect.StructOf([]reflect.StructField{{
		Name: soongConfigProperty,
		Type: reflect.StructOf(fields),
	}})

	props := reflect.New(typ)
	structConditions := props.Elem().FieldByName(soongConfigProperty)

	for i, c := range moduleType.Variables {
		c.initializeProperties(structConditions.Field(i), affectablePropertiesType)
	}

	return props
}

// createAffectablePropertiesType creates a reflect.Type of a struct that has a field for each affectable property
// that exists in factoryProps.
func createAffectablePropertiesType(affectableProperties []string, factoryProps []interface{}) reflect.Type {
	affectableProperties = append([]string(nil), affectableProperties...)
	sort.Strings(affectableProperties)

	var recurse func(prefix string, aps []string) ([]string, reflect.Type)
	recurse = func(prefix string, aps []string) ([]string, reflect.Type) {
		var fields []reflect.StructField

		for len(affectableProperties) > 0 {
			p := affectableProperties[0]
			if !strings.HasPrefix(affectableProperties[0], prefix) {
				break
			}
			affectableProperties = affectableProperties[1:]

			nestedProperty := strings.TrimPrefix(p, prefix)
			if i := strings.IndexRune(nestedProperty, '.'); i >= 0 {
				var nestedType reflect.Type
				nestedPrefix := nestedProperty[:i+1]

				affectableProperties, nestedType = recurse(prefix+nestedPrefix, affectableProperties)

				if nestedType != nil {
					nestedFieldName := proptools.FieldNameForProperty(strings.TrimSuffix(nestedPrefix, "."))

					fields = append(fields, reflect.StructField{
						Name: nestedFieldName,
						Type: nestedType,
					})
				}
			} else {
				typ := typeForPropertyFromPropertyStructs(factoryProps, p)
				if typ != nil {
					fields = append(fields, reflect.StructField{
						Name: proptools.FieldNameForProperty(nestedProperty),
						Type: typ,
					})
				}
			}
		}

		var typ reflect.Type
		if len(fields) > 0 {
			typ = reflect.StructOf(fields)
		}
		return affectableProperties, typ
	}

	affectableProperties, typ := recurse("", affectableProperties)
	if len(affectableProperties) > 0 {
		panic(fmt.Errorf("didn't handle all affectable properties"))
	}

	if typ != nil {
		return reflect.PtrTo(typ)
	}

	return nil
}

func typeForPropertyFromPropertyStructs(psList []interface{}, property string) reflect.Type {
	for _, ps := range psList {
		if typ := typeForPropertyFromPropertyStruct(ps, property); typ != nil {
			return typ
		}
	}

	return nil
}

func typeForPropertyFromPropertyStruct(ps interface{}, property string) reflect.Type {
	v := reflect.ValueOf(ps)
	for len(property) > 0 {
		if !v.IsValid() {
			return nil
		}

		if v.Kind() == reflect.Interface {
			if v.IsNil() {
				return nil
			} else {
				v = v.Elem()
			}
		}

		if v.Kind() == reflect.Ptr {
			if v.IsNil() {
				v = reflect.Zero(v.Type().Elem())
			} else {
				v = v.Elem()
			}
		}

		if v.Kind() != reflect.Struct {
			return nil
		}

		if index := strings.IndexRune(property, '.'); index >= 0 {
			prefix := property[:index]
			property = property[index+1:]

			v = v.FieldByName(proptools.FieldNameForProperty(prefix))
		} else {
			f := v.FieldByName(proptools.FieldNameForProperty(property))
			if !f.IsValid() {
				return nil
			}
			return f.Type()
		}
	}
	return nil
}

// PropertiesToApply returns the applicable properties from a ModuleType that should be applied
// based on SoongConfig values.
func PropertiesToApply(moduleType *ModuleType, props reflect.Value, config SoongConfig) ([]interface{}, error) {
	var ret []interface{}
	props = props.Elem().FieldByName(soongConfigProperty)
	for i, c := range moduleType.Variables {
		if ps, err := c.PropertiesToApply(config, props.Field(i)); err != nil {
			return nil, err
		} else if ps != nil {
			ret = append(ret, ps)
		}
	}
	return ret, nil
}

type ModuleType struct {
	BaseModuleType  string
	ConfigNamespace string
	Variables       []soongConfigVariable

	affectableProperties []string
	variableNames        []string
}

type soongConfigVariable interface {
	// variableProperty returns the name of the variable.
	variableProperty() string

	// conditionalValuesType returns a reflect.Type that contains an interface{} for each possible value.
	variableValuesType() reflect.Type

	// initializeProperties is passed a reflect.Value of the reflect.Type returned by conditionalValuesType and a
	// reflect.Type of the affectable properties, and should initialize each interface{} in the reflect.Value with
	// the zero value of the affectable properties type.
	initializeProperties(v reflect.Value, typ reflect.Type)

	// PropertiesToApply should return one of the interface{} values set by initializeProperties to be applied
	// to the module.
	PropertiesToApply(config SoongConfig, values reflect.Value) (interface{}, error)
}

type baseVariable struct {
	variable string
}

func (c *baseVariable) variableProperty() string {
	return CanonicalizeToProperty(c.variable)
}

type stringVariable struct {
	baseVariable
	values []string
}

func (s *stringVariable) variableValuesType() reflect.Type {
	var fields []reflect.StructField

	for _, v := range s.values {
		fields = append(fields, reflect.StructField{
			Name: proptools.FieldNameForProperty(v),
			Type: emptyInterfaceType,
		})
	}

	return reflect.StructOf(fields)
}

func (s *stringVariable) initializeProperties(v reflect.Value, typ reflect.Type) {
	for i := range s.values {
		v.Field(i).Set(reflect.Zero(typ))
	}
}

func (s *stringVariable) PropertiesToApply(config SoongConfig, values reflect.Value) (interface{}, error) {
	for j, v := range s.values {
		if config.String(s.variable) == v {
			return values.Field(j).Interface(), nil
		}
	}

	return nil, nil
}

type boolVariable struct {
	baseVariable
}

func (b boolVariable) variableValuesType() reflect.Type {
	return emptyInterfaceType
}

func (b boolVariable) initializeProperties(v reflect.Value, typ reflect.Type) {
	v.Set(reflect.Zero(typ))
}

func (b boolVariable) PropertiesToApply(config SoongConfig, values reflect.Value) (interface{}, error) {
	if config.Bool(b.variable) {
		return values.Interface(), nil
	}

	return nil, nil
}

type valueVariable struct {
	baseVariable
}

func (s *valueVariable) variableValuesType() reflect.Type {
	return emptyInterfaceType
}

func (s *valueVariable) initializeProperties(v reflect.Value, typ reflect.Type) {
	v.Set(reflect.Zero(typ))
}

func (s *valueVariable) PropertiesToApply(config SoongConfig, values reflect.Value) (interface{}, error) {
	if !config.IsSet(s.variable) {
		return nil, nil
	}
	configValue := config.String(s.variable)

	propStruct := values.Elem().Elem()
	for i := 0; i < propStruct.NumField(); i++ {
		field := propStruct.Field(i)
		kind := field.Kind()
		if kind == reflect.Ptr {
			if field.IsNil() {
				continue
			}
			field = field.Elem()
		}
		switch kind {
		case reflect.String:
			err := printfIntoProperty(field, configValue)
			if err != nil {
				return nil, fmt.Errorf("soong_config_variables.%s.%s: %s", s.variable, propStruct.Type().Field(i).Name, err)
			}
		case reflect.Slice:
			for j := 0; j < field.Len(); j++ {
				err := printfIntoProperty(field.Index(j), configValue)
				if err != nil {
					return nil, fmt.Errorf("soong_config_variables.%s.%s: %s", s.variable, propStruct.Type().Field(i).Name, err)
				}
			}
		case reflect.Bool:
			// Nothing to do
		default:
			return nil, fmt.Errorf("soong_config_variables.%s.%s: unsupported property type %q", s.variable, propStruct.Type().Field(i).Name, kind)
		}
	}

	return values.Interface(), nil
}

func printfIntoProperty(propertyValue reflect.Value, configValue string) error {
	s := propertyValue.String()

	count := strings.Count(s, "%")
	if count == 0 {
		return nil
	}

	if count > 1 {
		return fmt.Errorf("value variable properties only support a single '%%'")
	}

	if !strings.Contains(s, "%s") {
		return fmt.Errorf("unsupported %% in value variable property")
	}

	propertyValue.Set(reflect.ValueOf(fmt.Sprintf(s, configValue)))

	return nil
}

func CanonicalizeToProperty(v string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'A' && r <= 'Z',
			r >= 'a' && r <= 'z',
			r >= '0' && r <= '9',
			r == '_':
			return r
		default:
			return '_'
		}
	}, v)
}

func CanonicalizeToProperties(values []string) []string {
	ret := make([]string, len(values))
	for i, v := range values {
		ret[i] = CanonicalizeToProperty(v)
	}
	return ret
}

type emptyInterfaceStruct struct {
	i interface{}
}

var emptyInterfaceType = reflect.TypeOf(emptyInterfaceStruct{}).Field(0).Type
