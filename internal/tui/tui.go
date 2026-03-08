// Package tui provides a Bubble Tea–based terminal UI for the video converter.
//
// Layout (adapts to terminal width):
//
//	╭─ Video Converter ────────────────────────── ⏱ 00:02:34 ─╮
//	│                                                           │
//	│  [3/12] myvideo_long.mkv              1080p              │
//	│  ████████████████░░░░░░░░   64%  elapsed 0:45  ETA ~25s  │
//	│                                                           │
//	│  [4/12] another_film.mp4               720p              │
//	│  ██████░░░░░░░░░░░░░░░░░░   24%  elapsed 1:12  ETA ~3m   │
//	│                                                           │
//	├─ Log ─────────────────────────────────────────────────── ┤
//	│  ✓ KEPT    film_a.mkv   234.5 MB → 156.2 MB  (-33.3%)   │
//	│  ✗ DISCARD film_b.mp4   890.1 MB → 912.3 MB  (+2.5%)    │
//	╰─────────────────────────────────────────────────────────╯
//
// External goroutines (FFmpeg workers) drive the UI by calling methods on [UI].
// Those methods are thread-safe and send messages to the Bubble Tea program via
// [tea.Program.Send].
package tui

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

// ── Styles ────────────────────────────────────────────────────────────────────

var (
	colorAccent  = lipgloss.Color("#7C3AED") // purple
	colorOK      = lipgloss.Color("#22C55E") // green
	colorWarn    = lipgloss.Color("#F59E0B") // amber
	colorErr     = lipgloss.Color("#EF4444") // red
	colorMuted   = lipgloss.Color("#6B7280") // gray
	colorFill    = lipgloss.Color("#7C3AED")
	colorEmpty   = lipgloss.Color("#374151")
	colorBorder  = lipgloss.Color("#4B5563")
	colorTimer   = lipgloss.Color("#A78BFA")
	colorHeading = lipgloss.Color("#E5E7EB")

	styleBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder)

	styleTitle = lipgloss.NewStyle().
			Foreground(colorAccent).
			Bold(true)

	styleTimer = lipgloss.NewStyle().
			Foreground(colorTimer).
			Bold(true)

	styleFileName = lipgloss.NewStyle().
			Foreground(colorHeading).
			Bold(true)

	styleMeta = lipgloss.NewStyle().
			Foreground(colorMuted)

	stylePct = lipgloss.NewStyle().
			Foreground(colorAccent).
			Width(5).
			Align(lipgloss.Right)

	styleElapsed = lipgloss.NewStyle().
			Foreground(colorMuted)

	styleETA = lipgloss.NewStyle().
			Foreground(colorMuted)

	styleLogOK = lipgloss.NewStyle().
			Foreground(colorOK)

	styleLogDiscard = lipgloss.NewStyle().
			Foreground(colorWarn)

	styleLogErr = lipgloss.NewStyle().
			Foreground(colorErr)

	styleLogInfo = lipgloss.NewStyle().
			Foreground(colorMuted)

	styleSectionLabel = lipgloss.NewStyle().
				Foreground(colorMuted).
				Bold(false)

	styleMuted = lipgloss.NewStyle().
			Foreground(colorMuted)
)

// ── Message types ─────────────────────────────────────────────────────────────

type msgJobStart struct {
	id           uint64
	label        string
	fileNum      int
	totalFiles   int
	heightP      int // video height in pixels (for resolution label)
	totalSeconds float64
}

type msgJobProgress struct {
	id  uint64
	pct int
}

type msgJobComplete struct {
	id      uint64
	summary string
	kind    completionKind
}

type msgLog struct{ text string }

type msgStartTimer struct{}

type msgTick time.Time

type completionKind int

const (
	kindKept completionKind = iota
	kindDiscard
	kindError
	kindSkipped
)

// tickCmd fires a repaint message every repaintInterval.
const repaintInterval = 250 * time.Millisecond

func tickCmd() tea.Cmd {
	return tea.Tick(repaintInterval, func(t time.Time) tea.Msg {
		return msgTick(t)
	})
}

// ── Job state ─────────────────────────────────────────────────────────────────

type jobState struct {
	id           uint64
	label        string
	fileNum      int
	totalFiles   int
	heightP      int
	pct          int
	startedAt    time.Time
	totalSeconds float64
}

func (j *jobState) elapsed() time.Duration {
	return time.Since(j.startedAt).Round(time.Second)
}

func (j *jobState) eta() string {
	if j.pct <= 0 {
		return "–"
	}
	elapsed := time.Since(j.startedAt)
	remaining := time.Duration(float64(elapsed) * float64(100-j.pct) / float64(j.pct))
	return "~" + fmtDuration(remaining.Round(time.Second))
}

