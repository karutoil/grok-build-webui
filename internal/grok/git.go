package grok

import (
	"os"
	"os/exec"
	"strings"
)

type GitInfo struct {
	Branch string `json:"branch"`
	Dirty  bool   `json:"dirty"`
	OK     bool   `json:"ok"`
}

func GitStatus(dir string) GitInfo {
	info := GitInfo{}
	branch, err := runGit(dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return info
	}
	info.OK = true
	info.Branch = strings.TrimSpace(branch)
	out, err := runGit(dir, "status", "--porcelain")
	if err == nil {
		info.Dirty = strings.TrimSpace(out) != ""
	}
	return info
}

func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	path := os.Getenv("PATH")
	if path == "" {
		path = "/usr/bin:/bin:/usr/local/bin"
	}
	cmd.Env = []string{"PATH=" + path, "GIT_OPTIONAL_LOCKS=0"}
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
