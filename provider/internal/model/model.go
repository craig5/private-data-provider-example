// Package model parses the data directory into typed, validated data.
//
// It knows nothing about OpenTofu or Terraform. That is the point: all of the
// parsing rules live here, where `go test` can reach them, and the provider is
// a thin wrapper that calls Load. The c5egprov-show command calls it too.
package model

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"gopkg.in/yaml.v3"
)

// Directory is the parsed, validated result: everything the HCL needs, keyed
// so it can go straight into for_each.
//
// The json names match the provider's attribute names, so the c5egprov-show
// command prints exactly what the HCL sees.
type Directory struct {
	Users    map[string]User    `json:"users"`    // keyed by username
	Roles    map[string]Role    `json:"roles"`    // keyed by role name
	Bindings map[string]Binding `json:"bindings"` // keyed by "username/role"
}

type User struct {
	Username string   `json:"username"`
	FullName string   `json:"full_name"`
	Email    string   `json:"email"`
	Team     string   `json:"team"`
	Roles    []string `json:"roles"`
}

type Role struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type Binding struct {
	User string `json:"user"` // username
	Role string `json:"role"`
}

// Load reads a data directory laid out as roles.yaml plus teams/*.yaml (or
// *.yml).
func Load(dataDir, emailDomain string) (*Directory, error) {
	return loadFiles(filepath.Join(dataDir, "roles.yaml"), filepath.Join(dataDir, "teams"), emailDomain)
}

// loadFiles reads the roles file and every YAML file in teamsDir, then calls
// Parse.
func loadFiles(rolesFile, teamsDir, emailDomain string) (*Directory, error) {
	roles, err := os.ReadFile(rolesFile)
	if err != nil {
		return nil, err
	}

	var paths []string
	for _, pattern := range []string{"*.yaml", "*.yml"} {
		matches, err := filepath.Glob(filepath.Join(teamsDir, pattern))
		if err != nil {
			return nil, err
		}
		paths = append(paths, matches...)
	}
	// No team files means no users, and a plan that deletes every user. That
	// is far more likely to be a wrong path than a real intent, so refuse it.
	if len(paths) == 0 {
		return nil, fmt.Errorf("no team files (*.yaml or *.yml) in %s", teamsDir)
	}

	teams := make(map[string][]byte, len(paths))
	for _, p := range paths {
		if teams[p], err = os.ReadFile(p); err != nil {
			return nil, err
		}
	}

	return Parse(roles, teams, emailDomain)
}

// Parse builds a Directory from the raw file contents. teams maps a file name
// (used only in error messages) to its contents.
func Parse(rolesYAML []byte, teams map[string][]byte, emailDomain string) (*Directory, error) {
	var rf rolesFile
	if err := yaml.Unmarshal(rolesYAML, &rf); err != nil {
		return nil, fmt.Errorf("roles file: %w", err)
	}

	d := &Directory{
		Users:    map[string]User{},
		Roles:    map[string]Role{},
		Bindings: map[string]Binding{},
	}
	for name, r := range rf.Roles {
		role := Role{Name: name}
		if r != nil {
			role.Description = r.Description
		}
		d.Roles[name] = role
	}

	seen := map[string]string{} // username -> "file:line" of first definition

	// Sorted, so errors are deterministic when more than one file is bad.
	for _, file := range slices.Sorted(maps.Keys(teams)) {
		var tf teamFile
		if err := yaml.Unmarshal(teams[file], &tf); err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		if tf.Team == "" {
			return nil, fmt.Errorf("%s: no team name", file)
		}

		for _, u := range tf.Users {
			at := fmt.Sprintf("%s:%d", file, u.line)
			if u.Disabled {
				continue
			}
			if u.Username == "" {
				return nil, fmt.Errorf("%s: user has no username", at)
			}
			if u.FullName == "" {
				return nil, fmt.Errorf("%s: user %q has no full_name", at, u.Username)
			}
			if first, ok := seen[u.Username]; ok {
				return nil, fmt.Errorf("%s: user %q is already defined at %s", at, u.Username, first)
			}
			seen[u.Username] = at

			// Only a missing roles key falls back to the team default. A key
			// that is there but empty ("roles: []", or "roles:" with nothing
			// after it) is a mistake, not a request for the default.
			roles := u.Roles
			if u.has["roles"] {
				if len(roles) == 0 {
					return nil, fmt.Errorf("%s: user %q has an empty roles list", at, u.Username)
				}
			} else {
				roles = tf.DefaultRoles
				if len(roles) == 0 {
					return nil, fmt.Errorf("%s: user %q has no roles, and team %q has no default_roles", at, u.Username, tf.Team)
				}
			}
			for _, r := range roles {
				if _, ok := d.Roles[r]; !ok {
					return nil, fmt.Errorf("%s: user %q has role %q, which is not in the roles file", at, u.Username, r)
				}
				key := u.Username + "/" + r
				if _, ok := d.Bindings[key]; ok {
					return nil, fmt.Errorf("%s: user %q lists role %q twice", at, u.Username, r)
				}
				d.Bindings[key] = Binding{User: u.Username, Role: r}
			}

			// The same goes for email: missing gets the default, empty is an
			// error.
			email := u.Email
			if u.has["email"] && email == "" {
				return nil, fmt.Errorf("%s: user %q has an empty email", at, u.Username)
			}
			if email == "" {
				if emailDomain == "" {
					return nil, fmt.Errorf("%s: user %q has no email, and no email domain was given", at, u.Username)
				}
				email = u.Username + "@" + emailDomain
			}
			d.Users[u.Username] = User{
				Username: u.Username,
				FullName: u.FullName,
				Email:    email,
				Team:     tf.Team,
				Roles:    roles,
			}
		}
	}

	return d, nil
}

// The on-disk formats. These are unexported: callers only ever see Directory.
//
// Each one rejects keys it does not know, so a typo such as "role:" for
// "roles:" is an error instead of a silently ignored field.

type rolesFile struct {
	Roles map[string]*roleEntry `yaml:"roles"`
}

var rolesFileKeys = []string{"roles"}

func (f *rolesFile) UnmarshalYAML(n *yaml.Node) error {
	if err := checkKeys(n, rolesFileKeys, "roles file"); err != nil {
		return err
	}
	type plain rolesFile // no UnmarshalYAML method, so no recursion
	return n.Decode((*plain)(f))
}

type roleEntry struct {
	Description string `yaml:"description"`
}

var roleKeys = []string{"description"}

func (r *roleEntry) UnmarshalYAML(n *yaml.Node) error {
	if err := checkKeys(n, roleKeys, "role"); err != nil {
		return err
	}
	type plain roleEntry
	return n.Decode((*plain)(r))
}

type teamFile struct {
	Team         string      `yaml:"team"`
	DefaultRoles []string    `yaml:"default_roles"`
	Users        []userEntry `yaml:"users"`
}

var teamKeys = []string{"team", "default_roles", "users"}

func (t *teamFile) UnmarshalYAML(n *yaml.Node) error {
	if err := checkKeys(n, teamKeys, "team file"); err != nil {
		return err
	}
	type plain teamFile
	return n.Decode((*plain)(t))
}

type userEntry struct {
	Username string   `yaml:"username"`
	FullName string   `yaml:"full_name"`
	Email    string   `yaml:"email"`
	Roles    []string `yaml:"roles"`
	Disabled bool     `yaml:"disabled"`

	line int             // where the entry starts, for error messages
	has  map[string]bool // which keys were present, even if empty
}

var userKeys = []string{"username", "full_name", "email", "roles", "disabled"}

// UnmarshalYAML also records the entry's line number and which keys it had.
// Decoding alone cannot tell "roles:" with no value from no roles key at all.
func (u *userEntry) UnmarshalYAML(n *yaml.Node) error {
	if err := checkKeys(n, userKeys, "user entry"); err != nil {
		return err
	}
	type plain userEntry
	if err := n.Decode((*plain)(u)); err != nil {
		return err
	}
	u.line = n.Line
	u.has = map[string]bool{}
	for i := 0; i < len(n.Content); i += 2 {
		u.has[n.Content[i].Value] = true
	}
	return nil
}

// checkKeys returns an error for the first key in mapping n that is not in
// allowed. what names the thing being decoded, for the message.
func checkKeys(n *yaml.Node, allowed []string, what string) error {
	if n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(n.Content); i += 2 {
		if k := n.Content[i]; !slices.Contains(allowed, k.Value) {
			return fmt.Errorf("line %d: unknown key %q in %s", k.Line, k.Value, what)
		}
	}
	return nil
}