// ── Model ─────────────────────────────────────────────────────────────────────

const maxLogLines = 200

type model struct {
	width       int
	height      int
	jobs        []*jobState          // active jobs, in insertion order
	jobIndex    map[uint64]*jobState // fast lookup
	logLines    []logLine
	vp          viewport.Model
	vpReady     bool
	timerStart  time.Time
	timerActive bool
}

type logLine struct {
	text string
	kind completionKind
}

func initialModel() model {
	return model{
		width:    80,
		height:   24,
		jobIndex: make(map[uint64]*jobState),
	}
}

func (m model) Init() tea.Cmd {
	return tickCmd()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.vpReady {
			m.vp.Width = m.logViewportWidth()
			m.vp.Height = m.logViewportHeight()
			m.vp.SetContent(m.renderLogContent())
		}
		return m, nil

	case msgTick:
		// Rerender on each tick so elapsed/ETA update live.
		// Also keep viewport content fresh.
		if m.vpReady {
			m.vp.SetContent(m.renderLogContent())
			m.vp.GotoBottom()
		}
		return m, tickCmd()

	case msgStartTimer:
		m.timerStart = time.Now()
		m.timerActive = true
		return m, nil

	case msgJobStart:
		js := &jobState{
			id:           msg.id,
			label:        msg.label,
			fileNum:      msg.fileNum,
			totalFiles:   msg.totalFiles,
			heightP:      msg.heightP,
			startedAt:    time.Now(),
			totalSeconds: msg.totalSeconds,
		}
		m.jobs = append(m.jobs, js)
		m.jobIndex[msg.id] = js
		return m, nil

	case msgJobProgress:
		if js, ok := m.jobIndex[msg.id]; ok {
			js.pct = msg.pct
		}
		return m, nil

	case msgJobComplete:
		// Remove from active jobs
		if js, ok := m.jobIndex[msg.id]; ok {
			delete(m.jobIndex, msg.id)
			for i, j := range m.jobs {
				if j == js {
					m.jobs = append(m.jobs[:i], m.jobs[i+1:]...)
					break
				}
			}
		}
		// Append to log
		m.logLines = append(m.logLines, logLine{text: msg.summary, kind: msg.kind})
		if len(m.logLines) > maxLogLines {
			m.logLines = m.logLines[len(m.logLines)-maxLogLines:]
		}
		if m.vpReady {
			m.vp.SetContent(m.renderLogContent())
			m.vp.GotoBottom()
		}
		return m, nil

	case msgLog:
		m.logLines = append(m.logLines, logLine{text: msg.text, kind: kindSkipped})
		if len(m.logLines) > maxLogLines {
			m.logLines = m.logLines[len(m.logLines)-maxLogLines:]
		}
		if m.vpReady {
			m.vp.SetContent(m.renderLogContent())
			m.vp.GotoBottom()
		}
		return m, nil
	}

	// Forward to viewport
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

// ── View ──────────────────────────────────────────────────────────────────────

func (m model) View() string {
	inner := m.renderInner()
	// Wrap in rounded border, respecting terminal width
	available := m.width - 2 // 2 for border
	if available < 20 {
		available = 20
	}
	return styleBorder.Width(available).Render(inner) + "\n"
}

func (m model) renderInner() string {
	w := m.innerWidth()
	var b strings.Builder

	// ── Title bar ──
	title := styleTitle.Render("Video Converter")
	var timerStr string
	if m.timerActive {
		elapsed := time.Since(m.timerStart).Round(time.Second)
		timerStr = styleTimer.Render("⏱ " + fmtDuration(elapsed))
	}
	titleLine := padBetween(title, timerStr, w)
	b.WriteString(titleLine + "\n")

	// ── Active jobs ──
	if len(m.jobs) > 0 {
		b.WriteString("\n")
		for _, js := range m.jobs {
			b.WriteString(m.renderJob(js, w))
		}
	}

	// ── Log section ──
	sep := m.renderSectionSep("Log", w)
	b.WriteString(sep + "\n")

	// Viewport for scrolling log
	if !m.vpReady {
		// First time: initialize viewport
		// (can't mutate m here; do it next tick after size is known)
		// For now render log lines directly
		for _, ll := range m.lastLogLines(m.logHeight()) {
			b.WriteString(m.renderLogLine(ll, w) + "\n")
		}
	} else {
		b.WriteString(m.vp.View() + "\n")
	}

	return b.String()
}

