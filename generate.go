// json-to-struct generates go struct defintions from JSON documents
//
// # Reads from stdin and prints to stdout
//
// Example:
//
//	curl -s https://api.github.com/users/tmc | json-to-struct -name=User
//
// Output:
//
//	package main
//
//	type GithubUser struct {
//		AvatarURL         string      `json:"avatar_url,omitempty"`
//		Bio               string      `json:"bio,omitempty"`
//		Blog              string      `json:"blog,omitempty"`
//		Company           string      `json:"company,omitempty"`
//		CreatedAt         string      `json:"created_at,omitempty"`
//		Email             interface{} `json:"email,omitempty"`
//		EventsURL         string      `json:"events_url,omitempty"`
//		Followers         float64     `json:"followers,omitempty"`
//		FollowersURL      string      `json:"followers_url,omitempty"`
//		Following         float64     `json:"following,omitempty"`
//		FollowingURL      string      `json:"following_url,omitempty"`
//		GistsURL          string      `json:"gists_url,omitempty"`
//		GravatarID        string      `json:"gravatar_id,omitempty"`
//		Hireable          bool        `json:"hireable,omitempty"`
//		HtmlURL           string      `json:"html_url,omitempty"`
//		ID                float64     `json:"id,omitempty"`
//		Location          string      `json:"location,omitempty"`
//		Login             string      `json:"login,omitempty"`
//		Name              string      `json:"name,omitempty"`
//		NodeID            string      `json:"node_id,omitempty"`
//		OrganizationsURL  string      `json:"organizations_url,omitempty"`
//		PublicGists       float64     `json:"public_gists,omitempty"`
//		PublicRepos       float64     `json:"public_repos,omitempty"`
//		ReceivedEventsURL string      `json:"received_events_url,omitempty"`
//		ReposURL          string      `json:"repos_url,omitempty"`
//		SiteAdmin         bool        `json:"site_admin,omitempty"`
//		StarredURL        string      `json:"starred_url,omitempty"`
//		SubscriptionsURL  string      `json:"subscriptions_url,omitempty"`
//		Type              string      `json:"type,omitempty"`
//		UpdatedAt         string      `json:"updated_at,omitempty"`
//		URL               string      `json:"url,omitempty"`
//	}
package main

import (
	"encoding/json"
	"fmt"
	"go/format"
	"io"
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

	typ := generateType(iresult, cfg)
	if typ.TypeType == Array {
		typ = typ.Array
	}

	src := fmt.Sprintf("package %s\ntype %s %s",
		pkgName,
		structName,
		typ.GoString())

	formatted, err := format.Source([]byte(src))
	if err != nil {
		err = fmt.Errorf("error formatting: %s, was formatting\n%s", err, src)
	}
	return formatted, err
}

func generateType(value interface{}, cfg *Config) (typ *Type) {
	switch v := value.(type) {
	case []interface{}:
		ta := NullType()
		for _, o := range v {
			ta.Merge(*generateType(o, cfg))
		}
		typ = ArrayType(*ta)
	case map[string]interface{}:
		typ = ObjectType(generateFieldTypes(v, cfg))
	case nil:
		typ = ExplicitNullType()
	default:
		typ = PrimitiveType(v)
	}
	//log.Printf("generateType(%#v %T) -> %v", value, value, typ)
	return typ
}

func generateFieldTypes(obj map[string]interface{}, cfg *Config) map[string]*Type {
	result := map[string]*Type{}

	for key, value := range obj {
		keyName := fmtFieldName(key)
		result[keyName] = generateType(value, cfg)
		if keyName != key {
			result[keyName].AddTag(key)
		}
		if cfg.OmitEmpty {
			result[keyName].AddTag("omitempty")
		}
	}
	return result
}

var uppercaseFixups = map[string]bool{"id": true, "url": true}

// fmtFieldName formats a string as a struct key
//
// Example:
//
//	fmtFieldName("foo_id")
//
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
