// Package main exports the libwl wasm spike API.
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/gastownhall/wasteland/internal/backend"
	"github.com/gastownhall/wasteland/internal/commons"
	"github.com/gastownhall/wasteland/internal/federation"
	"github.com/gastownhall/wasteland/internal/remote"
	"github.com/gastownhall/wasteland/internal/sdk"
)

type Env struct {
	Upstream     string `json:"upstream"`
	DoltHubToken string `json:"dolthub_token"`
	UserID       string `json:"user_id"`
	RigHandle    string `json:"rig_handle"`
	ForkOrg      string `json:"fork_org"`
	ForkDB       string `json:"fork_db"`
	Direct       bool   `json:"direct"`
}

// BrowseInput mirrors commons.BrowseFilter for the wasm bridge. We use
// `*int` for Priority so that omitting the JSON field is distinguishable
// from sending priority=0 (the "low" priority level). When nil, Browse
// passes commons.PriorityUnset to the SDK; otherwise the integer value
// is used as-is.
type BrowseInput struct {
	Env
	Status    string `json:"status,omitempty"`
	Project   string `json:"project,omitempty"`
	Type      string `json:"type,omitempty"`
	Priority  *int   `json:"priority,omitempty"`
	Limit     int    `json:"limit,omitempty"`
	PostedBy  string `json:"posted_by,omitempty"`
	ClaimedBy string `json:"claimed_by,omitempty"`
	Search    string `json:"search,omitempty"`
	View      string `json:"view,omitempty"`
	Long      bool   `json:"long,omitempty"`
}

type ItemInput struct {
	Env
	ItemID string `json:"item_id"`
}

type DoneInput struct {
	Env
	ItemID   string `json:"item_id"`
	Evidence string `json:"evidence"`
}

type PostInput struct {
	Env
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Project     string   `json:"project"`
	Type        string   `json:"type"`
	Priority    int      `json:"priority"`
	EffortLevel string   `json:"effort_level"`
	Tags        []string `json:"tags"`
}

type AcceptInput struct {
	Env
	ItemID      string   `json:"item_id"`
	Quality     int      `json:"quality"`
	Reliability int      `json:"reliability"`
	Severity    string   `json:"severity"`
	SkillTags   []string `json:"skill_tags"`
	Message     string   `json:"message"`
}

type RejectInput struct {
	Env
	ItemID string `json:"item_id"`
	Reason string `json:"reason"`
}

type Result struct {
	Browse   *sdk.BrowseResult   `json:"browse,omitempty"`
	Mutation *sdk.MutationResult `json:"mutation,omitempty"`
}

func Browse(in BrowseInput) (*sdk.BrowseResult, error) {
	client, err := newClient(in.Env)
	if err != nil {
		return nil, err
	}
	priority := commons.PriorityUnset
	if in.Priority != nil {
		priority = *in.Priority
	}
	return client.Browse(commons.BrowseFilter{
		Status: in.Status, Project: in.Project, Type: in.Type, Priority: priority,
		Limit: in.Limit, PostedBy: in.PostedBy, ClaimedBy: in.ClaimedBy, Search: in.Search,
		View: in.View, Long: in.Long,
	})
}

func Claim(in ItemInput) (*sdk.MutationResult, error) {
	client, err := newClient(in.Env)
	if err != nil {
		return nil, err
	}
	return client.Claim(in.ItemID)
}

func Unclaim(in ItemInput) (*sdk.MutationResult, error) {
	client, err := newClient(in.Env)
	if err != nil {
		return nil, err
	}
	return client.Unclaim(in.ItemID)
}

func Done(in DoneInput) (*sdk.MutationResult, error) {
	client, err := newClient(in.Env)
	if err != nil {
		return nil, err
	}
	return client.Done(in.ItemID, in.Evidence)
}

