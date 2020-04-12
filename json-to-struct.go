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
	"flag"
	"fmt"
	"go/format"
	"io"
	"os"
	"reflect"
	"sort"
	"strings"
	"unicode"
)

var (
	name      = flag.String("name", "Foo", "the name of the struct")
	pkg       = flag.String("pkg", "main", "the name of the package for the generated code")
	omitEmpty = flag.Bool("omitempty", true, "if true, emits struct field tags with 'omitempty'")
)

type Config struct {
	OmitEmpty bool
}

var DefaultConfig = Config{
	OmitEmpty: true,
}

// Given a JSON string representation of an object and a name structName,
// attemp to generate a struct definition
func generate(input io.Reader, structName, pkgName string, cfg *Config) ([]byte, error) {
	var iresult interface{}
	var result map[string]interface{}
	if cfg == nil {
		cfg = &DefaultConfig
	}
	if err := json.NewDecoder(input).Decode(&iresult); err != nil {
		return nil, err
	}

	switch iresult := iresult.(type) {
	case map[string]interface{}:
		result = iresult
	case []map[string]interface{}:
		if len(iresult) > 0 {
			result = iresult[0]
		} else {
			return nil, fmt.Errorf("empty array")
		}
	default:
		return nil, fmt.Errorf("unexpected type: %T", iresult)
	}

	src := fmt.Sprintf("package %s\ntype %s %s}",
		pkgName,
		structName,
		generateTypes(result, 0, cfg))
	formatted, err := format.Source([]byte(src))
	if err != nil {
		err = fmt.Errorf("error formatting: %s, was formatting\n%s", err, src)
	}
	return formatted, err
}

// Generate go struct entries for a map[string]interface{} structure
func generateTypes(obj map[string]interface{}, depth int, cfg *Config) string {
	structure := "struct {"

	keys := make([]string, 0, len(obj))
	for key := range obj {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	omitEmptyString := ""
	if cfg.OmitEmpty {
		omitEmptyString = ",omitempty"
	}
	for _, key := range keys {
		value := obj[key]
		valueType := typeForValue(value, cfg)

		//If a nested value, recurse
		switch value := value.(type) {
		case []map[string]interface{}:
			valueType = "[]" + generateTypes(value[0], depth+1, cfg) + "}"
		case map[string]interface{}:
			valueType = generateTypes(value, depth+1, cfg) + "}"
		}

		fieldName := fmtFieldName(key)

		if fieldName != key {
			structure += fmt.Sprintf("\n%s %s `json:\"%s%s\"`",
				fieldName,
				valueType,
				key,
				omitEmptyString, // Depending on cfg.OmitEmpty, include omitempty.
			)
		} else {
			structure += fmt.Sprintf("\n%s %s", fieldName, valueType)
		}
	}
	return structure
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

// generate an appropriate struct type entry
func typeForValue(value interface{}, cfg *Config) string {
	//Check if this is an array
	if objects, ok := value.([]interface{}); ok {
		types := make(map[reflect.Type]bool, 0)
		for _, o := range objects {
			types[reflect.TypeOf(o)] = true
		}
		if len(types) == 1 {
			return "[]" + typeForValue(objects[0], cfg)
		}
		return "[]interface{}"
	} else if object, ok := value.(map[string]interface{}); ok {
		return generateTypes(object, 0, cfg) + "}"
	} else if reflect.TypeOf(value) == nil {
		return "interface{}"
	}
	return reflect.TypeOf(value).Name()
}

// Return true if os.Stdin appears to be interactive
func isInteractive() bool {
	fileInfo, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fileInfo.Mode()&(os.ModeCharDevice|os.ModeCharDevice) != 0
}

func main() {
	flag.Parse()

	if isInteractive() {
		flag.Usage()
		fmt.Fprintln(os.Stderr, "Expects input on stdin")
		os.Exit(1)
	}

	cfg := &Config{}
	*cfg = DefaultConfig
	cfg.OmitEmpty = *omitEmpty

	if output, err := generate(os.Stdin, *name, *pkg, cfg); err != nil {
		fmt.Fprintln(os.Stderr, "error parsing", err)
		os.Exit(1)
	} else {
		fmt.Print(string(output))
	}
}
