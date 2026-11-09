package domain

import (
	"encoding/json"
	"errors"
	"fmt"
)

// The members of Claude Code's files that make up the active login. Nothing
// else in those files is ever written (CLAUDE.md rule 3).
const (
	credentialsKey  = "claudeAiOauth"
	organizationKey = "organizationUuid"
	profileKey      = "oauthAccount"
)

// PatchCredentialsFile returns ~/.claude/.credentials.json with claudeAiOauth
// (and organizationUuid when the login has one) set to l's. Every other
// top-level member, such as mcpOAuth, keeps its raw bytes and position; an
// absent member is appended.
func PatchCredentialsFile(file []byte, l Login) ([]byte, error) {
	if !isObject(l.Credentials) {
		return nil, errors.New("login has no claudeAiOauth object")
	}
	fields := []field{{credentialsKey, l.Credentials}}
	if l.OrganizationUUID != nil {
		fields = append(fields, field{organizationKey, l.OrganizationUUID})
	}
	out, err := setMembers(file, fields)
	if err != nil {
		return nil, fmt.Errorf("patch .credentials.json: %w", err)
	}
	return out, nil
}

// PatchConfigFile returns ~/.claude.json with oauthAccount set to l's
// profile. Every other top-level member keeps its raw bytes and position.
func PatchConfigFile(file []byte, l Login) ([]byte, error) {
	if !isObject(l.Profile) {
		return nil, errors.New("login has no oauthAccount object")
	}
	out, err := setMembers(file, []field{{profileKey, l.Profile}})
	if err != nil {
		return nil, fmt.Errorf("patch .claude.json: %w", err)
	}
	return out, nil
}

// LoginFromFiles reads the login out of the two files' contents.
func LoginFromFiles(credentials, config []byte) (Login, error) {
	var c, p map[string]json.RawMessage
	if err := json.Unmarshal(credentials, &c); err != nil {
		return Login{}, fmt.Errorf(".credentials.json: %w", err)
	}
	if err := json.Unmarshal(config, &p); err != nil {
		return Login{}, fmt.Errorf(".claude.json: %w", err)
	}
	l := Login{Credentials: c[credentialsKey], OrganizationUUID: c[organizationKey], Profile: p[profileKey]}
	if !isObject(l.Credentials) {
		return Login{}, errors.New(".credentials.json has no claudeAiOauth object (not logged in?)")
	}
	if !isObject(l.Profile) {
		return Login{}, errors.New(".claude.json has no oauthAccount object (not logged in?)")
	}
	return l, nil
}
