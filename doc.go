// json-to-struct is tool that generates Go struct definitions from JSON documents.
//
// json-to-struct reads from stdin and prints to stdout.
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
//		Email             any `json:"email,omitempty"`
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
