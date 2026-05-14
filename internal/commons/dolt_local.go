//go:build !js

package commons

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// doltRetry runs fn up to 3 times with backoff (1s, 2s) between attempts.
func doltRetry(fn func() error) error {
	var err error
	for i := 0; i < 3; i++ {
		if i > 0 {
			time.Sleep(time.Duration(i) * time.Second)
		}
		if err = fn(); err == nil {
			return nil
		}
	}
	return err
}

func doltRetryContext(ctx context.Context, fn func(context.Context) error) error {
	var err error
	for i := 0; i < 3; i++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if i > 0 {
			select {
			case <-time.After(time.Duration(i) * time.Second):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if err = fn(ctx); err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return err
}

// DoltHubToken returns the DoltHub API token from the environment.
func DoltHubToken() string { return os.Getenv("DOLTHUB_TOKEN") }

// DoltHubOrg returns the default DoltHub organization from the environment.
func DoltHubOrg() string { return os.Getenv("DOLTHUB_ORG") }

// PushWithSync pushes the local main branch to both upstream and origin remotes.
// If a push is rejected (stale), it pulls to merge and retries.
// Returns an error if any remote push ultimately fails.
func PushWithSync(dbDir string, stdout io.Writer) error {
	var failures []string
	for _, remote := range []string{"upstream", "origin"} {
		if err := pushRemote(dbDir, remote); err != nil {
			fmt.Fprintf(stdout, "  Syncing with %s...\n", remote)
			if pullErr := pullRemote(dbDir, remote); pullErr != nil {
				fmt.Fprintf(stdout, "  warning: sync from %s failed: %v\n", remote, pullErr)
				failures = append(failures, remote)
				continue
			}
			if err := pushRemote(dbDir, remote); err != nil {
				fmt.Fprintf(stdout, "  warning: push to %s failed after sync: %v\n", remote, err)
				failures = append(failures, remote)
				continue
			}
		}
		fmt.Fprintf(stdout, "  Pushed to %s\n", remote)
	}
	if len(failures) > 0 {
		return fmt.Errorf("push failed for remotes: %s", strings.Join(failures, ", "))
	}
	return nil
}

func pushRemote(dbDir, remote string) error {
	return doltRetry(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "dolt", "push", remote, "main")
		cmd.Dir = dbDir
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("dolt push %s main: %w (%s)", remote, err, strings.TrimSpace(string(output)))
		}
		return nil
	})
}

func pullRemote(dbDir, remote string) error {
	return doltRetry(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "dolt", "pull", remote, "main")
		cmd.Dir = dbDir
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("dolt pull %s main: %w (%s)", remote, err, strings.TrimSpace(string(output)))
		}
		return nil
	})
}

// PullUpstream pulls the latest changes from the upstream remote.
func PullUpstream(dbDir string) error { return pullRemote(dbDir, "upstream") }

// ResetMainToUpstream fetches upstream and hard-resets local main to match.
// Used in PR mode where main should always mirror upstream exactly.
func ResetMainToUpstream(dbDir string) error {
	if err := FetchRemote(dbDir, "upstream"); err != nil {
		return err
	}
	return doltExec(dbDir, "reset", "--hard", "upstream/main")
}

// FetchRemote fetches the latest refs from a named remote without merging.
func FetchRemote(dbDir, remote string) error {
	return doltRetry(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "dolt", "fetch", remote)
		cmd.Dir = dbDir
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("dolt fetch %s: %w (%s)", remote, err, strings.TrimSpace(string(output)))
		}
		return nil
	})
}

// DoltSQLScript executes a SQL script against a dolt database directory.
func DoltSQLScript(dbDir, script string) error {
	tmpFile, err := os.CreateTemp("", "dolt-script-*.sql")
	if err != nil {
		return fmt.Errorf("creating temp SQL file: %w", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	if _, err := tmpFile.WriteString(script); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("writing SQL script: %w", err)
	}
	_ = tmpFile.Close()

	return doltRetry(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "dolt", "sql", "--file", tmpFile.Name())
		cmd.Dir = dbDir
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("%w (output: %s)", err, strings.TrimSpace(string(output)))
		}
		return nil
	})
}

