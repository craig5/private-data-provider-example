"""Consistency checks for ../../data, for the HCL-only version in ../main.tf.

HCL cannot unit test its own parsing, so these checks live in a second
language, in a second place, and run as a separate CI step. Every rule here
is a copy of a rule in main.tf. Nothing keeps the two in sync: change one
and the other is silently out of date.

Compare provider/internal/model, where the rules and their tests are the
same code.

Run with `make test-tf-hcl-only` from the repo root.
"""

from pathlib import Path

import pytest
import yaml

DATA = Path(__file__).resolve().parents[2] / "data"

USER_KEYS = {"username", "full_name", "email", "roles", "disabled"}
TEAM_KEYS = {"team", "default_roles", "users"}
ROLE_KEYS = {"description"}


def load_roles():
    return yaml.safe_load((DATA / "roles.yaml").read_text())["roles"]


def team_files():
    teams = DATA / "teams"
    return sorted([*teams.glob("*.yaml"), *teams.glob("*.yml")])


def load_teams():
    return {path.name: yaml.safe_load(path.read_text()) for path in team_files()}


def all_users():
    """Every user entry, disabled or not, as (file, team, user)."""
    return [
        (file, team, user)
        for file, team in load_teams().items()
        for user in team["users"]
    ]


def enabled_users():
    return [entry for entry in all_users() if not entry[2].get("disabled", False)]


def effective_roles(team, user):
    """The user's roles, or the team's default_roles if the key is missing.

    A key that is there but empty stays empty: it does not fall back to the
    default.
    """
    if "roles" in user:
        return user["roles"] or []
    return team.get("default_roles") or []


def user_id(entry):
    file, _, user = entry
    return f"{file}:{user.get('username', '?')}"


@pytest.mark.parametrize("entry", all_users(), ids=user_id)
def test_user_has_only_known_keys(entry):
    _, _, user = entry
    unknown = set(user) - USER_KEYS
    assert not unknown, f"unknown keys {sorted(unknown)} (typo for one of {sorted(USER_KEYS)}?)"


@pytest.mark.parametrize("entry", enabled_users(), ids=user_id)
def test_user_has_username_and_full_name(entry):
    _, _, user = entry
    assert user.get("username"), "missing username"
    assert user.get("full_name"), "missing full_name"


@pytest.mark.parametrize("entry", enabled_users(), ids=user_id)
def test_roles_is_a_list(entry):
    _, _, user = entry
    if "roles" in user:
        assert isinstance(user["roles"], list), f"roles must be a list, got {user['roles']!r}"


def test_there_are_team_files():
    # No team files means no users, and a plan that deletes every user.
    assert team_files(), f"no team files (*.yaml or *.yml) in {DATA / 'teams'}"


@pytest.mark.parametrize("entry", enabled_users(), ids=user_id)
def test_user_has_at_least_one_role(entry):
    _, team, user = entry
    if "roles" in user:
        assert user["roles"], "empty roles list"
    assert effective_roles(team, user), f"no roles, and team {team.get('team')!r} has no default_roles"


@pytest.mark.parametrize("entry", enabled_users(), ids=user_id)
def test_email_is_not_empty(entry):
    _, _, user = entry
    if "email" in user:
        assert user["email"], "empty email"


@pytest.mark.parametrize("entry", enabled_users(), ids=user_id)
def test_user_only_uses_roles_that_exist(entry):
    _, team, user = entry
    unknown = set(effective_roles(team, user)) - set(load_roles())
    assert not unknown, f"roles {sorted(unknown)} are not in roles.yaml"


@pytest.mark.parametrize("entry", enabled_users(), ids=user_id)
def test_user_lists_each_role_once(entry):
    _, team, user = entry
    roles = effective_roles(team, user)
    twice = sorted({r for r in roles if roles.count(r) > 1})
    assert not twice, f"roles {twice} listed more than once"


@pytest.mark.parametrize("file", sorted(load_teams()))
def test_team_has_a_name(file):
    assert load_teams()[file].get("team"), "no team name"


@pytest.mark.parametrize("file", sorted(load_teams()))
def test_team_has_only_known_keys(file):
    unknown = set(load_teams()[file]) - TEAM_KEYS
    assert not unknown, f"unknown keys {sorted(unknown)} (typo for one of {sorted(TEAM_KEYS)}?)"


def test_roles_file_has_only_known_keys():
    roles_file = yaml.safe_load((DATA / "roles.yaml").read_text())
    assert set(roles_file) <= {"roles"}, f"unknown keys {sorted(set(roles_file) - {'roles'})}"
    for name, role in roles_file["roles"].items():
        unknown = set(role or {}) - ROLE_KEYS
        assert not unknown, f"role {name!r} has unknown keys {sorted(unknown)}"


@pytest.mark.parametrize("file", sorted(load_teams()))
def test_team_default_roles_exist(file):
    team = load_teams()[file]
    unknown = set(team.get("default_roles", [])) - set(load_roles())
    assert not unknown, f"default_roles {sorted(unknown)} are not in roles.yaml"


def test_usernames_are_unique_across_teams():
    seen = {}
    duplicates = []
    for file, _, user in enabled_users():
        name = user["username"]
        if name in seen:
            duplicates.append(f"{name} in {seen[name]} and {file}")
        seen[name] = file
    assert not duplicates, f"duplicate usernames: {duplicates}"
