package installer

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Palette. Adaptive colors pick a shade that reads on light and dark terminals.
const tuiAccentHex = "#2F5BD8"

var (
	tuiAccent = lipgloss.AdaptiveColor{Light: tuiAccentHex, Dark: "#6E8FF2"}
	tuiText   = lipgloss.AdaptiveColor{Light: "#1C1C1C", Dark: "#E4E4E4"}
	tuiGrey   = lipgloss.AdaptiveColor{Light: "#8A8A8A", Dark: "#7C7C7C"}
	tuiGreen  = lipgloss.AdaptiveColor{Light: "28", Dark: "35"}
	tuiYellow = lipgloss.AdaptiveColor{Light: "130", Dark: "178"}
	tuiRed    = lipgloss.AdaptiveColor{Light: "160", Dark: "160"}
)

const (
	tuiMinWidth     = 40
	tuiDefaultWidth = 80
	tuiTailLines    = 10
)

// tuiStrings holds every piece of text the TUI renders itself.
type tuiStrings struct {
	subtitle, required, mismatch, repeat, next, back, choose, exit string
	key, checkCode, fingerprint, instruction                       string
}

var tuiTexts = map[Lang]tuiStrings{
	RU: {
		subtitle:    "установка",
		required:    "Поле обязательно",
		mismatch:    "Значения не совпадают, введите снова",
		repeat:      "повторите",
		next:        "далее",
		back:        "назад",
		choose:      "↑↓ выбор",
		exit:        "Нажмите любую клавишу, чтобы выйти",
		key:         "Ключ",
		checkCode:   "Код сверки",
		fingerprint: "Отпечаток",
		instruction: "Введите ключ в приложении: «Добавить сервер», проверьте, что код совпадает",
	},
	EN: {
		subtitle:    "installer",
		required:    "This field is required",
		mismatch:    "Values do not match, enter again",
		repeat:      "repeat",
		next:        "next",
		back:        "back",
		choose:      "↑↓ choose",
		exit:        "Press any key to exit",
		key:         "Key",
		checkCode:   "Check code",
		fingerprint: "Fingerprint",
		instruction: "Enter the key in the application: «Add server», check that the code matches",
	},
}

func tuiStringsFor(l Lang) tuiStrings {
	if s, ok := tuiTexts[l]; ok {
		return s
	}
	return tuiTexts[EN]
}

// NewTUI returns the interactive bubbletea frontend. It draws on the alternate screen
// and prints the final Summary to the normal screen once the program exits.
func NewTUI() Frontend { return tuiFrontend{} }

type tuiFrontend struct{}

type tuiFlowResult struct {
	summary *Summary
	err     error
}

// Run implements Frontend.
func (tuiFrontend) Run(ctx context.Context, lang Lang, flow Flow) (*Summary, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	root := newTUIModel(lang)
	p := tea.NewProgram(root, tea.WithAltScreen(), tea.WithContext(ctx))
	con := &tuiConsole{p: p, ctx: ctx, lang: lang}

	flowCh := make(chan tuiFlowResult, 1)
	go func() {
		s, err := flow(ctx, con)
		flowCh <- tuiFlowResult{summary: s, err: err}
		p.Send(tuiFlowFinished{})
	}()

	if _, err := p.Run(); err != nil {
		return nil, err
	}
	if root.aborted {
		cancel()
		select {
		case <-flowCh:
		case <-time.After(2 * time.Second):
		}
		return nil, ErrAborted
	}

	res := <-flowCh
	printSummary(res.summary)
	return res.summary, res.err
}

// tuiConsole bridges the Console interface to a running bubbletea program.
type tuiConsole struct {
	p    *tea.Program
	ctx  context.Context
	lang Lang
}

func (c *tuiConsole) Interactive() bool { return true }

func (c *tuiConsole) Lang() Lang { return c.lang }

func (c *tuiConsole) Ask(f Form) (map[string]string, error) {
	reply := make(chan askResult, 1)
	c.p.Send(askMsg{form: f, reply: reply})
	select {
	case res := <-reply:
		return res.answers, res.err
	case <-c.ctx.Done():
		return nil, ErrAborted
	}
}

func (c *tuiConsole) Steps(titles []string) {
	c.p.Send(stepsMsg{titles: append([]string(nil), titles...)})
}

func (c *tuiConsole) Event(e Event) { c.p.Send(eventMsg{e: e}) }