// BranchExists checks whether a branch exists in the dolt database.
func BranchExists(dbDir, branch string) (bool, error) {
	out, err := DoltSQLQuery(dbDir, fmt.Sprintf(
		"SELECT COUNT(*) AS cnt FROM dolt_branches WHERE name = '%s'",
		strings.ReplaceAll(branch, "'", "''"),
	))
	if err != nil {
		return false, err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 {
		return false, fmt.Errorf("unexpected dolt_branches output: %s", out)
	}
	return strings.TrimSpace(lines[1]) != "0", nil
}

// CheckoutBranch creates the branch if it doesn't exist, then checks it out.
func CheckoutBranch(dbDir, branch string) error {
	exists, err := BranchExists(dbDir, branch)
	if err != nil {
		return fmt.Errorf("checking branch %s: %w", branch, err)
	}
	if !exists {
		remoteBranch := "remotes/origin/" + branch
		if remoteExists, _ := RemoteBranchExists(dbDir, remoteBranch); remoteExists {
			if err := doltExec(dbDir, "branch", branch, remoteBranch); err != nil {
				return fmt.Errorf("creating branch %s from %s: %w", branch, remoteBranch, err)
			}
		} else if err := doltExec(dbDir, "branch", branch); err != nil {
			return fmt.Errorf("creating branch %s: %w", branch, err)
		}
	}
	return doltExec(dbDir, "checkout", branch)
}

// RemoteBranchExists checks whether a remote tracking branch exists in the
// dolt_remote_branches system table.
func RemoteBranchExists(dbDir, remoteBranch string) (bool, error) {
	escaped := strings.ReplaceAll(remoteBranch, "'", "''")
	out, err := DoltSQLQuery(dbDir, fmt.Sprintf(
		"SELECT COUNT(*) AS cnt FROM dolt_remote_branches WHERE name = '%s'", escaped))
	if err != nil {
		return false, err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 {
		return false, nil
	}
	return strings.TrimSpace(lines[1]) != "0", nil
}

// MergeRemoteTracking merges the origin remote tracking branch into the
// currently checked-out local branch.
func MergeRemoteTracking(dbDir, branch string) error {
	remoteBranch := "remotes/origin/" + branch
	exists, err := RemoteBranchExists(dbDir, remoteBranch)
	if err != nil || !exists {
		return nil
	}
	return doltExec(dbDir, "merge", remoteBranch)
}

// CheckoutBranchFrom checks out a branch if it exists, or creates it from
// startPoint if it doesn't.
func CheckoutBranchFrom(dbDir, branch, startPoint string) error {
	exists, err := BranchExists(dbDir, branch)
	if err != nil {
		return fmt.Errorf("checking branch %s: %w", branch, err)
	}
	if !exists {
		if err := doltExec(dbDir, "branch", branch, startPoint); err != nil {
			return fmt.Errorf("creating branch %s from %s: %w", branch, startPoint, err)
		}
	}
	return doltExec(dbDir, "checkout", branch)
}

// CheckoutMain switches the working directory back to the main branch.
func CheckoutMain(dbDir string) error { return doltExec(dbDir, "checkout", "main") }

// doltExec runs a dolt CLI command in the given database directory.
func doltExec(dbDir string, args ...string) error {
	return doltRetry(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "dolt", args...)
		cmd.Dir = dbDir
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("dolt %s: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
		}
		return nil
	})
}

// PushBranch force-pushes a named branch to origin.
func PushBranch(dbDir, branch string, stdout io.Writer) error {
	err := doltRetry(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "dolt", "push", "--force", "origin", branch)
		cmd.Dir = dbDir
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("push branch %s: %w (%s)", branch, err, strings.TrimSpace(string(output)))
		}
		return nil
	})
	if err != nil {
		fmt.Fprintf(stdout, "  warning: %v\n", err)
		return err
	}
	fmt.Fprintf(stdout, "  Pushed branch %s to origin\n", branch)
	return nil
}

// ListBranches returns branch names matching a prefix (e.g. "wl/").
func ListBranches(dbDir, prefix string) ([]string, error) {
	return ListBranchesContext(context.Background(), dbDir, prefix)
}

// ListBranchesContext returns branch names matching a prefix, binding the underlying dolt query to ctx.
func ListBranchesContext(ctx context.Context, dbDir, prefix string) ([]string, error) {
	out, err := DoltSQLQueryContext(ctx, dbDir, fmt.Sprintf(
		"SELECT name FROM dolt_branches WHERE name LIKE '%s%%' ORDER BY name",
		EscapeLIKE(prefix),
	))
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 {
		return nil, nil
	}
	var branches []string
	for _, line := range lines[1:] {
		name := strings.TrimSpace(line)
		if name != "" {
			branches = append(branches, name)
		}
	}
	return branches, nil
}

// TrackOriginBranches creates local tracking branches for any remotes/origin/{prefix}* branches that don't already exist locally.
func TrackOriginBranches(dbDir, prefix string) error {
	remotePrefix := "remotes/origin/" + prefix
	out, err := DoltSQLQuery(dbDir, fmt.Sprintf(
		"SELECT name FROM dolt_remote_branches WHERE name LIKE '%s%%' ORDER BY name",
		EscapeLIKE(remotePrefix),
	))
	if err != nil {
		return nil
	}
	var remoteBranches []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n")[1:] {
		name := strings.TrimSpace(line)
		if name != "" {
			remoteBranches = append(remoteBranches, name)
		}
	}
	if len(remoteBranches) == 0 {
		return nil
	}
	localBranches, _ := ListBranches(dbDir, prefix)
	localSet := make(map[string]bool, len(localBranches))
	for _, b := range localBranches {
		localSet[b] = true
	}
	for _, remote := range remoteBranches {
		local := strings.TrimPrefix(remote, "remotes/origin/")
		if localSet[local] {
			continue
		}
		cmd := exec.Command("dolt", "branch", local, remote)
		cmd.Dir = dbDir
		_ = cmd.Run()
	}
	remoteSet := make(map[string]bool, len(remoteBranches))
	for _, r := range remoteBranches {
		remoteSet[strings.TrimPrefix(r, "remotes/origin/")] = true
	}
	for _, local := range localBranches {
		if !strings.HasPrefix(local, prefix) {
			continue
		}
		if !remoteSet[local] {
			_ = DeleteBranch(dbDir, local)
		}
	}
	return nil
}