func (m model) renderJob(js *jobState, w int) string {
	var b strings.Builder

	// Row 1: filename + resolution
	label := truncate(js.label, w-12)
	res := ""
	if js.heightP > 0 {
		res = styleMeta.Render(fmt.Sprintf("%dp", js.heightP))
	}
	counter := styleMeta.Render(fmt.Sprintf("[%d/%d]", js.fileNum, js.totalFiles))
	nameLine := counter + " " + styleFileName.Render(label)
	row1 := padBetween(nameLine, res, w)
	b.WriteString(row1 + "\n")

	// Row 2: progress bar + pct + elapsed + eta
	barWidth := w - 30 // reserve for pct/elapsed/eta
	if barWidth < 10 {
		barWidth = 10
	}
	bar := renderBar(js.pct, barWidth)
	pctStr := stylePct.Render(fmt.Sprintf("%d%%", js.pct))
	elapsedStr := styleElapsed.Render("elapsed " + fmtDuration(js.elapsed()))
	etaStr := styleETA.Render("ETA " + js.eta())
	right := pctStr + "  " + elapsedStr + "  " + etaStr
	row2 := bar + "  " + right
	// Clamp to width
	row2 = truncateANSI(row2, w)
	b.WriteString(row2 + "\n\n")

	return b.String()
}

func (m model) renderLogLine(ll logLine, w int) string {
	text := truncate(ll.text, w)
	switch ll.kind {
	case kindKept:
		return styleLogOK.Render(text)
	case kindDiscard:
		return styleLogDiscard.Render(text)
	case kindError:
		return styleLogErr.Render(text)
	default:
		return styleLogInfo.Render(text)
	}
}

func (m model) renderLogContent() string {
	w := m.innerWidth()
	var lines []string
	for _, ll := range m.logLines {
		lines = append(lines, m.renderLogLine(ll, w))
	}
	return strings.Join(lines, "\n")
}

func (m model) renderSectionSep(label string, w int) string {
	lbl := styleSectionLabel.Render(" " + label + " ")
	lblWidth := lipgloss.Width(lbl)
	lineWidth := w - lblWidth - 1
	if lineWidth < 0 {
		lineWidth = 0
	}
	return styleMuted.Render("─") + lbl + styleMuted.Render(strings.Repeat("─", lineWidth))
}

// innerWidth is the usable width inside the border.
func (m model) innerWidth() int {
	w := m.width - 4 // 2 border + 2 padding
	if w < 20 {
		w = 20
	}
	return w
}

func (m model) logHeight() int {
	// Active jobs each take 3 lines (name + bar + blank), plus title (1) + sep (1)
	used := 2 + len(m.jobs)*3
	h := m.height - 4 - used // 4 for border/padding
	if h < 3 {
		h = 3
	}
	return h
}

func (m model) logViewportWidth() int  { return m.innerWidth() }
func (m model) logViewportHeight() int { return m.logHeight() }

func (m model) lastLogLines(n int) []logLine {
	if len(m.logLines) <= n {
		return m.logLines
	}
	return m.logLines[len(m.logLines)-n:]
}

// renderBar renders a Unicode block progress bar of the given width.
func renderBar(pct, width int) string {
	if width <= 0 {
		return ""
	}
	filled := width * pct / 100
	if filled > width {
		filled = width
	}
	fill := lipgloss.NewStyle().Foreground(colorFill).Render(strings.Repeat("█", filled))
	empty := lipgloss.NewStyle().Foreground(colorEmpty).Render(strings.Repeat("░", width-filled))
	return fill + empty
}

// padBetween returns a string of exactly width runes with left on the left and
// right on the right, filled with spaces in between.
func padBetween(left, right string, width int) string {
	lw := lipgloss.Width(left)
	rw := lipgloss.Width(right)
	gap := width - lw - rw
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

func fmtDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// truncate clips s to maxLen visible characters (no ANSI awareness needed
// here since we apply styles after truncation).
func truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-3]) + "..."
}

// truncateANSI is a best-effort truncation of an ANSI-styled string.
// We pass it through as-is; the bar renderer ensures the bar width is already
// bounded, so lines won't exceed the terminal width in practice.
func truncateANSI(s string, _ int) string {
	return s
}

// ── UI (thread-safe façade) ───────────────────────────────────────────────────

// UI wraps a Bubble Tea program and exposes a thread-safe API for worker
// goroutines to drive the display.
type UI struct {
	prog       *tea.Program
	nextID     atomic.Uint64
	mu         sync.Mutex
	logW       *tuiWriter
	timerStart time.Time
	// plain mode: used when stdout is not a TTY
	plain  bool
	plainW io.Writer
}