func (c *tuiConsole) Wait(w Waiting) { c.p.Send(waitMsg{w: w}) }

// Messages exchanged between the flow goroutine and the program.
type (
	askMsg struct {
		form  Form
		reply chan askResult
	}
	askResult struct {
		answers map[string]string
		err     error
	}
	stepsMsg struct{ titles []string }
	eventMsg struct{ e Event }
	waitMsg  struct{ w Waiting }
	// tuiFlowFinished is sent after the flow returned and its result was stored.
	tuiFlowFinished struct{}
)

type stepState int

const (
	stepPending stepState = iota
	stepRunning
	stepDone
	stepSkipped
	stepFailed
)

type tuiStep struct {
	title string
	state stepState
	info  string
}

// tuiModel is the root bubbletea model: header plus whichever screen is active.
type tuiModel struct {
	lang    Lang
	strs    tuiStrings
	width   int
	spin    spinner.Model
	bar     progress.Model
	form    *formModel
	reply   chan askResult
	waiting *Waiting
	steps   []tuiStep
	failed  bool
	failErr error
	tail    []string
	flowEnd bool
	aborted bool
}

func newTUIModel(lang Lang) *tuiModel {
	return &tuiModel{
		lang:  lang,
		strs:  tuiStringsFor(lang),
		width: tuiDefaultWidth,
		spin: spinner.New(
			spinner.WithSpinner(spinner.Dot),
			spinner.WithStyle(lipgloss.NewStyle().Foreground(tuiAccent)),
		),
		bar: progress.New(
			progress.WithSolidFill(tuiAccentHex),
			progress.WithoutPercentage(),
			progress.WithWidth(40),
		),
	}
}

// Init implements tea.Model.
func (r *tuiModel) Init() tea.Cmd { return r.spin.Tick }

// Update implements tea.Model.
func (r *tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		r.width = max(msg.Width, tuiMinWidth)
		r.bar.Width = min(max(r.width-8, 20), 60)
		if r.form != nil {
			r.form.resize(r.width)
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		r.spin, cmd = r.spin.Update(msg)
		return r, cmd
	case askMsg:
		f := newFormModel(r.lang, msg.form, r.width)
		r.form = &f
		r.reply = msg.reply
		r.waiting = nil
		if f.done {
			return r.finishForm()
		}
	case waitMsg:
		w := msg.w
		r.waiting = &w
	case stepsMsg:
		r.steps = make([]tuiStep, len(msg.titles))
		for i, t := range msg.titles {
			r.steps[i] = tuiStep{title: t}
		}
	case eventMsg:
		r.waiting = nil
		r.applyEvent(msg.e)
	case tuiFlowFinished:
		r.flowEnd = true
		if !r.failed {
			return r, tea.Quit
		}
	case tea.KeyMsg:
		return r.handleKey(msg)
	}
	return r, nil
}

func (r *tuiModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyCtrlC {
		return r.abort()
	}
	if r.form != nil {
		nm, cmd := r.form.Update(msg)
		*r.form = nm
		switch {
		case nm.aborted:
			return r.abort()
		case nm.done:
			return r.finishForm()
		}
		return r, cmd
	}
	if r.failed && r.flowEnd {
		return r, tea.Quit
	}
	return r, nil
}

func (r *tuiModel) finishForm() (tea.Model, tea.Cmd) {
	r.reply <- askResult{answers: r.form.answers}
	r.form = nil
	r.reply = nil
	return r, nil
}

func (r *tuiModel) abort() (tea.Model, tea.Cmd) {
	r.aborted = true
	if r.reply != nil {
		r.reply <- askResult{err: ErrAborted}
		r.reply = nil
	}
	r.form = nil
	return r, tea.Quit
}

func (r *tuiModel) applyEvent(e Event) {
	if e.Index < 0 || e.Index >= len(r.steps) {
		return
	}
	s := &r.steps[e.Index]
	switch e.State {
	case StepStart:
		s.state = stepRunning
		s.info = e.Info
	case StepDone:
		s.state = stepDone
		s.info = ""
	case StepSkip:
		s.state = stepSkipped
		s.info = ""
	case StepFail:
		s.state = stepFailed
		s.info = ""
		r.failed = true
		r.failErr = e.Err
		r.tail = tuiLastLines(e.Tail, tuiTailLines)
	}
}

