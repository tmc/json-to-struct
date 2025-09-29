// json-to-struct generates go struct defintions from JSON documents
//
// Reads from stdin and prints to stdout
//
// Example:
// 	curl -s https://api.github.com/users/tmc | json-to-struct -name=User
//
// Output:
//  package main
//
//  type GithubUser struct {
//  	AvatarURL         string      `json:"avatar_url,omitempty"`
//  	Bio               string      `json:"bio,omitempty"`
//  	Blog              string      `json:"blog,omitempty"`
//  	Company           string      `json:"company,omitempty"`
//  	CreatedAt         string      `json:"created_at,omitempty"`
//  	Email             interface{} `json:"email,omitempty"`
//  	EventsURL         string      `json:"events_url,omitempty"`
//  	Followers         float64     `json:"followers,omitempty"`
//  	FollowersURL      string      `json:"followers_url,omitempty"`
//  	Following         float64     `json:"following,omitempty"`
//  	FollowingURL      string      `json:"following_url,omitempty"`
//  	GistsURL          string      `json:"gists_url,omitempty"`
//  	GravatarID        string      `json:"gravatar_id,omitempty"`
//  	Hireable          bool        `json:"hireable,omitempty"`
//  	HtmlURL           string      `json:"html_url,omitempty"`
//  	ID                float64     `json:"id,omitempty"`
//  	Location          string      `json:"location,omitempty"`
//  	Login             string      `json:"login,omitempty"`
//  	Name              string      `json:"name,omitempty"`
//  	NodeID            string      `json:"node_id,omitempty"`
//  	OrganizationsURL  string      `json:"organizations_url,omitempty"`
//  	PublicGists       float64     `json:"public_gists,omitempty"`
//  	PublicRepos       float64     `json:"public_repos,omitempty"`
//  	ReceivedEventsURL string      `json:"received_events_url,omitempty"`
//  	ReposURL          string      `json:"repos_url,omitempty"`
//  	SiteAdmin         bool        `json:"site_admin,omitempty"`
//  	StarredURL        string      `json:"starred_url,omitempty"`
//  	SubscriptionsURL  string      `json:"subscriptions_url,omitempty"`
//  	Type              string      `json:"type,omitempty"`
//  	UpdatedAt         string      `json:"updated_at,omitempty"`
//  	URL               string      `json:"url,omitempty"`
//  }
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

type Config struct {
	// If True, emit "omitempty" tags on output fields.
	OmitEmpty bool
}

var DefaultConfig = Config{
	OmitEmpty: true,
}

// Given a JSON string representation of an object and a name structName,
// attemp to generate a struct definition
func generate(input io.Reader, structName, pkgName string, cfg *Config) ([]byte, error) {
	var iresult interface{}
	if cfg == nil {
		cfg = &DefaultConfig
	}
	if err := json.NewDecoder(input).Decode(&iresult); err != nil {
		return nil, err
	}

	var typ *Type
	switch iresult := iresult.(type) {
	case map[string]interface{}:
		typ = generateType(structName, iresult, cfg)
	case []map[string]interface{}:
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
	case []interface{}:
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

	src := fmt.Sprintf("package %s\n%s",
		pkgName,
		typ.String())
	formatted, err := format.Source([]byte(src))
	if err != nil {
		err = fmt.Errorf("error formatting: %s, was formatting\n%s", err, src)
	}
	return formatted, err
}

func generateType(name string, value interface{}, cfg *Config) *Type {
	result := &Type{Name: name, Config: cfg}
	switch v := value.(type) {
	case []interface{}:
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
			result.Type = "interface{}"
		}
	case map[string]interface{}:
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

func generateFieldTypes(obj map[string]interface{}, cfg *Config) []*Type {
	result := []*Type{}

	keys := make([]string, 0, len(obj))
	for key := range obj {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		var typ *Type
		switch v := obj[key].(type) {
		case map[string]interface{}:
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

func renderTypes(types []Type, depth int, cfg *Config) string {
	result := "struct {"

	for _, typ := range types {
		result += fmt.Sprintf("%v %v %v\n", typ.Name, typ.GetType(), typ.GetTags())
	}
	return result
}

var uppercaseFixups = map[string]bool{"id": true, "url": true}

// fmtFieldName formats a string as a struct key
//
// Example:
// 	fmtFieldName("foo_id")
// Output: FooID
func fmtFieldName(s string) string {
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