// MergeBranch merges a branch into main. If the merge produces conflicts it aborts and returns an error.
func MergeBranch(dbDir, branch string) error {
	escaped := strings.ReplaceAll(branch, "'", "''")
	err := DoltSQLScript(dbDir, fmt.Sprintf(
		"CALL DOLT_CHECKOUT('main');\nCALL DOLT_MERGE('%s');", escaped,
	))
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "conflict") {
			_ = DoltSQLScript(dbDir, "CALL DOLT_MERGE('--abort');")
			return fmt.Errorf("merge conflict on branch %s: resolve manually or delete the branch", branch)
		}
		return fmt.Errorf("merging branch %s: %w", branch, err)
	}
	return nil
}

// DeleteBranch deletes a local branch.
func DeleteBranch(dbDir, branch string) error {
	escaped := strings.ReplaceAll(branch, "'", "''")
	return DoltSQLScript(dbDir, fmt.Sprintf("CALL DOLT_BRANCH('-D', '%s');", escaped))
}

// DeleteRemoteBranch deletes a branch on a named remote using refspec syntax.
func DeleteRemoteBranch(dbDir, remote, branch string) error {
	return doltRetry(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "dolt", "push", remote, ":"+branch)
		cmd.Dir = dbDir
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("dolt push %s :%s: %w (%s)", remote, branch, err, strings.TrimSpace(string(output)))
		}
		return nil
	})
}

// EnsureGitHubRemote adds a "github" Dolt remote pointing to the rig's GitHub fork.
func EnsureGitHubRemote(dbDir, forkOrg, forkDB string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	checkCmd := exec.CommandContext(ctx, "dolt", "remote", "-v")
	checkCmd.Dir = dbDir
	output, err := checkCmd.CombinedOutput()
	if err == nil {
		for _, line := range strings.Split(string(output), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "github") {
				return nil
			}
		}
	}
	remoteURL := fmt.Sprintf("https://github.com/%s/%s.git", forkOrg, forkDB)
	addCmd := exec.CommandContext(ctx, "dolt", "remote", "add", "github", remoteURL)
	addCmd.Dir = dbDir
	output, err = addCmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(output))
		if strings.Contains(strings.ToLower(msg), "already exists") {
			return nil
		}
		return fmt.Errorf("dolt remote add github: %w (%s)", err, msg)
	}
	return nil
}

// PushBranchToRemote pushes a branch to a named remote.
func PushBranchToRemote(dbDir, remote, branch string, stdout io.Writer) error {
	return PushBranchToRemoteForce(dbDir, remote, branch, false, stdout)
}

// PushBranchToRemoteForce pushes a branch to a named remote, optionally with --force.
func PushBranchToRemoteForce(dbDir, remote, branch string, force bool, stdout io.Writer) error {
	err := doltRetry(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		args := []string{"push"}
		if force {
			args = append(args, "--force")
		}
		args = append(args, remote, branch)
		cmd := exec.CommandContext(ctx, "dolt", args...)
		cmd.Dir = dbDir
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("dolt push %s %s: %w (%s)", remote, branch, err, strings.TrimSpace(string(output)))
		}
		return nil
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "  Pushed branch %s to %s\n", branch, remote)
	return nil
}

// DoltSQLQuery executes a SQL query and returns the raw CSV output.
func DoltSQLQuery(dbDir, query string) (string, error) {
	return DoltSQLQueryContext(context.Background(), dbDir, query)
}

// DoltSQLQueryContext executes a SQL query and returns the raw CSV output, binding the dolt subprocess lifetime to ctx.
func DoltSQLQueryContext(ctx context.Context, dbDir, query string) (string, error) {
	var result string
	err := doltRetryContext(ctx, func(ctx context.Context) error {
		queryCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		cmd := exec.CommandContext(queryCtx, "dolt", "sql", "-r", "csv", "-q", query)
		cmd.Dir = dbDir
		output, err := cmd.CombinedOutput()
		if err != nil {
			if queryCtx.Err() != nil {
				return queryCtx.Err()
			}
			return fmt.Errorf("dolt sql query failed: %w (%s)", err, strings.TrimSpace(string(output)))
		}
		result = string(output)
		return nil
	})
	return result, err
}