// View implements tea.Model.
func (r *tuiModel) View() string {
	var body string
	switch {
	case r.form != nil:
		body = r.form.View()
	case r.waiting != nil:
		body = r.waitView()
	case len(r.steps) > 0:
		body = r.runView()
	}
	margin := lipgloss.NewStyle().MarginLeft(2)
	return margin.Render(r.header()) + "\n\n" + margin.Render(body) + "\n"
}

func (r *tuiModel) header() string {
	text := lipgloss.NewStyle().Foreground(tuiText)
	square := lipgloss.NewStyle().Foreground(tuiAccent)
	rows := []string{
		text.Render("████████████"),
		text.Render("████████") + "  " + square.Render("██"),
		text.Render("████"),
	}
	titles := []string{
		lipgloss.NewStyle().Bold(true).Foreground(tuiAccent).Render("Linager"),
		lipgloss.NewStyle().Foreground(tuiGrey).Render(r.strs.subtitle),
		"",
	}
	lines := make([]string, len(rows))
	for i := range rows {
		lines[i] = lipgloss.NewStyle().Width(12).Render(rows[i]) + "  " + titles[i]
	}
	return strings.Join(lines, "\n")
}

func (r *tuiModel) runView() string {
	var b strings.Builder
	done := 0
	for _, st := range r.steps {
		var mark, label string
		switch st.state {
		case stepDone:
			mark, label = tuiStyled(tuiGreen).Render("✓"), st.title
			done++
		case stepSkipped:
			mark, label = tuiStyled(tuiGrey).Render("–"), tuiStyled(tuiGrey).Render(st.title)
			done++
		case stepRunning:
			mark, label = r.spin.View(), tuiStyled(tuiText).Bold(true).Render(st.title)
			if st.info != "" {
				label += "  " + tuiStyled(tuiGrey).Render(st.info)
			}
		case stepFailed:
			mark, label = tuiStyled(tuiRed).Render("✗"), tuiStyled(tuiRed).Render(st.title)
		default:
			mark, label = tuiStyled(tuiGrey).Render("○"), tuiStyled(tuiGrey).Render(st.title)
		}
		b.WriteString(mark + " " + label + "\n")
	}

	frac := 0.0
	if len(r.steps) > 0 {
		frac = float64(done) / float64(len(r.steps))
	}
	b.WriteString("\n" + r.bar.ViewAs(frac) + "\n")

	if r.failed {
		msg := "failed"
		if r.failErr != nil {
			msg = r.failErr.Error()
		}
		b.WriteString("\n" + tuiStyled(tuiRed).Render(tuiTruncate(msg, r.textWidth())) + "\n")
		if len(r.tail) > 0 {
			lines := make([]string, len(r.tail))
			for i, l := range r.tail {
				lines[i] = tuiTruncate(l, r.textWidth())
			}
			box := lipgloss.NewStyle().
				Border(lipgloss.NormalBorder()).
				BorderForeground(tuiGrey).
				Foreground(tuiGrey).
				Padding(0, 1)
			b.WriteString(box.Render(strings.Join(lines, "\n")) + "\n")
		}
		b.WriteString("\n" + tuiStyled(tuiGrey).Render(r.strs.exit) + "\n")
	}
	return b.String()
}

func (r *tuiModel) waitView() string {
	w := r.waiting
	var b strings.Builder
	b.WriteString(tuiStyled(tuiGrey).Render(r.strs.key) + "\n")
	b.WriteString("\n    " + lipgloss.NewStyle().Bold(true).Foreground(tuiAccent).Render(w.Key) + "\n\n")
	b.WriteString(tuiStyled(tuiGrey).Render(r.strs.checkCode) + "\n")
	b.WriteString("    " + lipgloss.NewStyle().Bold(true).Render(w.Code) + "\n\n")
	if w.Server != "" {
		b.WriteString(tuiStyled(tuiGrey).Render(w.Server) + "\n")
	}
	if w.Fingerprint != "" {
		b.WriteString(tuiStyled(tuiGrey).Render(r.strs.fingerprint+": "+w.Fingerprint) + "\n")
	}
	b.WriteString("\n" + r.spin.View() + " " + tuiStyled(tuiYellow).Render(r.strs.instruction) + "\n")
	return b.String()
}

// textWidth is the usable width for free text inside the body margin.
func (r *tuiModel) textWidth() int { return max(r.width-8, 20) }

