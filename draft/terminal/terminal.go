// Package terminal adds the interactive Mode renderer: a
// full-screen log view with a progress line, drawn on stderr. Import it
// only where the mode is offered; it carries the Charm dependencies.
package terminal

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"charm.land/bubbles/v2/progress"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/ludvigalden/toolkit-go/pkg/output"
)

// Option registers the terminal renderer with output.New.
// Mode selects the interactive renderer (--stdout-format=terminal).
const Mode output.Mode = "terminal"

func Option() output.Option { return output.WithRenderer(Mode, New) }

// New is the Mode Factory. When stderr is not a terminal it
// returns the plain text renderer, so piped runs stay readable.
func New(stdout, stderr io.Writer, args output.Args) (output.Renderer, error) {
	if !output.IsTerminal(stderr) {
		return output.TextRenderer(stdout, stderr, args)
	}
	args.Color = true
	return &renderer{stdout: stdout, stderr: stderr, args: args, ready: make(chan struct{}), done: make(chan struct{})}, nil
}

// readyTimeout bounds the wait for the first frame on terminals that
// never report a size.
const readyTimeout = 500 * time.Millisecond

// maxVisible bounds the in-memory log view; the full history is
// replayed to stderr after the screen is torn down.
const maxVisible = 100

type renderer struct {
	stdout, stderr io.Writer
	args           output.Args

	program   *tea.Program
	startOnce sync.Once
	readyOnce sync.Once
	ready     chan struct{}
	done      chan struct{}
	history   []string
}

func (r *renderer) start() {
	r.program = tea.NewProgram(newModel(r.markReady), tea.WithOutput(r.stderr))
	time.AfterFunc(readyTimeout, r.markReady)
	go func() {
		_, _ = r.program.Run() //nolint:errcheck // teardown is driven by Close
		close(r.done)
	}()
}

func (r *renderer) markReady() { r.readyOnce.Do(func() { close(r.ready) }) }

func (r *renderer) send(msg tea.Msg) {
	r.startOnce.Do(r.start)
	<-r.ready
	r.program.Send(msg)
}

func (r *renderer) Log(rec output.Record) {
	if rec.Kind == output.KindProgress {
		r.send(progressMsg(rec.Message))
		return
	}
	line := plainLine(rec)
	r.history = append(r.history, line)
	r.send(rec)
}

// Result bypasses the screen: stdout carries the payload as in text
// mode, independent of the interactive view.
func (r *renderer) Result(data any) {
	text, _ := output.TextRenderer(r.stdout, io.Discard, output.Args{Mode: output.ModeText, Verbosity: r.args.Verbosity})
	text.Result(data)
}

// Close tears the screen down, then replays the log on stderr so it
// survives in the scrollback.
func (r *renderer) Close(error) error {
	if r.program != nil {
		<-r.ready
		r.program.Send(quitMsg{})
		<-r.done
	}
	for _, l := range r.history {
		_, _ = fmt.Fprintln(r.stderr, l)
	}
	return nil
}

func plainLine(rec output.Record) string {
	body := rec.Message
	if rec.Code != "" {
		body = "[" + rec.Code + "] " + body
	}
	return rec.Time.Format("15:04:05") + " " + label(rec) + " " + body
}

func label(rec output.Record) string {
	switch rec.Kind {
	case output.KindDebug:
		return "DBUG"
	case output.KindSuccess:
		return "OK"
	case output.KindWarn:
		return "WARN"
	case output.KindError:
		if rec.Err != nil {
			return "FAIL"
		}
		return "ERR"
	}
	return "INFO"
}

type (
	progressMsg string
	quitMsg     struct{}
)

var (
	styleTime  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleInfo  = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	styleWarn  = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	styleError = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	styleOK    = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
)

func style(k output.Kind) lipgloss.Style {
	switch k {
	case output.KindWarn:
		return styleWarn
	case output.KindError:
		return styleError
	case output.KindSuccess:
		return styleOK
	}
	return styleInfo
}

type model struct {
	ready    func()
	log      []output.Record
	bar      progress.Model
	status   string
	height   int
	quitting bool
}

func newModel(ready func()) model {
	return model{ready: ready, bar: progress.New(progress.WithDefaultBlend())}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height
		m.bar.SetWidth(max(msg.Width-30, 10))
		m.ready()
	case progress.FrameMsg:
		var cmd tea.Cmd
		m.bar, cmd = m.bar.Update(msg)
		return m, cmd
	case progressMsg:
		m.status = string(msg)
		return m, m.bar.SetPercent(percentOf(m.status))
	case output.Record:
		m.log = append(m.log, msg)
		if len(m.log) > maxVisible {
			m.log = m.log[len(m.log)-maxVisible:]
		}
	case quitMsg:
		m.quitting = true
		return m, tea.Quit
	}
	return m, nil
}

func (m model) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}
	rows := max(m.height-1, 1)
	entries := m.log
	if len(entries) > rows {
		entries = entries[len(entries)-rows:]
	}
	var b strings.Builder
	for _, e := range entries {
		b.WriteString(styleTime.Render(e.Time.Format("15:04:05")) + " " + style(e.Kind).Render(label(e)) + " " + e.Message + "\n")
	}
	b.WriteString(strings.Repeat("\n", rows-len(entries)))
	if m.status != "" {
		b.WriteString(m.bar.View() + "  " + m.status)
	}
	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}

// percentOf reads an "NN%" token from a progress message; without one
// the bar sits at half to show indeterminate work.
func percentOf(s string) float64 {
	i := strings.IndexByte(s, '%')
	if i <= 0 {
		return 0.5
	}
	j := i
	for j > 0 && (s[j-1] >= '0' && s[j-1] <= '9' || s[j-1] == '.') {
		j--
	}
	var p float64
	if _, err := fmt.Sscanf(s[j:i], "%g", &p); err != nil {
		return 0.5
	}
	return min(max(p, 0), 100) / 100
}
