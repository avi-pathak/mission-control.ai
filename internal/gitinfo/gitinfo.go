// Package gitinfo extracts repository state using the git CLI.
package gitinfo

import (
	"context"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/avi-pathak/mission-control.ai/internal/protocol"
)

func run(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// Collect gathers git information for a working directory. Returns nil if the
// directory is not inside a git repository.
func Collect(ctx context.Context, dir string) *protocol.GitInfo {
	top, err := run(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil || top == "" {
		return nil
	}
	info := &protocol.GitInfo{Repo: filepath.Base(top)}

	if b, err := run(ctx, dir, "rev-parse", "--abbrev-ref", "HEAD"); err == nil {
		info.Branch = b
	}
	if u, err := run(ctx, dir, "config", "--get", "remote.origin.url"); err == nil {
		info.RemoteURL = u
	}
	if status, err := run(ctx, dir, "status", "--porcelain"); err == nil && status != "" {
		info.Dirty = true
		for _, line := range strings.Split(status, "\n") {
			if len(line) > 3 {
				info.ModifiedFiles = append(info.ModifiedFiles, strings.TrimSpace(line[3:]))
			}
		}
	}
	// ahead/behind vs upstream
	if ab, err := run(ctx, dir, "rev-list", "--left-right", "--count", "@{upstream}...HEAD"); err == nil {
		parts := strings.Fields(ab)
		if len(parts) == 2 {
			info.Behind, _ = strconv.Atoi(parts[0])
			info.Ahead, _ = strconv.Atoi(parts[1])
		}
	}
	info.RecentCommits = recentCommits(ctx, dir, 10)
	return info
}

func recentCommits(ctx context.Context, dir string, n int) []protocol.Commit {
	// format: hash<US>author<US>unixts<US>subject
	out, err := run(ctx, dir, "log", "-n", strconv.Itoa(n), "--pretty=format:%H%x1f%an%x1f%at%x1f%s")
	if err != nil || out == "" {
		return nil
	}
	var commits []protocol.Commit
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(line, "\x1f")
		if len(f) != 4 {
			continue
		}
		ts, _ := strconv.ParseInt(f[2], 10, 64)
		commits = append(commits, protocol.Commit{
			Hash:    f[0],
			Author:  f[1],
			Message: f[3],
			TS:      time.Unix(ts, 0).UnixMilli(),
		})
	}
	return commits
}
