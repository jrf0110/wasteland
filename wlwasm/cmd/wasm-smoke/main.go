//go:build !js

// Package main provides a host-side smoke test for the libwl wasm
// call graph. It exercises the same Go entry points the wasm bridge
// exposes (`Browse`, `Claim`, etc.) against a real DoltHub upstream,
// without going through the syscall/js bridge. Useful for regressing
// the pure-Go side of the bundle independently of any cross-language
// integration.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/gastownhall/wasteland/internal/backend"
	"github.com/gastownhall/wasteland/internal/commons"
	"github.com/gastownhall/wasteland/internal/federation"
	"github.com/gastownhall/wasteland/internal/sdk"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: spike-smoke browse")
		os.Exit(2)
	}
	token := os.Getenv("DOLTHUB_TOKEN")
	if token == "" {
		fmt.Fprintln(os.Stderr, "DOLTHUB_TOKEN is unset; skipping live smoke test")
		return
	}
	upstream := os.Getenv("WLWASM_UPSTREAM")
	if upstream == "" {
		upstream = "hop/wl-commons"
	}
	org := os.Getenv("DOLTHUB_ORG")
	if org == "" {
		org = "hop"
	}
	switch os.Args[1] {
	case "browse":
		upstreamOrg, upstreamDB, err := federation.ParseUpstream(upstream)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid upstream: %v\n", err)
			os.Exit(1)
		}
		db := backend.NewRemoteDB(token, upstreamOrg, upstreamDB, org, upstreamDB, federation.ModePR)
		client := sdk.New(sdk.ClientConfig{
			DB: db, RigHandle: getenv("WLWASM_RIG", "wasm-spike"), Upstream: upstream,
			Mode: federation.ModePR, BestEffortPendingReads: true, DisableGitHubCache: true,
		})
		result, err := client.Browse(commons.BrowseFilter{View: "all", Limit: 5, Priority: commons.PriorityUnset})
		if err != nil {
			fmt.Fprintf(os.Stderr, "browse failed: %v\n", err)
			os.Exit(1)
		}
		_ = json.NewEncoder(os.Stdout).Encode(result)
	default:
		fmt.Fprintf(os.Stderr, "unknown smoke command: %s\n", os.Args[1])
		os.Exit(2)
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
