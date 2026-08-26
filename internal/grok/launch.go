package grok

import "strings"

type Launch struct {
	Mode           string // new, continue, resume
	ResumeID       string
	Model          string
	PermissionMode string
	Sandbox        string
	Yolo           bool
}

func NormalizeMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "continue", "c":
		return "continue"
	case "resume", "r":
		return "resume"
	default:
		return "new"
	}
}

func BuildArgs(l Launch) []string {
	l.Mode = NormalizeMode(l.Mode)
	var args []string
	switch l.Mode {
	case "continue":
		args = append(args, "-c")
	case "resume":
		if strings.TrimSpace(l.ResumeID) != "" {
			args = append(args, "-r", strings.TrimSpace(l.ResumeID))
		} else {
			args = append(args, "-c")
		}
	}
	if m := strings.TrimSpace(l.Model); m != "" {
		args = append(args, "-m", m)
	}
	if l.Yolo {
		args = append(args, "--always-approve")
	} else if p := strings.TrimSpace(l.PermissionMode); p != "" && p != "ask" && p != "default" {
		args = append(args, "--permission-mode", p)
	}
	if s := strings.TrimSpace(l.Sandbox); s != "" && s != "default" {
		args = append(args, "--sandbox", s)
	}
	return args
}
