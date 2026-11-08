package model

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const roles = `
roles:
  admin:
    description: Full access
  developer:
  oncall:
`

func TestParse(t *testing.T) {
	d, err := Parse([]byte(roles), map[string][]byte{
		"platform.yaml": []byte(`
team: platform
default_roles: [developer]
users:
  - username: grace
    full_name: Grace Hopper
    roles: [admin]
  - username: dennis
    full_name: Dennis Ritchie
  - username: alan
    full_name: Alan Turing
    roles: [developer, oncall]
    email: alan.turing@example.org
  - username: charles
    full_name: Charles Babbage
    roles: [admin]
    disabled: true
`),
	}, "example.com")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		user  string
		email string
		roles []string
	}{
		{"grace", "grace@example.com", []string{"admin"}},                    // one role
		{"dennis", "dennis@example.com", []string{"developer"}},              // team default_roles
		{"alan", "alan.turing@example.org", []string{"developer", "oncall"}}, // two roles, email override
	}
	for _, tt := range tests {
		u, ok := d.Users[tt.user]
		if !ok {
			t.Errorf("user %s missing", tt.user)
			continue
		}
		if u.Email != tt.email {
			t.Errorf("%s: email = %q, want %q", tt.user, u.Email, tt.email)
		}
		if !slices.Equal(u.Roles, tt.roles) {
			t.Errorf("%s: roles = %v, want %v", tt.user, u.Roles, tt.roles)
		}
	}

	if _, ok := d.Users["charles"]; ok {
		t.Error("disabled user charles should be skipped")
	}
	if got := len(d.Bindings); got != 4 {
		t.Errorf("got %d bindings, want 4", got)
	}
	if got := d.Roles["developer"].Description; got != "" {
		t.Errorf("role with no body: description = %q, want empty", got)
	}
}

// Every one of these is either impossible or a silent wrong answer in HCL.
func TestParseErrors(t *testing.T) {
	tests := []struct {
		name  string
		teams map[string][]byte
		want  string
	}{
		{
			name: "unknown role",
			teams: map[string][]byte{"a.yaml": []byte(`
team: a
users:
  - username: ada
    full_name: Ada Lovelace
    roles: [superuser]
`)},
			want: `a.yaml:4: user "ada" has role "superuser", which is not in the roles file`,
		},
		{
			name: "no roles and no default",
			teams: map[string][]byte{"a.yaml": []byte(`
team: a
users:
  - username: ada
    full_name: Ada Lovelace
`)},
			want: `a.yaml:4: user "ada" has no roles, and team "a" has no default_roles`,
		},
		{
			name: "role instead of roles",
			teams: map[string][]byte{"a.yaml": []byte(`
team: a
default_roles: [developer]
users:
  - username: ada
    full_name: Ada Lovelace
    role: [admin]
`)},
			want: `line 7: unknown key "role" in user entry`,
		},
		{
			name: "roles is a string",
			teams: map[string][]byte{"a.yaml": []byte(`
team: a
users:
  - username: ada
    full_name: Ada Lovelace
    roles: admin
`)},
			want: "line 6: cannot unmarshal !!str `admin` into []string",
		},
		{
			name: "no full_name",
			teams: map[string][]byte{"a.yaml": []byte(`
team: a
users:
  - username: ada
    roles: [admin]
`)},
			want: `a.yaml:4: user "ada" has no full_name`,
		},
		{
			name: "explicit empty roles list",
			teams: map[string][]byte{"a.yaml": []byte(`
team: a
default_roles: [developer]
users:
  - username: ada
    full_name: Ada Lovelace
    roles: []
`)},
			want: `a.yaml:5: user "ada" has an empty roles list`,
		},
		{
			name: "roles key with no value",
			teams: map[string][]byte{"a.yaml": []byte(`
team: a
default_roles: [developer]
users:
  - username: ada
    full_name: Ada Lovelace
    roles:
`)},
			want: `a.yaml:5: user "ada" has an empty roles list`,
		},
		{
			name: "explicitly empty email",
			teams: map[string][]byte{"a.yaml": []byte(`
team: a
users:
  - username: ada
    full_name: Ada Lovelace
    roles: [admin]
    email: ""
`)},
			want: `a.yaml:4: user "ada" has an empty email`,
		},
		{
			name: "role listed twice",
			teams: map[string][]byte{"a.yaml": []byte(`
team: a
users:
  - username: ada
    full_name: Ada Lovelace
    roles: [admin, admin]
`)},
			want: `a.yaml:4: user "ada" lists role "admin" twice`,
		},
		{
			name: "no team name",
			teams: map[string][]byte{"a.yaml": []byte(`
users:
  - username: ada
    full_name: Ada Lovelace
    roles: [admin]
`)},
			want: `a.yaml: no team name`,
		},
		{
			name: "typo in a team file key",
			teams: map[string][]byte{"a.yaml": []byte(`
team: a
default_role: [developer]
users:
  - username: ada
    full_name: Ada Lovelace
`)},
			want: `a.yaml: line 3: unknown key "default_role" in team file`,
		},
		{
			name: "same user in two teams",
			teams: map[string][]byte{
				"a.yaml": []byte("team: a\nusers:\n  - username: ada\n    full_name: Ada Lovelace\n    roles: [admin]\n"),
				"b.yaml": []byte("team: b\nusers:\n  - username: ada\n    full_name: Ada Lovelace\n    roles: [admin]\n"),
			},
			want: `b.yaml:3: user "ada" is already defined at a.yaml:3`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(roles), tt.teams, "example.com")
			if err == nil {
				t.Fatalf("got no error, want %q", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("got error %q, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestParseRolesFileTypo(t *testing.T) {
	_, err := Parse([]byte("roles:\n  admin:\n    descripton: Full access\n"), nil, "example.com")
	want := `roles file: line 3: unknown key "descripton" in role`
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("got error %v, want it to contain %q", err, want)
	}
}

func TestParseNoEmailDomain(t *testing.T) {
	_, err := Parse([]byte(roles), map[string][]byte{
		"a.yaml": []byte("team: a\nusers:\n  - username: ada\n    full_name: Ada Lovelace\n    roles: [admin]\n"),
	}, "")
	want := `a.yaml:3: user "ada" has no email, and no email domain was given`
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("got error %v, want it to contain %q", err, want)
	}
}

// writeDataDir lays out a data directory under t.TempDir and returns its path.
func writeDataDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestLoadNoTeamFiles(t *testing.T) {
	dir := writeDataDir(t, map[string]string{"roles.yaml": roles})
	_, err := Load(dir, "example.com")
	want := "no team files (*.yaml or *.yml)"
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("got error %v, want it to contain %q", err, want)
	}
}

func TestLoadYmlTeamFile(t *testing.T) {
	dir := writeDataDir(t, map[string]string{
		"roles.yaml":   roles,
		"teams/a.yml":  "team: a\nusers:\n  - username: ada\n    full_name: Ada Lovelace\n    roles: [admin]\n",
		"teams/b.yaml": "team: b\nusers:\n  - username: alan\n    full_name: Alan Turing\n    roles: [admin]\n",
	})
	d, err := Load(dir, "example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Users) != 2 {
		t.Errorf("got %d users, want 2: a .yml team file must not be skipped", len(d.Users))
	}
}

// The repo's own data must always load cleanly.
func TestLoad(t *testing.T) {
	d, err := Load("../../../data", "example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Users) == 0 || len(d.Roles) == 0 || len(d.Bindings) == 0 {
		t.Errorf("got %d users, %d roles, %d bindings; want some of each", len(d.Users), len(d.Roles), len(d.Bindings))
	}
}
