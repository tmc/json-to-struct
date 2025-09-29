package main

import (
	"fmt"
	"sort"
	"strings"
)

type Fields []*Type

func (f Fields) String() string {
	result := []string{}
	for _, field := range f {
		result = append(result, field.String())
	}
	return strings.Join(result, "\n")
}

type Type struct {
	Name     string
	Repeated bool
	Type     string
	Tags     map[string]string
	Children Fields
	Config   *generator
}

func (t *Type) GetType() string {
	if t.Type == "nil" {
		t.Type = "any"
	}
	if t.Repeated {
		return "[]" + t.Type
	}
	return t.Type
}

func (t *Type) GetTags() string {
	if len(t.Tags) == 0 {
		return ""
	}

	keys := make([]string, 0, len(t.Tags))
	for key := range t.Tags {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := []string{}
	for _, k := range keys {
		v := t.Tags[k]
		if k == "json" && t.Config.OmitEmpty {
			v += ",omitempty"
		}
		parts = append(parts, fmt.Sprintf(`%v:"%v"`, k, v))
	}
	return fmt.Sprintf("`%v`", strings.Join(parts, ","))
}

func (t *Type) String() string {
	if t.Config != nil && t.Config.typeTemplate != nil {
		return t.Config.renderTypeWithTemplate(t)
	}
	return t.Config.renderType(t)
}

func (t *Type) Merge(t2 *Type) error {
	if strings.Trim(t.Type, "*") != strings.Trim(t2.Type, "*") {
		if t.Type == "nil" {
			t.Type = fmt.Sprintf("*%s", strings.Trim(t2.Type, "*"))
			return nil
		} else if t2.Type == "nil" {
			t.Type = fmt.Sprintf("*%s", strings.Trim(t.Type, "*"))
			return nil
		} else {
			t.Type = "any"
			return nil
		}
	}

	fields := map[string]*Type{}
	for _, typ := range t.Children {
		fields[typ.Name] = typ
	}
	for _, typ := range t2.Children {
		field, ok := fields[typ.Name]
		if !ok {
			t.Children = append(t.Children, typ)
			continue
		}
		if err := field.Merge(typ); err != nil {
			return fmt.Errorf("issue with '%v': %w", field.Name, err)
		}
	}

	return nil
}

// renderType renders the type as a Go struct definition.
func (g *generator) renderType(t *Type) string {
	return g.renderTypeWithKeyword(t, true)
}

// renderTypeWithKeyword renders the type, optionally including the 'type' keyword
func (g *generator) renderTypeWithKeyword(t *Type, includeTypeKeyword bool) string {
	if len(t.Children) == 0 {
		if includeTypeKeyword {
			return fmt.Sprintf("type %s %s %s", t.Name, t.GetType(), t.GetTags())
		}
		return fmt.Sprintf("%s %s %s", t.Name, t.GetType(), t.GetTags())
	}

	result := []string{}
	if includeTypeKeyword {
		result = append(result, fmt.Sprintf("type %s %s {", t.Name, t.GetType()))
	} else {
		result = append(result, fmt.Sprintf("%s %s {", t.Name, t.GetType()))
	}

	for _, child := range t.Children {
		result = append(result, fmt.Sprintf("    %s", g.renderTypeWithKeyword(child, false)))
	}
	result = append(result, fmt.Sprintf("}%s", t.GetTags()))
	return strings.Join(result, "\n")
}
