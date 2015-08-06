package main

import (
	"fmt"
	"strings"

	bpparser "github.com/google/blueprint/parser"
)

type Module struct {
	bpmod      *bpparser.Module
	bpname     string
	mkname     string
	isHostRule bool
}

func newModule(mod *bpparser.Module) *Module {
	return &Module{
		bpmod:  mod.Copy(),
		bpname: mod.Type.Name,
	}
}

func (m *Module) translateRuleName() error {
	var name string
	if translation, ok := moduleTypeToRule[m.bpname]; ok {
		name = translation
	} else {
		return fmt.Errorf("Unknown module type %q", m.bpname)
	}

	if m.isHostRule {
		if trans, ok := targetToHostModuleRule[name]; ok {
			name = trans
		} else {
			return fmt.Errorf("No corresponding host rule for %q", name)
		}
	} else {
		m.isHostRule = strings.Contains(name, "HOST")
	}

	m.mkname = name

	return nil
}

func (m *Module) Properties() Properties {
	return Properties{&m.bpmod.Properties}
}

func (m *Module) PropBool(name string) bool {
	if prop, ok := m.Properties().Prop(name); ok {
		return prop.Value.BoolValue
	}
	return false
}

func (m *Module) IterateArchPropertiesWithName(name string, f func(Properties, *bpparser.Property) error) error {
	if p, ok := m.Properties().Prop(name); ok {
		err := f(m.Properties(), p)
		if err != nil {
			return err
		}
	}

	for _, prop := range m.bpmod.Properties {
		switch prop.Name.Name {
		case "arch", "multilib", "target":
			for _, sub := range prop.Value.MapValue {
				props := Properties{&sub.Value.MapValue}
				if p, ok := props.Prop(name); ok {
					err := f(props, p)
					if err != nil {
						return err
					}
				}
			}
		}
	}

	return nil
}

type Properties struct {
	props *[]*bpparser.Property
}

func (p Properties) Prop(name string) (*bpparser.Property, bool) {
	for _, prop := range *p.props {
		if name == prop.Name.Name {
			return prop, true
		}
	}
	return nil, false
}

func (p Properties) AppendToProp(name string, src *bpparser.Property) error {
	if d, ok := p.Prop(name); ok {
		val, err := appendValueToValue(d.Value, src.Value)
		if err != nil {
			return err
		}

		d.Value = val
	} else {
		prop := src.Copy()
		prop.Name.Name = name
		*p.props = append(*p.props, prop)
	}

	return nil
}

func (p Properties) DeleteProp(name string) {
	for i, prop := range *p.props {
		if prop.Name.Name == name {
			*p.props = append((*p.props)[0:i], (*p.props)[i+1:]...)
			return
		}
	}
}
