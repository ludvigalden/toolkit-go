package plist

import "testing"

const want = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>com.example.demo.8080</string>
	<key>ProgramArguments</key>
	<array>
		<string>/opt/bin/demo</string>
		<string>serve</string>
		<string>--flag=a&amp;b</string>
	</array>
	<key>EnvironmentVariables</key>
	<dict>
		<key>HOME</key>
		<string>/Users/u</string>
		<key>PATH</key>
		<string>/opt/bin:/usr/bin</string>
	</dict>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>WorkingDirectory</key>
	<string>/srv/demo</string>
	<key>StandardOutPath</key>
	<string>/logs/demo.log</string>
	<key>StandardErrorPath</key>
	<string>/logs/demo.log</string>
</dict>
</plist>
`

func TestRender(t *testing.T) {
	got, err := Render(Agent{
		Label:            "com.example.demo.8080",
		Program:          []string{"/opt/bin/demo", "serve", "--flag=a&b"},
		Env:              map[string]string{"PATH": "/opt/bin:/usr/bin", "HOME": "/Users/u"},
		WorkingDirectory: "/srv/demo",
		LogPath:          "/logs/demo.log",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("render mismatch:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderRequiresLabelAndProgram(t *testing.T) {
	if _, err := Render(Agent{Program: []string{"/x"}}); err == nil {
		t.Error("missing label accepted")
	}
	if _, err := Render(Agent{Label: "l"}); err == nil {
		t.Error("missing program accepted")
	}
}

func TestParsePrint(t *testing.T) {
	out := "gui/501/com.example.demo.8080 = {\n\tstate = running\n\tpid = 731\n}\n"
	if State(out) != "running" || PID(out) != 731 {
		t.Fatalf("State=%q PID=%d", State(out), PID(out))
	}
	if State("") != "" || PID("pid = x") != 0 {
		t.Fatal("absent fields not empty")
	}
}