// formModel is one question-per-screen form. It is a plain value type so it can be
// driven directly in tests through Update.
type formModel struct {
	strs       tuiStrings
	form       Form
	width      int
	idx        int // index into form.Fields, -1 before the first field
	history    []int
	answers    map[string]string
	input      textinput.Model
	choice     int
	pending    string // first entry of a confirmed secret
	confirming bool
	errMsg     string
	done       bool
	aborted    bool
}

func newFormModel(lang Lang, f Form, width int) formModel {
	ti := textinput.New()
	ti.Prompt = ""
	m := formModel{
		strs:    tuiStringsFor(lang),
		form:    f,
		idx:     -1,
		answers: map[string]string{},
		input:   ti,
	}
	m.resize(width)
	m.next()
	return m
}

func (m *formModel) resize(width int) {
	m.width = width
	m.input.Width = m.boxWidth() - 2
}

func (m formModel) boxWidth() int { return min(max(m.width-4, 24), 60) }

func (m formModel) isVisible(i int) bool {
	f := m.form.Fields[i]
	return f.Visible == nil || f.Visible(m.answers)
}

func (m formModel) visibleCount() int {
	n := 0
	for i := range m.form.Fields {
		if m.isVisible(i) {
			n++
		}
	}
	return n
}

// next moves to the next visible field, or marks the form done.
func (m *formModel) next() {
	for i := m.idx + 1; i < len(m.form.Fields); i++ {
		if m.isVisible(i) {
			if m.idx >= 0 {
				m.history = append(m.history, m.idx)
			}
			m.enter(i)
			return
		}
	}
	m.done = true
}

// back returns to the previous visible field; false when there is none.
func (m *formModel) back() bool {
	if len(m.history) == 0 {
		return false
	}
	prev := m.history[len(m.history)-1]
	m.history = m.history[:len(m.history)-1]
	m.enter(prev)
	return true
}

func (m *formModel) enter(i int) {
	m.idx = i
	m.errMsg = ""
	m.confirming = false
	m.pending = ""
	f := m.form.Fields[i]
	cur, ok := m.answers[f.Key]
	if !ok {
		cur = f.Default
	}
	if f.Kind == KindChoice {
		m.choice = 0
		for j, o := range f.Options {
			if o.Value == cur {
				m.choice = j
			}
		}
		return
	}
	if f.Kind == KindSecret {
		m.input.EchoMode = textinput.EchoPassword
		m.input.EchoCharacter = '•'
	} else {
		m.input.EchoMode = textinput.EchoNormal
	}
	m.input.SetValue(cur)
	m.input.CursorEnd()
	m.input.Focus()
}

// Update handles one message and returns the new model state.
func (m formModel) Update(msg tea.Msg) (formModel, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	if m.done || m.aborted || m.idx < 0 {
		return m, nil
	}
	switch km.Type {
	case tea.KeyCtrlC:
		m.aborted = true
		return m, nil
	case tea.KeyEsc:
		if m.confirming {
			m.confirming = false
			m.pending = ""
			m.errMsg = ""
			m.input.SetValue("")
			return m, nil
		}
		if !m.back() {
			m.aborted = true
		}
		return m, nil
	}
	if m.form.Fields[m.idx].Kind == KindChoice {
		return m.updateChoice(km), nil
	}
	return m.updateInput(km)
}

func (m formModel) updateChoice(km tea.KeyMsg) formModel {
	f := m.form.Fields[m.idx]
	switch km.Type {
	case tea.KeyUp, tea.KeyLeft, tea.KeyShiftTab:
		if m.choice > 0 {
			m.choice--
		}
	case tea.KeyDown, tea.KeyRight, tea.KeyTab:
		if m.choice < len(f.Options)-1 {
			m.choice++
		}
	case tea.KeyEnter:
		value := ""
		if m.choice < len(f.Options) {
			value = f.Options[m.choice].Value
		}
		m.commit(f, value)
	}
	return m
}

func (m formModel) updateInput(km tea.KeyMsg) (formModel, tea.Cmd) {
	f := m.form.Fields[m.idx]
	if km.Type == tea.KeyEnter {
		m.submitInput(f)
		return m, nil
	}
	if km.Type == tea.KeySpace {
		km = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}}
	}
	if f.Kind == KindNumber && km.Type == tea.KeyRunes {
		digits := make([]rune, 0, len(km.Runes))
		for _, r := range km.Runes {
			if r >= '0' && r <= '9' {
				digits = append(digits, r)
			}
		}
		if len(digits) == 0 {
			return m, nil
		}
		km.Runes = digits
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(km)
	return m, cmd
}