func Post(in PostInput) (*sdk.MutationResult, error) {
	client, err := newClient(in.Env)
	if err != nil {
		return nil, err
	}
	return client.Post(sdk.PostInput{
		Title: in.Title, Description: in.Description, Project: in.Project, Type: in.Type,
		Priority: in.Priority, EffortLevel: in.EffortLevel, Tags: in.Tags,
	})
}

func Accept(in AcceptInput) (*sdk.MutationResult, error) {
	client, err := newClient(in.Env)
	if err != nil {
		return nil, err
	}
	return client.Accept(in.ItemID, sdk.AcceptInput{
		Quality: in.Quality, Reliability: in.Reliability, Severity: in.Severity,
		SkillTags: in.SkillTags, Message: in.Message,
	})
}

func Reject(in RejectInput) (*sdk.MutationResult, error) {
	client, err := newClient(in.Env)
	if err != nil {
		return nil, err
	}
	return client.Reject(in.ItemID, in.Reason)
}

func Close(in ItemInput) (*sdk.MutationResult, error) {
	client, err := newClient(in.Env)
	if err != nil {
		return nil, err
	}
	return client.Close(in.ItemID)
}

func newClient(env Env) (*sdk.Client, error) {
	if env.Upstream == "" {
		return nil, fmt.Errorf("upstream is required")
	}
	if env.DoltHubToken == "" {
		return nil, fmt.Errorf("dolthub_token is required")
	}
	upstreamOrg, upstreamDB, err := federation.ParseUpstream(env.Upstream)
	if err != nil {
		return nil, err
	}
	forkOrg := env.ForkOrg
	if forkOrg == "" {
		forkOrg = env.UserID
	}
	if forkOrg == "" {
		forkOrg = upstreamOrg
	}
	forkDB := env.ForkDB
	if forkDB == "" {
		forkDB = upstreamDB
	}
	mode := federation.ModePR
	if env.Direct {
		mode = federation.ModeWildWest
		forkOrg = upstreamOrg
		forkDB = upstreamDB
	}
	cfg := &federation.Config{
		Upstream: env.Upstream, ProviderType: "dolthub", ForkOrg: forkOrg, ForkDB: forkDB,
		RigHandle: env.RigHandle, HopURI: fmt.Sprintf("hop://%s/%s/", env.UserID, env.RigHandle),
		JoinedAt: time.Now(), Mode: mode, Backend: federation.BackendRemote,
	}
	_ = newMemConfigStore(cfg)
	db := backend.NewRemoteDB(env.DoltHubToken, upstreamOrg, upstreamDB, forkOrg, forkDB, mode)
	provider := remote.NewDoltHubProvider(env.DoltHubToken)
	return sdk.New(sdk.ClientConfig{
		DB: db, RigHandle: cfg.RigHandle, Upstream: cfg.Upstream, Mode: cfg.ResolveMode(), HopURI: cfg.HopURI,
		BestEffortPendingReads: true, DisableGitHubCache: true,
		CreatePR: func(branch string) (string, error) {
			return provider.CreatePR(forkOrg, upstreamOrg, upstreamDB, branch, "[wl] "+branch, "")
		},
		CheckPRContext: func(ctx context.Context, branch string) string {
			url, _ := provider.WithContext(ctx).FindPR(upstreamOrg, upstreamDB, forkOrg, branch)
			return url
		},
		CheckPR: func(branch string) string {
			url, _ := provider.FindPR(upstreamOrg, upstreamDB, forkOrg, branch)
			return url
		},
		ClosePR: func(branch string) error {
			_, id := provider.FindPR(upstreamOrg, upstreamDB, forkOrg, branch)
			if id == "" {
				return nil
			}
			return provider.ClosePR(upstreamOrg, upstreamDB, id)
		},
		BranchURL: func(branch string) string {
			return fmt.Sprintf("https://www.dolthub.com/repositories/%s/%s/tree/%s", forkOrg, forkDB, branch)
		},
	}), nil
}
