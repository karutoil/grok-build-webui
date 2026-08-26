package grok

import (
	"reflect"
	"testing"
)

func TestBuildArgs(t *testing.T) {
	cases := []struct {
		in   Launch
		want []string
	}{
		{Launch{Mode: "new"}, nil},
		{Launch{Mode: "continue"}, []string{"-c"}},
		{Launch{Mode: "resume", ResumeID: "abc"}, []string{"-r", "abc"}},
		{Launch{Mode: "resume"}, []string{"-c"}},
		{Launch{Mode: "new", Model: "grok-build", Yolo: true, Sandbox: "off"}, []string{"-m", "grok-build", "--always-approve", "--sandbox", "off"}},
		{Launch{Mode: "new", PermissionMode: "auto"}, []string{"--permission-mode", "auto"}},
		{Launch{Mode: "new", PermissionMode: "ask"}, nil},
	}
	for _, c := range cases {
		got := BuildArgs(c.in)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("BuildArgs(%+v)=%v want %v", c.in, got, c.want)
		}
	}
}
