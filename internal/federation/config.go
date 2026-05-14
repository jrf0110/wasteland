// Package federation implements the Wasteland federation protocol.
//
// The Wasteland is a federation of Gas Towns via DoltHub. Each rig has a
// sovereign fork of a shared commons database. Rigs register by writing
// to the commons' rigs table, and contribute wanted work items and
// completions through DoltHub's fork/PR/merge primitives.
//
// This file defines the data types and value-only helpers that compile
// in any build mode, including GOOS=js GOARCH=wasm. Subprocess- and
// filesystem-coupled helpers (Service, DoltCLI, Join, Create,
// LocalCloneDir, file-backed ConfigStore) live in `federation.go`,
// which is gated behind `//go:build !js`.
package federation

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrNotJoined indicates the rig has not joined a wasteland.
var ErrNotJoined = errors.New("rig has not joined a wasteland")

// ErrAmbiguous indicates multiple wastelands are joined and --wasteland is required.
var ErrAmbiguous = errors.New("multiple wastelands joined; use --wasteland to select one")

// Mode constants for the wasteland workflow.
const (
	ModeWildWest = "wild-west"
	ModePR       = "pr"
)

// Backend constants.
const (
	BackendRemote = "remote"
	BackendLocal  = "local"
)

// Config holds the wasteland configuration for a rig.
type Config struct {
	// Upstream is the DoltHub path of the upstream commons (e.g., "steveyegge/wl-commons").
	Upstream string `json:"upstream"`

	// ProviderType is the upstream provider ("dolthub", "file", "git", "github").
	ProviderType string `json:"provider_type,omitempty"`

	// UpstreamURL is the resolved dolt-compatible remote URL for the upstream.
	// Used by browse for ephemeral clones.
	UpstreamURL string `json:"upstream_url,omitempty"`

	// ForkOrg is the DoltHub org where the fork lives (e.g., "alice-dev").
	ForkOrg string `json:"fork_org"`

	// ForkDB is the database name of the fork (e.g., "wl-commons").
	ForkDB string `json:"fork_db"`

	// LocalDir is the absolute path to the local clone of the fork.
	LocalDir string `json:"local_dir"`

	// RigHandle is the rig's handle in the registry.
	RigHandle string `json:"rig_handle"`

	// HopURI is the rig's HOP protocol URI (e.g., "hop://alice@example.com/alice-rig/").
	HopURI string `json:"hop_uri,omitempty"`

	// JoinedAt is when the rig joined the wasteland.
	JoinedAt time.Time `json:"joined_at"`

	// Mode is the workflow mode: "" or "pr" (default) or "wild-west".
	Mode string `json:"mode,omitempty"`

	// Backend is the database backend: "remote" (DoltHub API, default) or "local" (dolt CLI).
	Backend string `json:"backend,omitempty"`

	// Signing enables GPG-signed Dolt commits when true.
	Signing bool `json:"signing,omitempty"`

	// LastSyncAt records when the local clone was last synced with upstream.
	LastSyncAt *time.Time `json:"last_sync_at,omitempty"`

	// GitHubRepo is the upstream GitHub repo for PR shells (e.g., "steveyegge/wl-commons").
	//
	// Deprecated: use ProviderType == "github" instead.
	GitHubRepo string `json:"github_repo,omitempty"`
}

// ResolveMode returns the effective mode, defaulting to PR mode.
func (c *Config) ResolveMode() string {
	if c.Mode == "" || c.Mode == ModePR {
		return ModePR
	}
	return c.Mode
}

// ResolveBackend returns the effective backend.
// Explicit "local" or "remote" values are returned as-is.
// When unset, defaults to "local" if LocalDir is configured (backward compat),
// "remote" otherwise (new remote-only users).
func (c *Config) ResolveBackend() string {
	switch c.Backend {
	case BackendLocal:
		return BackendLocal
	case BackendRemote:
		return BackendRemote
	default:
		if c.LocalDir != "" {
			return BackendLocal
		}
		return BackendRemote
	}
}

// ResolveProviderType returns the effective provider type.
// Falls back to "dolthub" for backward compatibility with old configs.
func (c *Config) ResolveProviderType() string {
	if c.ProviderType != "" {
		return c.ProviderType
	}
	return "dolthub"
}

// IsGitHub returns true if the provider type is "github".
func (c *Config) IsGitHub() bool {
	return c.ResolveProviderType() == "github"
}

// ParseUpstream parses an upstream path like "steveyegge/wl-commons" into org and db.
func ParseUpstream(upstream string) (org, db string, err error) {
	parts := strings.SplitN(upstream, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid upstream path %q: expected format 'org/database'", upstream)
	}
	return parts[0], parts[1], nil
}

// ConfigStore abstracts wasteland config persistence.
//
// The host build provides a file-backed implementation in
// `federation.go`. Other implementations (in-memory, durable-object-
// backed, etc.) can be supplied by callers that consume the wasm
// build target — see `wlwasm/configstore_mem.go` for an example.
type ConfigStore interface {
	Load(upstream string) (*Config, error)
	Save(cfg *Config) error
	Delete(upstream string) error
	List() ([]string, error)
}

// ResolveConfig resolves the active wasteland config.
// If explicit is non-empty, loads that specific upstream config.
// If exactly one wasteland is joined, returns it.
// If zero are joined, returns ErrNotJoined.
// If multiple are joined, returns ErrAmbiguous.
func ResolveConfig(store ConfigStore, explicit string) (*Config, error) {
	if explicit != "" {
		cfg, err := store.Load(explicit)
		if err != nil {
			return nil, fmt.Errorf("loading config for %s: %w", explicit, err)
		}
		return cfg, nil
	}

	upstreams, err := store.List()
	if err != nil {
		return nil, fmt.Errorf("listing wastelands: %w", err)
	}

	switch len(upstreams) {
	case 0:
		return nil, fmt.Errorf("%w (run 'wl join <upstream>')", ErrNotJoined)
	case 1:
		cfg, err := store.Load(upstreams[0])
		if err != nil {
			return nil, fmt.Errorf("loading config for %s: %w", upstreams[0], err)
		}
		return cfg, nil
	default:
		var list strings.Builder
		for _, u := range upstreams {
			fmt.Fprintf(&list, "  - %s\n", u)
		}
		return nil, fmt.Errorf("%w:\n%s", ErrAmbiguous, list.String())
	}
}
