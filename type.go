package main

import (
	"fmt"
	"sort"
	"strings"
)

type TypeType string

const (
	// Real types
	Null    = TypeType("null")    // Also means - not yet observed specific type
	Integer = TypeType("int")     // Observed only integers.
	Number  = TypeType("float64") // Observed floating numbers.
	String  = TypeType("string")
	Boolean = TypeType("bool")
	Array   = TypeType("array")
	Object  = TypeType("object")
	// Special types
	Mixed = TypeType("mixed") // Observed mixed types
)

type Type struct {
	TypeType TypeType
	Object   map[string]*Type
	Array    *Type
	Nullable bool

	Tags []string
}

func NullType() *Type {
	return &Type{TypeType: Null, Nullable: false}
}

func ExplicitNullType() *Type {
	return &Type{TypeType: Null, Nullable: true}
}

func ArrayType(t Type) *Type {
	return &Type{TypeType: Array, Array: &t, Nullable: false}
}

func ObjectType(obj map[string]*Type) *Type {
	return &Type{TypeType: Object, Object: obj, Nullable: false}
}

func PrimitiveType(v interface{}) *Type {
	switch v := v.(type) {
	case int:
		return &Type{TypeType: Integer, Nullable: false}
	case float64:
		if v == float64(int(v)) {
			return &Type{TypeType: Integer, Nullable: false}
		} else {
			return &Type{TypeType: Number, Nullable: false}
		}
	case bool:
		return &Type{TypeType: Boolean, Nullable: false}
	case string:
		return &Type{TypeType: String, Nullable: false}
	default:
		return &Type{TypeType: Mixed, Nullable: false}
	}
}

func (t *Type) AddTag(tag string) {
	t.Tags = append(t.Tags, tag)
}

func (t Type) String() string {
	if t.TypeType == Object {
		output := "struct{"
		for k, v := range t.Object {
			output += fmt.Sprintf("%v:%v,", k, v)
		}
		output += "}"
		return output
	} else if t.TypeType == Array {
		return fmt.Sprintf("[]%v", t.Array)
	} else {
		return string(t.TypeType)
	}
}

func (t Type) GoString() string {
	prefix := ""
	if t.Nullable {
		prefix = "*"
	}
	switch t.TypeType {
	case Object:
		output := prefix + "struct{\n"
		keys := make([]string, 0, len(t.Object))
		for k := range t.Object {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v := t.Object[k]

			tags := ""
			if v.Tags != nil {
				tags = fmt.Sprintf("  `json:\"%v\"`", strings.Join(v.Tags, ","))
			}

			output += fmt.Sprintf("  %v %v %v\n", k, v.GoString(), tags)
		}
		output += "}"
		return output
	case Array:
		return fmt.Sprintf("%v[]%v", prefix, t.Array.GoString())
	case Mixed:
		return "interface{}"
	case Null:
		return "interface{}" // might actually return struct{}
	default:
		return prefix + string(t.TypeType)
	}
}

func (t *Type) CopyFrom(t2 Type) {
	*t = t2
}

func (t *Type) Merge(t2 Type) error {
	//log.Printf("  Merging %v and %v", t, t2)
	if t.TypeType == Null {
		t.CopyFrom(t2)
		return nil
	}
	if t2.TypeType == Null {
		t.Nullable = t.Nullable || t2.Nullable
		return nil
	}
	if (t.TypeType == Integer && t2.TypeType == Number) || (t.TypeType == Number && t2.TypeType == Integer) {
		t.TypeType = Number
		return nil
	}
	if t.TypeType == Mixed || t2.TypeType == Mixed || t.TypeType != t2.TypeType {
		t.CopyFrom(Type{TypeType: Mixed})
		return nil
	}
	if t.TypeType == Array {
		return t.Array.Merge(*t2.Array)
	}
	if t.TypeType == Object {
		for k2, v2 := range t2.Object {
			if v, ok := t.Object[k2]; !ok {
				t.Object[k2] = v2
			} else {
				v.Merge(*v2)
			}
		}
	}
	if t.TypeType == t2.TypeType {
		return nil
	}
	panic(fmt.Sprintf("Unknown type for marge:\n    %v\nand %v", t, t2))
}
