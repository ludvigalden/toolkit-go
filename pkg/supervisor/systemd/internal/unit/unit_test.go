package unit

import (
	"strings"
	"testing"

	"github.com/ludvigalden/toolkit-go/pkg/supervisor"
)

const wantDefault = `[Unit]
Description=demo supervised service (port 8080)
After=network.target
StartLimitIntervalSec=120
StartLimitBurst=10

[Service]
Type=exec
Environment=HOME=/home/u
Environment=PATH=/opt/bin:/usr/bin
ExecStart=/opt/bin/demo serve --port 8080
WorkingDirectory=/srv/demo
TimeoutStopSec=35
Restart=always
RestartSec=2
RestartSteps=5
RestartMaxDelaySec=60
OOMScoreAdjust=-500
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=default.target
`

func spec() supervisor.Spec {
	return supervisor.Spec{
		Namespace: "com.example", UnitName: "demo:8080", ExePath: "/opt/bin/demo",
		Args: []string{"serve", "--port", "8080"}, Port: 8080, WorkingDirectory: "/srv/demo",
	}
}

func TestRender(t *testing.T) {
	env := map[string]string{"PATH": "/opt/bin:/usr/bin", "HOME": "/home/u"}
	got, err := Render(spec(), env)
	if err != nil {
		t.Fatal(err)
	}
	if got != wantDefault {
		t.Fatalf("render mismatch:\n%s\nwant:\n%s", got, wantDefault)
	}
}

func TestRenderVariants(t *testing.T) {
	s := spec()
	oom := 1000
	s.OOMScoreAdjust = &oom
	s.StderrLogPath = "/var/log/demo.log"
	got, err := Render(s, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{
		"OOMScoreAdjust=1000\n",
		"StandardOutput=append:/var/log/demo.log\n",
		"StandardError=append:/var/log/demo.log\n",
	} {
		if !contains(got, line) {
			t.Errorf("missing %q", line)
		}
	}
	if contains(got, "Environment=") {
		t.Error("nil env rendered Environment lines")
	}
}

func TestRenderRejects(t *testing.T) {
	s := spec()
	s.Args = []string{"a\nExecStartPre=/bin/evil"}
	if _, err := Render(s, nil); err == nil {
		t.Error("newline in args accepted")
	}
	if _, err := Render(spec(), map[string]string{"K": "v\n"}); err == nil {
		t.Error("newline in env accepted")
	}
	s = spec()
	s.UnitName = ""
	if _, err := Render(s, nil); err == nil {
		t.Error("empty unit name accepted")
	}
}

func TestRenderEscapesSpecialValues(t *testing.T) {
	s := spec()
	s.ExePath = "/opt/my tool/demo"
	s.Args = []string{"--label", `he said "hi" $USER %p`, "--plain"}
	env := map[string]string{"HOME": "/Users/John Doe", "FMT": "50%n$X"}
	got, err := Render(s, env)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{
		`ExecStart="/opt/my tool/demo" --label "he said \"hi\" $$USER %%p" --plain`,
		`Environment="HOME=/Users/John Doe"`,
		`Environment=FMT=50%%n$X`,
	} {
		if !contains(got, line+"\n") {
			t.Errorf("missing %q in:\n%s", line, got)
		}
	}
	// Path settings take the whole line value: quoting is kept literal
	// there, so these must stay unquoted.
	s.WorkingDirectory = "/srv/my data"
	s.StderrLogPath = "/var/log/my data.log"
	got, err = Render(s, env)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{
		"WorkingDirectory=/srv/my data\n",
		`StandardOutput=append:/var/log/my data.log`,
	} {
		if !contains(got, line) {
			t.Errorf("missing %q in:\n%s", line, got)
		}
	}
}

func TestStartFailure(t *testing.T) {
	for _, tc := range []struct {
		props map[string]string
		fatal bool
	}{
		{map[string]string{"ActiveState": "activating", "SubState": "start"}, false},
		{map[string]string{"ActiveState": "activating", "SubState": "auto-restart", "ExecMainStatus": "0"}, false},
		{map[string]string{"ActiveState": "activating", "SubState": "auto-restart", "ExecMainStatus": "2"}, true},
		{map[string]string{"ActiveState": "failed"}, true},
		{map[string]string{"Result": "start-limit-hit"}, true},
		{map[string]string{"ActiveState": "inactive"}, true},
	} {
		if got := StartFailure(tc.props) != ""; got != tc.fatal {
			t.Errorf("StartFailure(%v) fatal = %v, want %v", tc.props, got, tc.fatal)
		}
	}
}

func TestParseShow(t *testing.T) {
	m := ParseShow("ActiveState=active\nSubState=running\nnoise\nExecMainPID=42\n")
	if m["ActiveState"] != "active" || m["SubState"] != "running" || m["ExecMainPID"] != "42" || len(m) != 3 {
		t.Fatalf("ParseShow = %v", m)
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }
