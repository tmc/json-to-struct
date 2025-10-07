// Command json-to-struct generates Go struct definitions from JSON documents.
//
// # json-to-struct
//
// Command json-to-struct generates Go struct definitions from JSON documents.
//
// json-to-struct reads from stdin and prints to stdout, making it easy to integrate into
// shell pipelines and build workflows.
//
// ```sh
// $ json-to-struct -h
// Usage of json-to-struct:
//
//	-cpuprofile string
//		write CPU profile to file
//	-extract-structs
//		if true, extracts repeated nested structs to reduce duplication
//	-field-order string
//		field ordering: alphabetical, encounter, common-first, or rare-first (default "alphabetical")
//	-name string
//		the name of the struct (default "Foo")
//	-omitempty
//		if true, emits struct field tags with 'omitempty' (default true)
//	-pkg string
//		the name of the package for the generated code (default "main")
//	-pprof string
//		pprof server address (e.g., :6060)
//	-roundtrip
//		if true, generates and runs a round-trip validation test
//	-stat-comments
//		if true, adds field statistics as comments
//	-stream
//		if true, shows progressive output with terminal clearing
//	-template string
//		path to txtar template file
//	-update-interval int
//		milliseconds between stream mode updates (default 500)
//
// ```
//
// It effectively exposes JSON-to-Go struct conversion for use in shells.
//
// # Example
//
// Given a JSON API response:
//
//	$ curl -s https://api.github.com/users/tmc | json-to-struct -name=User
//
// # Produces
//
//	package main
//
//	type User struct {
//		AvatarURL         string  `json:"avatar_url,omitempty"`
//		Bio               string  `json:"bio,omitempty"`
//		Blog              string  `json:"blog,omitempty"`
//		Company           string  `json:"company,omitempty"`
//		CreatedAt         string  `json:"created_at,omitempty"`
//		Email             any     `json:"email,omitempty"`
//		EventsURL         string  `json:"events_url,omitempty"`
//		Followers         float64 `json:"followers,omitempty"`
//		FollowersURL      string  `json:"followers_url,omitempty"`
//		Following         float64 `json:"following,omitempty"`
//		FollowingURL      string  `json:"following_url,omitempty"`
//		GistsURL          string  `json:"gists_url,omitempty"`
//		GravatarID        string  `json:"gravatar_id,omitempty"`
//		Hireable          bool    `json:"hireable,omitempty"`
//		HtmlURL           string  `json:"html_url,omitempty"`
//		ID                float64 `json:"id,omitempty"`
//		Location          string  `json:"location,omitempty"`
//		Login             string  `json:"login,omitempty"`
//		Name              string  `json:"name,omitempty"`
//		NodeID            string  `json:"node_id,omitempty"`
//		OrganizationsURL  string  `json:"organizations_url,omitempty"`
//		PublicGists       float64 `json:"public_gists,omitempty"`
//		PublicRepos       float64 `json:"public_repos,omitempty"`
//		ReceivedEventsURL string  `json:"received_events_url,omitempty"`
//		ReposURL          string  `json:"repos_url,omitempty"`
//		SiteAdmin         bool    `json:"site_admin,omitempty"`
//		StarredURL        string  `json:"starred_url,omitempty"`
//		SubscriptionsURL  string  `json:"subscriptions_url,omitempty"`
//		Type              string  `json:"type,omitempty"`
//		UpdatedAt         string  `json:"updated_at,omitempty"`
//		URL               string  `json:"url,omitempty"`
//	}
package main
