package auth

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// DefaultHostGroups defines the Linux groups that grant host-level container visibility.
var DefaultHostGroups = []string{"wheel", "sudo", "gotty-host"}

// UserIdentity holds the resolved system user and container privilege level.
type UserIdentity struct {
	Username     string   `json:"username"`
	UID          uint32   `json:"uid"`
	GID          uint32   `json:"gid"`
	GECOS        string   `json:"gecos"`
	HomeDir      string   `json:"home_dir"`
	Shell        string   `json:"shell"`
	Groups       []string `json:"groups"`
	GroupIDs     []uint32 `json:"group_ids"`
	IsHostAccess bool     `json:"is_host_access"`
}

// Resolver resolves an X-Forwarded-User header value against /etc/passwd and /etc/group.
type Resolver struct {
	PasswdPath string
	GroupPath  string
	HostGroups []string
}

// NewResolver returns a Resolver configured with standard system paths.
func NewResolver() *Resolver {
	return &Resolver{
		PasswdPath: "/etc/passwd",
		GroupPath:  "/etc/group",
		HostGroups: DefaultHostGroups,
	}
}

// Resolve maps a username or email address from X-Forwarded-User to a UserIdentity.
// Returns an error if the header is empty or does not match any entry.
func (r *Resolver) Resolve(headerVal string) (*UserIdentity, error) {
	ident := strings.TrimSpace(headerVal)
	if ident == "" {
		return nil, fmt.Errorf("auth: empty identity header")
	}

	passwdPath := r.PasswdPath
	if passwdPath == "" {
		passwdPath = "/etc/passwd"
	}

	f, err := os.Open(passwdPath)
	if err != nil {
		return nil, fmt.Errorf("auth: failed to open passwd file: %w", err)
	}
	defer f.Close()

	var matched *UserIdentity
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) < 7 {
			continue
		}
		name := parts[0]
		uidStr := parts[2]
		gidStr := parts[3]
		gecos := parts[4]
		homeDir := parts[5]
		shell := parts[6]

		if matchesIdentity(ident, name, gecos) {
			uid, err := strconv.ParseUint(uidStr, 10, 32)
			if err != nil {
				continue
			}
			gid, err := strconv.ParseUint(gidStr, 10, 32)
			if err != nil {
				continue
			}
			matched = &UserIdentity{
				Username: name,
				UID:      uint32(uid),
				GID:      uint32(gid),
				GECOS:    gecos,
				HomeDir:  homeDir,
				Shell:    shell,
			}
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("auth: error reading passwd file: %w", err)
	}
	if matched == nil {
		return nil, fmt.Errorf("auth: unmapped user %s", ident)
	}

	groups, groupIDs, _ := r.resolveGroups(matched.Username, matched.GID)
	matched.Groups = groups
	matched.GroupIDs = groupIDs

	hostGroups := r.HostGroups
	if len(hostGroups) == 0 {
		hostGroups = DefaultHostGroups
	}
	if matched.UID == 0 {
		matched.IsHostAccess = true
	} else {
		for _, g := range groups {
			for _, hg := range hostGroups {
				if g == hg {
					matched.IsHostAccess = true
					break
				}
			}
		}
	}

	return matched, nil
}

func matchesIdentity(ident, username, gecos string) bool {
	if username == ident {
		return true
	}
	identLower := strings.ToLower(ident)
	for _, token := range strings.Split(gecos, ",") {
		t := strings.TrimSpace(token)
		if t == "" {
			continue
		}
		if strings.ToLower(t) == identLower {
			return true
		}
		// Support "Full Name <user@example.com>" format inside a GECOS subfield
		if start := strings.IndexByte(t, '<'); start != -1 {
			if end := strings.IndexByte(t[start+1:], '>'); end != -1 {
				extracted := strings.TrimSpace(t[start+1 : start+1+end])
				if strings.ToLower(extracted) == identLower {
					return true
				}
			}
		}
	}
	return false
}

func (r *Resolver) resolveGroups(username string, primaryGID uint32) ([]string, []uint32, error) {
	groupPath := r.GroupPath
	if groupPath == "" {
		groupPath = "/etc/group"
	}

	f, err := os.Open(groupPath)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	var groups []string
	var gids []uint32
	seen := make(map[string]bool)

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) < 4 {
			continue
		}
		groupName := parts[0]
		gid, err := strconv.ParseUint(parts[2], 10, 32)
		memberList := parts[3]

		isMember := false
		if err == nil && uint32(gid) == primaryGID {
			isMember = true
		}
		if !isMember && memberList != "" {
			for _, m := range strings.Split(memberList, ",") {
				if strings.TrimSpace(m) == username {
					isMember = true
					break
				}
			}
		}

		if isMember && !seen[groupName] {
			seen[groupName] = true
			groups = append(groups, groupName)
			if err == nil {
				gids = append(gids, uint32(gid))
			}
		}
	}

	return groups, gids, scanner.Err()
}
