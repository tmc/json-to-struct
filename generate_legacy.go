//go:build legacy

package main

import (
	"encoding/json"
	"fmt"
	"go/format"
	"io"
	"reflect"
	"sort"
	"strings"
	"unicode"
)

func init() {
	legacyGenerateFunc = generate
}

func generate(input io.Reader, structName, pkgName string, cfg *generator) ([]byte, error) {
	var iresult any
	if cfg == nil {
		cfg = &generator{OmitEmpty: true}
	}
	if err := json.NewDecoder(input).Decode(&iresult); err != nil {
		return nil, err
	}

	var typ *Type
	switch iresult := iresult.(type) {
	case map[string]any:
		typ = generateType(structName, iresult, cfg)
	case []map[string]any:
		if len(iresult) == 0 {
			return nil, fmt.Errorf("empty array")
		}
		typ = generateType(structName, iresult[0], cfg)
		for _, r := range iresult[0:] {
			t2 := generateType(structName, r, cfg)
			if err := typ.Merge(t2); err != nil {
				return nil, fmt.Errorf("issue merging: %w", err)
			}
		}
	case []any:
		// TODO: reduce repetition
		if len(iresult) == 0 {
			return nil, fmt.Errorf("empty array")
		}
		typ = generateType(structName, iresult[0], cfg)
		for _, r := range iresult[0:] {
			t2 := generateType(structName, r, cfg)
			if err := typ.Merge(t2); err != nil {
				return nil, fmt.Errorf("issue merging: %w", err)
			}
		}
	default:
		return nil, fmt.Errorf("unexpected type: %T", iresult)
	}

	src := renderFile(pkgName, typ.String(), cfg)
	formatted, err := format.Source([]byte(src))
	if err != nil {
		err = fmt.Errorf("error generating struct: %w", err)
	}
	return formatted, err
}

func generateType(name string, value any, cfg *generator) *Type {
	result := &Type{Name: name, Config: cfg}
	switch v := value.(type) {
	case []any:
		types := make(map[reflect.Type]bool, 0)
		for _, o := range v {
			types[reflect.TypeOf(o)] = true
		}
		result.Repeated = true
		if len(types) == 1 {
			t := generateType("", v[0], cfg)
			result.Type = t.Type
			result.Children = t.Children
		} else {
			result.Type = "any"
		}
	case map[string]any:
		result.Type = "struct"
		result.Children = generateFieldTypes(v, cfg)
	default:
		if reflect.TypeOf(value) == nil {
			result.Type = "nil"
		} else {
			result.Type = reflect.TypeOf(value).Name()
		}
	}
	return result
}

func generateFieldTypes(obj map[string]any, cfg *generator) []*Type {
	result := []*Type{}

	keys := make([]string, 0, len(obj))
	for key := range obj {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		var typ *Type
		switch v := obj[key].(type) {
		case map[string]any:
			typ = generateType(key, v, cfg)
		default:
			typ = generateType(key, obj[key], cfg)
		}
		typ.Name = fmtFieldName(key)
		// if we need to rewrite the field name we need to record the json field in a tag.
		if typ.Name != key {
			typ.Tags = map[string]string{"json": key}
		}
		result = append(result, typ)
	}
	return result
}

func fmtFieldName(s string) string {
	uppercaseFixups := map[string]bool{"id": true, "url": true}
	parts := strings.Split(s, "_")
	for i := range parts {
		parts[i] = strings.Title(parts[i])
	}
	if len(parts) > 0 {
		last := parts[len(parts)-1]
		if uppercaseFixups[strings.ToLower(last)] {
			parts[len(parts)-1] = strings.ToUpper(last)
		}
	}
	assembled := strings.Join(parts, "")
	runes := []rune(assembled)
	for i, c := range runes {
		ok := unicode.IsLetter(c) || unicode.IsDigit(c)
		if i == 0 {
			ok = unicode.IsLetter(c)
		}
		if !ok {
			runes[i] = '_'
		}
	}
	return string(runes)
}
