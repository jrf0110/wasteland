package commons

import (
	"fmt"
	"strings"
)

// BranchName returns the conventional branch name for a PR-mode mutation.
func BranchName(rigHandle, wantedID string) string {
	return fmt.Sprintf("wl/%s/%s", rigHandle, wantedID)
}

// ListWantedIDs returns wanted item IDs, optionally filtered by status.
func ListWantedIDs(db DB, statusFilter string) ([]string, error) {
	query := "SELECT id FROM wanted"
	if statusFilter != "" {
		query += fmt.Sprintf(" WHERE status = '%s'", EscapeSQL(statusFilter))
	}
	query += " ORDER BY created_at DESC LIMIT 50"
	out, err := db.Query(query, "")
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 {
		return nil, nil
	}
	var ids []string
	for _, line := range lines[1:] {
		id := strings.TrimSpace(line)
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// ResolveWantedID resolves a wanted ID or unambiguous prefix to a full ID.
func ResolveWantedID(db DB, idOrPrefix string) (string, error) {
	query := fmt.Sprintf("SELECT id FROM wanted WHERE id LIKE '%s%%' LIMIT 3", EscapeLIKE(idOrPrefix))
	out, err := db.Query(query, "")
	if err != nil {
		return "", err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 {
		return "", fmt.Errorf("no wanted item matching %q", idOrPrefix)
	}
	var matches []string
	for _, line := range lines[1:] {
		id := strings.TrimSpace(line)
		if id != "" {
			matches = append(matches, id)
		}
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no wanted item matching %q", idOrPrefix)
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("ambiguous prefix %q matches: %s", idOrPrefix, strings.Join(matches, ", "))
	}
	return matches[0], nil
}

// QueryItemStatus returns the status of a wanted item at a specific ref.
// If ref is empty, queries the working copy.
// Returns (status, true, nil) if found, ("", false, nil) if not found,
// or ("", false, err) if the query failed.
func QueryItemStatus(db DB, wantedID, ref string) (string, bool, error) {
	query := fmt.Sprintf(
		"SELECT status FROM wanted WHERE id = '%s'",
		EscapeSQL(wantedID),
	)
	out, err := db.Query(query, ref)
	if err != nil {
		return "", false, err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 {
		return "", false, nil
	}
	return strings.TrimSpace(lines[1]), true, nil
}

// QueryItemStatusAsOf is a convenience wrapper that returns "" on not-found or error.
//
// Deprecated: prefer QueryItemStatus for explicit error handling.
func QueryItemStatusAsOf(db DB, wantedID, ref string) string {
	status, _, _ := QueryItemStatus(db, wantedID, ref)
	return status
}