// New creates and starts a Bubble Tea UI writing to stdout.
// When stdout is not a TTY (e.g. redirected to a file), the TUI is skipped
// and a plain-text fallback is used instead.
// Call [UI.Wait] (or defer it) to block until the program exits.
func New() *UI {
	// If stdout is not a terminal, skip Bubble Tea entirely — it requires a TTY.
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return &UI{plain: true, plainW: os.Stdout, logW: &tuiWriter{plain: true, plainW: os.Stdout}}
	}

	m := initialModel()
	prog := tea.NewProgram(m,
		tea.WithAltScreen(),       // use alternate screen buffer
		tea.WithMouseCellMotion(), // optional: allow scroll
	)
	w := &tuiWriter{prog: prog}
	u := &UI{prog: prog, logW: w}
	go func() { prog.Run() }() //nolint:errcheck
	// Give bubbletea a moment to start and claim the screen
	time.Sleep(50 * time.Millisecond)
	return u
}

// Wait shuts down the Bubble Tea program and waits for it to finish.
// Call this before the process exits (typically via defer).
func (u *UI) Wait() {
	if u.plain {
		return
	}
	u.prog.Quit()
	// small grace period so the final frame is flushed
	time.Sleep(100 * time.Millisecond)
}

// StartTimer signals that work has begun — starts the global elapsed timer.
func (u *UI) StartTimer() {
	u.timerStart = time.Now()
	if u.plain {
		return
	}
	u.prog.Send(msgStartTimer{})
}

// Elapsed returns the wall-clock duration since StartTimer was called.
// Returns 0 if StartTimer was never called.
func (u *UI) Elapsed() time.Duration {
	if u.timerStart.IsZero() {
		return 0
	}
	return time.Since(u.timerStart).Round(time.Second)
}

// StartJob registers a new conversion job and returns an opaque job ID.
// fileNum/totalFiles are used for the "[N/M]" counter.
// heightP is the video vertical resolution (0 to omit).
// totalSeconds is the video duration (used for ETA; 0 = unknown).
func (u *UI) StartJob(label string, fileNum, totalFiles, heightP int, totalSeconds float64) uint64 {
	id := u.nextID.Add(1)
	if u.plain {
		fmt.Fprintf(u.plainW, "[%d/%d] Converting: %s (%dp)\n", fileNum, totalFiles, label, heightP)
		return id
	}
	u.prog.Send(msgJobStart{
		id:           id,
		label:        label,
		fileNum:      fileNum,
		totalFiles:   totalFiles,
		heightP:      heightP,
		totalSeconds: totalSeconds,
	})
	return id
}

// UpdateProgress updates the progress for a running job (0–100).
// Thread-safe; can be called from any goroutine.
func (u *UI) UpdateProgress(id uint64, pct int) {
	if u.plain {
		return // no progress output in plain mode
	}
	u.prog.Send(msgJobProgress{id: id, pct: pct})
}

// CompleteJob removes the active bar and appends a completion summary line.
func (u *UI) CompleteJob(id uint64, summary string, kind completionKind) {
	if u.plain {
		fmt.Fprintln(u.plainW, summary)
		return
	}
	u.prog.Send(msgJobComplete{id: id, summary: summary, kind: kind})
}

// CompleteKept is a helper for kept files.
func (u *UI) CompleteKept(id uint64, summary string) {
	u.CompleteJob(id, summary, kindKept)
}

// CompleteDiscard is a helper for discarded files.
func (u *UI) CompleteDiscard(id uint64, summary string) {
	u.CompleteJob(id, summary, kindDiscard)
}

// CompleteError is a helper for errored files.
func (u *UI) CompleteError(id uint64, summary string) {
	u.CompleteJob(id, summary, kindError)
}

// Log appends an informational line to the log area.
func (u *UI) Log(text string) {
	if u.plain {
		fmt.Fprintln(u.plainW, text)
		return
	}
	u.prog.Send(msgLog{text: text})
}

// Writer returns an io.Writer that routes lines into the TUI log panel.
// Suitable for use as the logrus output writer.
func (u *UI) Writer() io.Writer {
	return u.logW
}

// PrintSummary writes the final summary block directly (after TUI exits).
// Call this AFTER Wait() so the alternate screen has been restored.
func (u *UI) PrintSummary(lines []string) {
	for _, l := range lines {
		fmt.Println(l)
	}
}

// ── tuiWriter ─────────────────────────────────────────────────────────────────

// tuiWriter adapts io.Writer to route log lines into the TUI via Send.
type tuiWriter struct {
	prog *tea.Program
	buf  strings.Builder
	mu   sync.Mutex
	// plain mode: write directly instead of sending to Bubble Tea
	plain  bool
	plainW io.Writer
}

func (w *tuiWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.plain {
		return w.plainW.Write(p)
	}
	w.buf.Write(p)
	// Flush complete lines
	for {
		s := w.buf.String()
		idx := strings.IndexByte(s, '\n')
		if idx < 0 {
			break
		}
		line := strings.TrimRight(s[:idx], "\r")
		w.buf.Reset()
		w.buf.WriteString(s[idx+1:])
		if line != "" {
			w.prog.Send(msgLog{text: line})
		}
	}
	return len(p), nil
}
