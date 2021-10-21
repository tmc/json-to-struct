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
	Config   *Config
}

func (t *Type) GetType() string {
	if t.Type == "nil"{
		t.Type = "interface{}"
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
	if t.Type == "struct" {
		return fmt.Sprintf(`%v %v {
%s } %v`, t.Name, t.GetType(), t.Children, t.GetTags())
	}
	return fmt.Sprintf("%v %v %v", t.Name, t.GetType(), t.GetTags())
}

func (t *Type) Merge(t2 *Type) error {
	if t.Type != t2.Type {
		if t.Type == "nil" {
			t.Type = fmt.Sprintf("*%s", t2.Type)
			return nil
		} else if t2.Type == "nil" {
			t.Type = fmt.Sprintf("*%s", t.Type)
			return nil
		} else {
			t.Type = "interface{}"
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