func (m *formModel) submitInput(f Field) {
	value := m.input.Value()
	if f.Kind != KindSecret {
		value = strings.TrimSpace(value)
	}
	if strings.TrimSpace(value) == "" && f.Required {
		m.errMsg = m.strs.required
		return
	}
	if f.Kind == KindSecret && f.Confirm && (value != "" || m.confirming) {
		if !m.confirming {
			if err := m.check(f, value); err != nil {
				m.errMsg = err.Error()
				return
			}
			m.pending = value
			m.confirming = true
			m.errMsg = ""
			m.input.SetValue("")
			return
		}
		if value != m.pending {
			m.errMsg = m.strs.mismatch
			m.confirming = false
			m.pending = ""
			m.input.SetValue("")
			return
		}
	}
	m.commit(f, value)
}

func (m *formModel) check(f Field, value string) error {
	if f.Validate == nil {
		return nil
	}
	return f.Validate(value, m.answers)
}

func (m *formModel) commit(f Field, value string) {
	if err := m.check(f, value); err != nil {
		m.errMsg = err.Error()
		return
	}
	m.answers[f.Key] = value
	m.errMsg = ""
	m.next()
}

// View renders the current field.
func (m formModel) View() string {
	if m.idx < 0 || m.idx >= len(m.form.Fields) {
		return ""
	}
	f := m.form.Fields[m.idx]
	var b strings.Builder

	b.WriteString(tuiStyled(tuiGrey).Render(fmt.Sprintf("%d / %d", len(m.history)+1, m.visibleCount())) + "\n\n")
	if m.form.Title != "" {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(tuiAccent).Render(m.form.Title) + "\n\n")
	}
	label := f.Label
	if m.confirming {
		label += "  ·  " + m.strs.repeat
	}
	b.WriteString(lipgloss.NewStyle().Bold(true).Render(label) + "\n")

	if f.Kind == KindChoice {
		for j, o := range f.Options {
			text := o.Label
			if text == "" {
				text = o.Value
			}
			if j == m.choice {
				b.WriteString(tuiStyled(tuiAccent).Render("❯ ") + lipgloss.NewStyle().Bold(true).Render(text) + "\n")
			} else {
				b.WriteString("  " + text + "\n")
			}
		}
	} else {
		box := lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(tuiAccent).
			Padding(0, 1).
			Width(m.boxWidth())
		b.WriteString(box.Render(m.input.View()) + "\n")
	}

	if f.Hint != "" {
		b.WriteString(tuiStyled(tuiGrey).Render(f.Hint) + "\n")
	}
	if m.errMsg != "" {
		b.WriteString(tuiStyled(tuiRed).Render(m.errMsg) + "\n")
	}

	footer := fmt.Sprintf("Enter %s · Esc %s", m.strs.next, m.strs.back)
	if f.Kind == KindChoice {
		footer = m.strs.choose + " · " + footer
	}
	b.WriteString("\n" + tuiStyled(tuiGrey).Render(footer))
	return b.String()
}

// printSummary writes the final summary to the normal screen.
func printSummary(s *Summary) {
	if s == nil {
		return
	}
	titleColor := tuiRed
	if s.Success {
		titleColor = tuiGreen
	}
	if s.Title != "" {
		_, _ = fmt.Fprintln(os.Stdout, lipgloss.NewStyle().Bold(true).Foreground(titleColor).Render(s.Title))
	}
	width := 0
	for _, kv := range s.Lines {
		width = max(width, utf8.RuneCountInString(kv.Label))
	}
	for _, kv := range s.Lines {
		pad := strings.Repeat(" ", width-utf8.RuneCountInString(kv.Label))
		_, _ = fmt.Fprintf(os.Stdout, "  %s%s  %s\n", kv.Label, pad, kv.Value)
	}
	for _, n := range s.Notes {
		_, _ = fmt.Fprintln(os.Stdout, tuiStyled(tuiGrey).Render("  "+n))
	}
}

func tuiStyled(c lipgloss.TerminalColor) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(c)
}

// tuiLastLines keeps at most n trailing lines.
func tuiLastLines(lines []string, n int) []string {
	if len(lines) <= n {
		return lines
	}
	return lines[len(lines)-n:]
}

// tuiTruncate shortens s to at most limit runes.
func tuiTruncate(s string, limit int) string {
	if limit < 1 {
		return ""
	}
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	r := []rune(s)
	return string(r[:limit-1]) + "…"
}
