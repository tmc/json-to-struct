// +build !js

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
	"flag"
	"fmt"
	"os"
)

var (
	flagName      = flag.String("name", "Foo", "the name of the struct")
	flagPkg       = flag.String("pkg", "main", "the name of the package for the generated code")
	flagOmitEmpty = flag.Bool("omitempty", true, "if true, emits struct field tags with 'omitempty'")
)

func main() {
	flag.Parse()

	if isInteractive() {
		flag.Usage()
		fmt.Fprintln(os.Stderr, "Expects input on stdin")
		os.Exit(1)
	}

	cfg := &Config{}
	*cfg = DefaultConfig
	cfg.OmitEmpty = *flagOmitEmpty

	fmt.Println("environ:", os.Environ())

	if output, err := generate(os.Stdin, *flagName, *flagPkg, cfg); err != nil {
		fmt.Fprintln(os.Stderr, "error parsing", err)
		os.Exit(1)
	} else {
		fmt.Print(string(output))
	}
}

// Return true if os.Stdin appears to be interactive
func isInteractive() bool {
	fileInfo, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fileInfo.Mode()&(os.ModeCharDevice|os.ModeCharDevice) != 0
}
