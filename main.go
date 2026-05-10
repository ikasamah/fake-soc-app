// fake-soc app — terminal-independent GUI version (Fyne).
// Single binary that opens a full-screen window and renders the dashboard
// with a fully controlled theme/colour scheme. The host terminal's
// settings (colors, font) do not affect the look.
//
// Run:
//   go build -o fake-soc-app .
//   ./fake-soc-app          # opens a borderless full-screen window
//
// Quit: Cmd-Q (macOS) or Esc.
package main

import (
	_ "embed"
	"flag"
	"fmt"
	"image/color"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// Embedded Cica (Japanese-English unified monospace, OFL).
// Half-width chars take 1 cell, full-width 2 cells — aligns cleanly in TextGrid.
//
//go:embed assets/font.ttf
var fontTTF []byte

var embeddedFont = fyne.NewStaticResource("font.ttf", fontTTF)

// Configurable display sizing.
//
// Default = "auto": derive font size from window size on first frame so the
// dashboard fills the screen at a comfortable density.
//
// Manual override:
//   ./fake-soc-app -font-size 22
//   FAKESOC_FONT_SIZE=22 ./fake-soc-app
//
// charW / charH are derived from fontSize and used to lay out sparkline
// columns and wrap the LLM stream.
var (
	fontSize float32 = 18 // initial; replaced when fontMode == "auto"
	charW    float32      // derived (half-width cell)
	charH    float32      // derived (line height)
	fontMode = "auto"     // "auto" | "manual"
)

func parseConfig() {
	flagSize := flag.Float64("font-size", 0, "monospace font size in points; 0 = auto from window size")
	flag.Parse()

	envSize := os.Getenv("FAKESOC_FONT_SIZE")

	switch {
	case *flagSize > 0:
		fontSize = float32(*flagSize)
		fontMode = "manual"
	case envSize != "":
		if v, err := strconv.ParseFloat(envSize, 32); err == nil && v > 0 {
			fontSize = float32(v)
			fontMode = "manual"
		}
	}
	if fontSize < 8 {
		fontSize = 8
	}
	if fontSize > 64 {
		fontSize = 64
	}
	updateMetrics()
}

func updateMetrics() {
	// Fyne TextGrid uses a cell pitch close to fontSize. Slight bias keeps
	// the row count from being one short on most displays.
	charW = fontSize * 0.55
	charH = fontSize * 0.95
}

// runeVisualWidth returns 1 for half-width and 2 for full-width chars,
// matching how Cica renders them in the TextGrid.
func runeVisualWidth(r rune) int {
	switch {
	case r < 0x80:
		return 1
	case unicode.Is(unicode.Hiragana, r),
		unicode.Is(unicode.Katakana, r),
		unicode.Is(unicode.Han, r),
		unicode.Is(unicode.Hangul, r):
		return 2
	case r >= 0x3000 && r <= 0x303F: // CJK symbols/punctuation
		return 2
	case r >= 0xFF00 && r <= 0xFF60: // fullwidth ASCII
		return 2
	case r >= 0xFFE0 && r <= 0xFFE6: // fullwidth signs
		return 2
	case r >= 0x2E80 && r <= 0x9FFF: // CJK
		return 2
	}
	return 1
}

// wrapByVisualWidth folds a line so each piece's visual width is <= maxCols.
// Half-width = 1 cell, full-width = 2 cells. Empty input returns one empty
// line so the caller still gets a row to render.
func wrapByVisualWidth(s string, maxCols int) []string {
	if maxCols <= 0 {
		return []string{s}
	}
	var out []string
	var cur strings.Builder
	visW := 0
	for _, r := range s {
		w := runeVisualWidth(r)
		if visW+w > maxCols && cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
			visW = 0
		}
		cur.WriteRune(r)
		visW += w
	}
	if cur.Len() > 0 || len(out) == 0 {
		out = append(out, cur.String())
	}
	return out
}

// truncateByVisualWidth cuts s at maxCols visual cells. Used when wrap
// would cause too many rows (e.g. log feed). Returns s if maxCols <= 0.
func truncateByVisualWidth(s string, maxCols int) string {
	if maxCols <= 0 {
		return s
	}
	visW := 0
	var b strings.Builder
	for _, r := range s {
		w := runeVisualWidth(r)
		if visW+w > maxCols {
			break
		}
		b.WriteRune(r)
		visW += w
	}
	return b.String()
}

// expandFullwidth pads each full-width rune with a trailing space so each
// glyph occupies 2 TextGrid cells on screen. Without this, Cica's full-width
// glyphs (rendered with 2 advance units) overlap the next cell because Fyne's
// TextGrid uses a fixed half-width cell pitch.
func expandFullwidth(s string) string {
	var b strings.Builder
	for _, r := range s {
		b.WriteRune(r)
		if runeVisualWidth(r) == 2 {
			b.WriteRune(' ')
		}
	}
	return b.String()
}

// ============ Theme ============

type fakeSocTheme struct{}

var (
	bgColor      = color.NRGBA{R: 0x0E, G: 0x0E, B: 0x10, A: 0xFF}
	fgColor      = color.NRGBA{R: 0xC8, G: 0xC8, B: 0xCA, A: 0xFF}
	dimColor     = color.NRGBA{R: 0x6A, G: 0x6A, B: 0x6E, A: 0xFF}
	dimColor2    = color.NRGBA{R: 0x40, G: 0x40, B: 0x44, A: 0xFF}
	cyanColor    = color.NRGBA{R: 0x6E, G: 0xA8, B: 0xC8, A: 0xFF}
	greenColor   = color.NRGBA{R: 0x88, G: 0xC0, B: 0x90, A: 0xFF}
	blueColor    = color.NRGBA{R: 0x70, G: 0x90, B: 0xC0, A: 0xFF}
	amberColor   = color.NRGBA{R: 0xD9, G: 0xA0, B: 0x4E, A: 0xFF}
	redColor     = color.NRGBA{R: 0xD8, G: 0x60, B: 0x60, A: 0xFF}
	redBgColor   = color.NRGBA{R: 0x6E, G: 0x16, B: 0x18, A: 0xFF}
	whiteBrColor = color.NRGBA{R: 0xF4, G: 0xF4, B: 0xF6, A: 0xFF}
)

func (fakeSocTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	switch n {
	case theme.ColorNameBackground:
		return bgColor
	case theme.ColorNameForeground:
		return fgColor
	case theme.ColorNameDisabled:
		return dimColor
	case theme.ColorNamePlaceHolder:
		return dimColor
	case theme.ColorNamePrimary:
		return cyanColor
	case theme.ColorNameInputBackground:
		return bgColor
	case theme.ColorNameOverlayBackground:
		return bgColor
	case theme.ColorNameButton:
		return bgColor
	case theme.ColorNameMenuBackground:
		return bgColor
	case theme.ColorNameSeparator:
		return dimColor2
	}
	return theme.DefaultTheme().Color(n, v)
}

func (fakeSocTheme) Font(s fyne.TextStyle) fyne.Resource {
	// Cica handles both Latin and Japanese in a single monospace face,
	// preventing fallback chains that would otherwise mis-render CJK.
	return embeddedFont
}

func (fakeSocTheme) Icon(n fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(n)
}

func (fakeSocTheme) Size(n fyne.ThemeSizeName) float32 {
	switch n {
	case theme.SizeNameText:
		return fontSize
	case theme.SizeNameSeparatorThickness:
		return 1
	case theme.SizeNameInnerPadding:
		return 4
	case theme.SizeNamePadding:
		return 2
	}
	return theme.DefaultTheme().Size(n)
}

// ============ Scenarios ============

type Scenario int

const (
	ScenarioNormal Scenario = iota
	ScenarioDegraded
	ScenarioIncident
	ScenarioRecovery
)

var scenarioCycle = []struct {
	scenario Scenario
	duration time.Duration
}{
	{ScenarioNormal, 50 * time.Second},
	{ScenarioDegraded, 10 * time.Second},
	{ScenarioIncident, 20 * time.Second},
	{ScenarioRecovery, 10 * time.Second},
}

// ============ Sparkline ============

var sparkChars = []rune{' ', '▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

const sparkSteps = 9

// ============ State ============

type appState struct {
	rng *rand.Rand

	scenario      Scenario
	scenarioIdx   int
	scenarioStart time.Time

	// Left
	leftLog       *strings.Builder
	leftCurrent   *Response
	leftStreamIdx int
	leftPhase     int // 0 idle, 2 thinking, 3 streaming, 4 done
	leftPhaseAt   time.Time
	leftSpinIdx   int

	// Right top
	rtSeries       [][]int
	rtCols         int
	rtRows         int
	dialogActive   bool
	dialogMsg      string
	dialogStartAt  time.Time
	dialogScenario Scenario
	dialogSpin     int
	nextDialogAt   time.Time

	// Stats (rendered below sparklines)
	statValues map[string]float64
	statTrend  map[string]int // -1, 0, 1

	// Right bottom
	logLines []string
	logMax   int
}

// ============ Helpers ============

func metricKey(label string) string {
	return strings.TrimRight(label, " ")
}

func hexN(rng *rand.Rand, n int) string {
	const hex = "0123456789abcdef"
	out := make([]byte, n)
	for i := 0; i < n; i++ {
		out[i] = hex[rng.Intn(16)]
	}
	return string(out)
}

func paint(s string, c color.Color) string {
	// We use lipgloss-free approach: emit raw text and decorate via TextGrid
	// per-cell styles in the renderer. The string itself is plain.
	// (Helper retained for future ANSI-string rendering if needed.)
	_ = c
	return s
}

// ============ Scenario advance ============

func (s *appState) advanceScenario() {
	if time.Since(s.scenarioStart) < scenarioCycle[s.scenarioIdx].duration {
		return
	}
	s.scenarioIdx = (s.scenarioIdx + 1) % len(scenarioCycle)
	s.scenario = scenarioCycle[s.scenarioIdx].scenario
	s.scenarioStart = time.Now()
	s.fireDialog()
}

// ============ Left pane (LLM stream) ============

func (s *appState) responsesForCurrent() []*Response {
	switch s.scenario {
	case ScenarioNormal:
		return responsesNormal()
	case ScenarioDegraded:
		return responsesDegraded()
	case ScenarioIncident:
		return responsesIncident()
	case ScenarioRecovery:
		return responsesRecovery()
	}
	return responsesNormal()
}

func (s *appState) stepLeft() {
	switch s.leftPhase {
	case 0:
		pool := s.responsesForCurrent()
		s.leftCurrent = pool[s.rng.Intn(len(pool))]
		s.leftLog.WriteString("> " + s.leftCurrent.Prompt + "\n\n")
		s.leftPhase = 2
		s.leftPhaseAt = time.Now()
	case 2:
		if time.Since(s.leftPhaseAt) > time.Duration(900+s.rng.Intn(700))*time.Millisecond {
			s.leftPhase = 3
			s.leftStreamIdx = 0
		}
	case 3:
		if s.leftStreamIdx >= len(s.leftCurrent.Chunks) {
			s.leftLog.WriteString("\n─────────────────────────────────────────────────────────\n\n")
			s.leftPhase = 4
			s.leftPhaseAt = time.Now()
			break
		}
		s.leftLog.WriteString(s.leftCurrent.Chunks[s.leftStreamIdx])
		s.leftStreamIdx++
	case 4:
		if time.Since(s.leftPhaseAt) > time.Duration(800+s.rng.Intn(700))*time.Millisecond {
			s.leftPhase = 0
		}
	}

	// trim
	if s.leftLog.Len() > 30000 {
		full := s.leftLog.String()
		half := len(full) / 2
		if cut := strings.IndexByte(full[half:], '\n'); cut >= 0 {
			full = full[half+cut+1:]
		}
		s.leftLog.Reset()
		s.leftLog.WriteString(full)
	}
}

// ============ Right top (sparklines + dialog) ============

// statsRowCount = 1 separator + N stats lines that we render below the sparklines
const statsRowCount = 11

func (s *appState) recalcSparkSize(width, height float32, charW, charH float32) {
	const labelW = 22 // approx character columns reserved for label
	const suffixW = 6 // " 100%" + spaces

	cols := int(width/charW) - labelW - suffixW
	if cols < 20 {
		cols = 20
	}
	rows := int(height/charH) - 2 - statsRowCount
	if rows < 4 {
		rows = 4
	}
	if rows > len(Metrics) {
		rows = len(Metrics)
	}

	if s.rtCols == cols && s.rtRows == rows && s.rtSeries != nil {
		return
	}
	s.rtCols = cols
	s.rtRows = rows
	s.rtSeries = make([][]int, rows)
	for i := 0; i < rows; i++ {
		s.rtSeries[i] = make([]int, cols)
		for j := 0; j < cols; j++ {
			s.rtSeries[i][j] = s.rng.Intn(4)
		}
	}
}

func (s *appState) stepSparklines() {
	if s.rtSeries == nil || s.rtCols == 0 {
		return
	}
	for i := 0; i < s.rtRows; i++ {
		row := s.rtSeries[i]
		last := row[len(row)-1]
		next := last + (s.rng.Intn(5) - 2)

		isSpike := SpikeMetricKeys[metricKey(Metrics[i])]
		switch {
		case s.scenario == ScenarioIncident && isSpike:
			next = 6 + s.rng.Intn(3)
		case s.scenario == ScenarioDegraded && isSpike:
			next = 3 + s.rng.Intn(3)
		case s.scenario == ScenarioRecovery && isSpike:
			if last > 3 {
				next = last - 1
			}
		}

		if s.rng.Intn(60) == 0 {
			next = sparkSteps - 1
		}
		if s.rng.Intn(70) == 0 {
			next = 0
		}

		if next < 0 {
			next = 0
		}
		if next >= sparkSteps {
			next = sparkSteps - 1
		}

		copy(row, row[1:])
		row[len(row)-1] = next
	}
}

// ============ Stats step ============

func (s *appState) stepStats() {
	if s.statValues == nil {
		s.statValues = map[string]float64{
			"pnl":     0,
			"trades":  82400,
			"usdjpy":  148.43,
			"eurusd":  1.0823,
			"gbpusd":  1.2654,
			"hedge":   98.4,
			"ack":     12,
			"clear":   18,
			"vwap":    18342.5,
			"book":    -0.32,
		}
		s.statTrend = map[string]int{}
	}
	step := func(key string, delta float64) {
		old := s.statValues[key]
		s.statValues[key] += delta
		switch {
		case s.statValues[key] > old:
			s.statTrend[key] = 1
		case s.statValues[key] < old:
			s.statTrend[key] = -1
		default:
			s.statTrend[key] = 0
		}
	}
	scl := 1.0
	if s.scenario == ScenarioIncident {
		scl = 4.0
	} else if s.scenario == ScenarioDegraded {
		scl = 2.0
	}
	step("pnl", (s.rng.Float64()-0.5)*50000*scl)
	s.statValues["trades"] += float64(s.rng.Intn(40))
	s.statTrend["trades"] = 1
	step("usdjpy", (s.rng.Float64()-0.5)*0.04)
	step("eurusd", (s.rng.Float64()-0.5)*0.0008)
	step("gbpusd", (s.rng.Float64()-0.5)*0.0009)
	s.statValues["hedge"] = 95 + s.rng.Float64()*5
	if s.scenario == ScenarioIncident {
		s.statValues["hedge"] = 80 + s.rng.Float64()*10
	}
	s.statTrend["hedge"] = 0
	s.statValues["ack"] = float64(s.rng.Intn(30))
	if s.scenario == ScenarioIncident {
		s.statValues["ack"] = float64(200 + s.rng.Intn(400))
	}
	s.statTrend["ack"] = 0
	s.statValues["clear"] = float64(s.rng.Intn(40) + 5)
	s.statTrend["clear"] = 0
	step("vwap", (s.rng.Float64()-0.5)*8.0)
	step("book", (s.rng.Float64()-0.5)*0.4)
}

func (s *appState) stepDialog() {
	now := time.Now()
	if !s.dialogActive && now.After(s.nextDialogAt) {
		s.fireDialog()
	}
	if s.dialogActive {
		if time.Since(s.dialogStartAt) > 5*time.Second {
			s.dialogActive = false
			var gap time.Duration
			switch s.scenario {
			case ScenarioIncident:
				gap = time.Duration(8+s.rng.Intn(8)) * time.Second
			case ScenarioDegraded:
				gap = time.Duration(15+s.rng.Intn(15)) * time.Second
			default:
				gap = time.Duration(30+s.rng.Intn(30)) * time.Second
			}
			s.nextDialogAt = time.Now().Add(gap)
		} else {
			s.dialogSpin++
		}
	}
}

func (s *appState) fireDialog() {
	var pool []string
	switch s.scenario {
	case ScenarioNormal:
		pool = DialogsNormal
	case ScenarioDegraded:
		pool = DialogsDegraded
	case ScenarioIncident:
		pool = DialogsIncident
	case ScenarioRecovery:
		pool = DialogsRecovery
	}
	s.dialogMsg = pool[s.rng.Intn(len(pool))]
	s.dialogActive = true
	s.dialogStartAt = time.Now()
	s.dialogScenario = s.scenario
	s.dialogSpin = 0
}

// ============ Right bottom (logs) ============

func (s *appState) stepLogs() {
	count := 1
	switch s.scenario {
	case ScenarioIncident:
		count = 2 + s.rng.Intn(3)
	case ScenarioDegraded:
		count = 1 + s.rng.Intn(2)
	}
	for k := 0; k < count; k++ {
		if s.scenario == ScenarioIncident && s.rng.Intn(10) == 0 {
			s.logLines = append(s.logLines, s.renderAlertLine())
		} else {
			s.logLines = append(s.logLines, s.renderLogLine())
		}
	}
	if len(s.logLines) > s.logMax {
		s.logLines = s.logLines[len(s.logLines)-s.logMax:]
	}
}

func (s *appState) pickLevel() string {
	r := s.rng.Intn(100)
	switch s.scenario {
	case ScenarioNormal:
		switch {
		case r < 70:
			return "INFO"
		case r < 90:
			return "DEBUG"
		case r < 98:
			return "WARN"
		default:
			return "ERROR"
		}
	case ScenarioDegraded:
		switch {
		case r < 45:
			return "INFO"
		case r < 55:
			return "DEBUG"
		case r < 85:
			return "WARN"
		default:
			return "ERROR"
		}
	case ScenarioIncident:
		switch {
		case r < 25:
			return "INFO"
		case r < 30:
			return "DEBUG"
		case r < 55:
			return "WARN"
		case r < 90:
			return "ERROR"
		default:
			return "FATAL"
		}
	case ScenarioRecovery:
		switch {
		case r < 78:
			return "INFO"
		case r < 88:
			return "DEBUG"
		case r < 97:
			return "WARN"
		default:
			return "ERROR"
		}
	}
	return "INFO"
}

func (s *appState) eventForLevel(level string) string {
	pick := func(p []string) string { return p[s.rng.Intn(len(p))] }
	switch level {
	case "INFO":
		return pick(EventsInfo)
	case "DEBUG":
		return pick(EventsDebug)
	case "WARN":
		return pick(EventsWarn)
	case "ERROR":
		return pick(EventsError)
	case "FATAL":
		return pick(EventsFatal)
	}
	return "noop"
}

func (s *appState) renderLogLine() string {
	level := s.pickLevel()
	event := s.eventForLevel(level)
	svc := Services[s.rng.Intn(len(Services))]
	req := hexN(s.rng, 8)
	trace := hexN(s.rng, 16)
	span := hexN(s.rng, 8)
	user := 1000 + s.rng.Intn(900000)
	status := 200 + s.rng.Intn(30) - 5
	if status < 200 {
		status = 200
	}
	if status > 599 {
		status = 599
	}
	lat := 1 + s.rng.Intn(1500)
	notional := s.rng.Intn(5_000_000)
	ts := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")

	// Inline markers: we encode level as prefix to colour later in the renderer.
	return fmt.Sprintf("\x00%s\x00 %s %-5s svc=%s event=%s trace=%s span=%s req=%s user=%d status=%d lat_ms=%d notional=%d",
		level, ts, level, svc, event, trace, span, req, user, status, lat, notional)
}

func (s *appState) renderAlertLine() string {
	line := AlertLines[s.rng.Intn(len(AlertLines))]
	return "\x00ALERT\x00 " + line
}

// ============ Renderers (Fyne TextGrid) ============

// Build a colored TextGrid for a multi-line string, applying styles.
// maxCols caps visual width per row (half=1 cell, full=2 cells) so the
// TextGrid's MinSize does not blow out the left pane.
func setLeftGrid(g *widget.TextGrid, text string, dialogScenario Scenario, maxCols int) {
	rawLines := strings.Split(text, "\n")
	rows := make([]widget.TextGridRow, 0, len(rawLines))
	for _, raw := range rawLines {
		// Pre-wrap each logical line.
		isPrompt := strings.HasPrefix(raw, "> ")
		isSep := strings.HasPrefix(raw, "─")

		// Pad full-width chars so each glyph occupies 2 cells (avoids overlap).
		raw = expandFullwidth(raw)

		wrapped := wrapByVisualWidth(raw, maxCols)
		for _, w := range wrapped {
			row := widget.TextGridRow{}
			switch {
			case isPrompt:
				styleDim := &widget.CustomTextGridStyle{FGColor: dimColor}
				for _, r := range w {
					row.Cells = append(row.Cells, widget.TextGridCell{Rune: r, Style: styleDim})
				}
			case isSep:
				styleDim := &widget.CustomTextGridStyle{FGColor: dimColor2}
				for _, r := range w {
					row.Cells = append(row.Cells, widget.TextGridCell{Rune: r, Style: styleDim})
				}
			default:
				for _, r := range w {
					row.Cells = append(row.Cells, widget.TextGridCell{Rune: r})
				}
			}
			rows = append(rows, row)
		}
	}
	g.Rows = rows
	g.Refresh()
}

func renderRightTopGrid(g *widget.TextGrid, s *appState) {
	if s.rtSeries == nil {
		g.Rows = nil
		g.Refresh()
		return
	}

	rows := make([]widget.TextGridRow, 0, s.rtRows)
	for i := 0; i < s.rtRows; i++ {
		row := widget.TextGridRow{}
		series := s.rtSeries[i]
		current := series[len(series)-1]
		pct := (current * 100) / (sparkSteps - 1)

		isSpike := SpikeMetricKeys[metricKey(Metrics[i])]
		var labelStyle *widget.CustomTextGridStyle
		switch {
		case isSpike && s.scenario == ScenarioIncident:
			labelStyle = &widget.CustomTextGridStyle{FGColor: redColor}
		case isSpike && s.scenario == ScenarioDegraded:
			labelStyle = &widget.CustomTextGridStyle{FGColor: amberColor}
		default:
			labelStyle = &widget.CustomTextGridStyle{FGColor: dimColor}
		}

		// label
		for _, r := range Metrics[i] {
			row.Cells = append(row.Cells, widget.TextGridCell{Rune: r, Style: labelStyle})
		}

		// sparkline
		sparkStyle := &widget.CustomTextGridStyle{FGColor: fgColor}
		for _, v := range series {
			row.Cells = append(row.Cells, widget.TextGridCell{Rune: sparkChars[v], Style: sparkStyle})
		}

		// percentage suffix
		row.Cells = append(row.Cells, widget.TextGridCell{Rune: ' ', Style: labelStyle})
		pctStr := fmt.Sprintf("%3d%%", pct)
		for _, r := range pctStr {
			row.Cells = append(row.Cells, widget.TextGridCell{Rune: r, Style: labelStyle})
		}

		rows = append(rows, row)
	}

	// Append stats block below sparklines
	rows = append(rows, buildStatsRows(s)...)

	// Overlay dialog if active
	if s.dialogActive {
		rows = overlayDialog(rows, s)
	}

	g.Rows = rows
	g.Refresh()
}

// ============ Stats rows ============

func buildStatsRows(s *appState) []widget.TextGridRow {
	rows := make([]widget.TextGridRow, 0, statsRowCount)

	sepStyle := &widget.CustomTextGridStyle{FGColor: dimColor2}
	labelStyle := &widget.CustomTextGridStyle{FGColor: dimColor}
	valStyle := &widget.CustomTextGridStyle{FGColor: fgColor}

	// Separator line
	sep := widget.TextGridRow{}
	header := "─── session stats " + strings.Repeat("─", 60)
	for _, r := range header {
		sep.Cells = append(sep.Cells, widget.TextGridCell{Rune: r, Style: sepStyle})
	}
	rows = append(rows, sep)

	type item struct {
		label string
		value string
		trend int
	}

	signedPNL := func(v float64) string {
		sign := "+"
		if v < 0 {
			sign = "-"
			v = -v
		}
		return fmt.Sprintf("%s$ %9.0f", sign, v)
	}
	if s.statValues == nil {
		return rows
	}

	regions := []string{"ap-northeast-1", "ap-southeast-1", "us-east-1", "eu-west-1"}
	region := regions[(int(time.Since(s.scenarioStart).Seconds())/12)%len(regions)]

	scores := []string{"AAA", "AA+", "AA", "A+"}
	if s.scenario == ScenarioIncident {
		scores = []string{"BBB-", "BB+", "BB"}
	}

	items := []item{
		{"session.pnl.usd     ", signedPNL(s.statValues["pnl"]), s.statTrend["pnl"]},
		{"trades.today        ", fmt.Sprintf("%12.0f", s.statValues["trades"]), 1},
		{"fx.usdjpy.spot      ", fmt.Sprintf("%12.4f", s.statValues["usdjpy"]), s.statTrend["usdjpy"]},
		{"fx.eurusd.spot      ", fmt.Sprintf("%12.4f", s.statValues["eurusd"]), s.statTrend["eurusd"]},
		{"fx.gbpusd.spot      ", fmt.Sprintf("%12.4f", s.statValues["gbpusd"]), s.statTrend["gbpusd"]},
		{"hedge.coverage.pct  ", fmt.Sprintf("%11.2f%%", s.statValues["hedge"]), 0},
		{"vwap.tse.100        ", fmt.Sprintf("%12.2f", s.statValues["vwap"]), s.statTrend["vwap"]},
		{"book.imbalance.pct  ", fmt.Sprintf("%+11.2f%%", s.statValues["book"]), s.statTrend["book"]},
		{"ack.pending         ", fmt.Sprintf("%12.0f", s.statValues["ack"]), 0},
		{"liquidity.score     ", fmt.Sprintf("%12s", scores[s.rng.Intn(len(scores))]), 0},
	}

	// Add session.region as the last
	items = append(items, item{"session.region      ", fmt.Sprintf("%14s", region), 0})

	// Truncate to statsRowCount-1 (separator already used 1 row)
	if len(items) > statsRowCount-1 {
		items = items[:statsRowCount-1]
	}

	for _, it := range items {
		row := widget.TextGridRow{}
		// label
		for _, r := range it.label {
			row.Cells = append(row.Cells, widget.TextGridCell{Rune: r, Style: labelStyle})
		}
		// value
		valColor := valStyle
		switch s.scenario {
		case ScenarioIncident:
			if strings.Contains(it.label, "ack.pending") || strings.Contains(it.label, "hedge.coverage") {
				valColor = &widget.CustomTextGridStyle{FGColor: redColor}
			}
		}
		for _, r := range it.value {
			row.Cells = append(row.Cells, widget.TextGridCell{Rune: r, Style: valColor})
		}
		// trend arrow
		row.Cells = append(row.Cells, widget.TextGridCell{Rune: ' ', Style: labelStyle})
		var arrow rune = ' '
		var arrowColor *widget.CustomTextGridStyle = labelStyle
		switch it.trend {
		case 1:
			arrow = '▲'
			arrowColor = &widget.CustomTextGridStyle{FGColor: greenColor}
		case -1:
			arrow = '▼'
			arrowColor = &widget.CustomTextGridStyle{FGColor: redColor}
		}
		row.Cells = append(row.Cells, widget.TextGridCell{Rune: arrow, Style: arrowColor})
		rows = append(rows, row)
	}
	return rows
}

func overlayDialog(rows []widget.TextGridRow, s *appState) []widget.TextGridRow {
	if len(rows) == 0 {
		return rows
	}

	style := &widget.CustomTextGridStyle{FGColor: dimColor}
	switch s.dialogScenario {
	case ScenarioDegraded:
		style = &widget.CustomTextGridStyle{FGColor: amberColor}
	case ScenarioIncident:
		style = &widget.CustomTextGridStyle{FGColor: redColor}
	case ScenarioRecovery:
		style = &widget.CustomTextGridStyle{FGColor: greenColor}
	}

	spinner := []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}
	sp := spinner[s.dialogSpin%len(spinner)]

	maxCols := 0
	for _, r := range rows {
		if len(r.Cells) > maxCols {
			maxCols = len(r.Cells)
		}
	}
	boxW := len(s.dialogMsg) + 14
	if boxW > maxCols-4 {
		boxW = maxCols - 4
	}
	if boxW < 50 {
		boxW = 50
	}
	boxH := 5
	x := (maxCols - boxW) / 2
	if x < 0 {
		x = 0
	}
	y := (len(rows) - boxH) / 2
	if y < 0 {
		y = 0
	}

	top := []rune("┌" + strings.Repeat("─", boxW-2) + "┐")
	mid := []rune("│" + strings.Repeat(" ", boxW-2) + "│")
	bot := []rune("└" + strings.Repeat("─", boxW-2) + "┘")
	msgPadLen := boxW - 2 - 4 - len([]rune(s.dialogMsg))
	if msgPadLen < 0 {
		msgPadLen = 0
	}
	msgLine := []rune("│  " + string(sp) + "  " + s.dialogMsg + strings.Repeat(" ", msgPadLen) + "│")

	overlayRow := func(rIdx int, content []rune) {
		if rIdx < 0 || rIdx >= len(rows) {
			return
		}
		// Replace from x to x+boxW with new content. If existing cells short, pad.
		newCells := make([]widget.TextGridCell, x+len(content))
		// Keep leading cells from existing row up to x (if exist)
		for i := 0; i < x; i++ {
			if i < len(rows[rIdx].Cells) {
				newCells[i] = rows[rIdx].Cells[i]
			} else {
				newCells[i] = widget.TextGridCell{Rune: ' '}
			}
		}
		for i, r := range content {
			newCells[x+i] = widget.TextGridCell{Rune: r, Style: style}
		}
		// Append remaining cells if any (right of the box)
		if x+len(content) < len(rows[rIdx].Cells) {
			newCells = append(newCells, rows[rIdx].Cells[x+len(content):]...)
		}
		rows[rIdx].Cells = newCells
	}

	overlayRow(y, top)
	overlayRow(y+1, mid)
	overlayRow(y+2, msgLine)
	overlayRow(y+3, mid)
	overlayRow(y+4, bot)

	return rows
}

func renderRightBottomGrid(g *widget.TextGrid, s *appState, maxRows, maxCols int) {
	rows := make([]widget.TextGridRow, 0, len(s.logLines))

	// Show last maxRows lines
	start := 0
	if len(s.logLines) > maxRows {
		start = len(s.logLines) - maxRows
	}
	for i := start; i < len(s.logLines); i++ {
		line := s.logLines[i]

		// Parse "\x00LEVEL\x00 rest"
		level := ""
		rest := line
		if strings.HasPrefix(line, "\x00") {
			end := strings.Index(line[1:], "\x00")
			if end > 0 {
				level = line[1 : 1+end]
				rest = line[2+end:]
				rest = strings.TrimPrefix(rest, " ")
			}
		}

		// Truncate so the whole line fits within maxCols (incl. trailing fields).
		// Logs are dense — wrapping each line would explode row count, so we
		// cut and let the trailing fields fall off the right edge naturally.
		if maxCols > 0 {
			rest = truncateByVisualWidth(rest, maxCols-2)
		}

		row := widget.TextGridRow{}

		if level == "ALERT" {
			alertStyle := &widget.CustomTextGridStyle{FGColor: whiteBrColor, BGColor: redBgColor}
			full := " " + rest + " "
			for _, r := range full {
				row.Cells = append(row.Cells, widget.TextGridCell{Rune: r, Style: alertStyle})
			}
		} else {
			// Decorate ts (first 24 chars), level field (5 chars), and key=value separators
			levelStyle := levelToStyle(level)
			tsStyle := &widget.CustomTextGridStyle{FGColor: dimColor2}
			keyStyle := &widget.CustomTextGridStyle{FGColor: dimColor}
			valStyle := &widget.CustomTextGridStyle{FGColor: fgColor}
			svcStyle := &widget.CustomTextGridStyle{FGColor: cyanColor}
			eventStyle := &widget.CustomTextGridStyle{FGColor: whiteBrColor}

			fields := strings.Fields(rest)
			// fields[0] = ts (long), fields[1] = LEVEL, fields[2..] = k=v pairs
			for fIdx, f := range fields {
				if fIdx > 0 {
					row.Cells = append(row.Cells, widget.TextGridCell{Rune: ' '})
				}
				switch {
				case fIdx == 0:
					for _, r := range f {
						row.Cells = append(row.Cells, widget.TextGridCell{Rune: r, Style: tsStyle})
					}
				case fIdx == 1:
					for _, r := range f {
						row.Cells = append(row.Cells, widget.TextGridCell{Rune: r, Style: levelStyle})
					}
				default:
					eq := strings.Index(f, "=")
					if eq >= 0 {
						key := f[:eq+1]
						val := f[eq+1:]
						style := valStyle
						switch {
						case strings.HasPrefix(key, "svc"):
							style = svcStyle
						case strings.HasPrefix(key, "event"):
							style = eventStyle
						}
						for _, r := range key {
							row.Cells = append(row.Cells, widget.TextGridCell{Rune: r, Style: keyStyle})
						}
						for _, r := range val {
							row.Cells = append(row.Cells, widget.TextGridCell{Rune: r, Style: style})
						}
					} else {
						for _, r := range f {
							row.Cells = append(row.Cells, widget.TextGridCell{Rune: r, Style: valStyle})
						}
					}
				}
			}
		}

		rows = append(rows, row)
	}

	g.Rows = rows
	g.Refresh()
}

func levelToStyle(level string) *widget.CustomTextGridStyle {
	switch level {
	case "INFO":
		return &widget.CustomTextGridStyle{FGColor: greenColor}
	case "DEBUG":
		return &widget.CustomTextGridStyle{FGColor: blueColor}
	case "WARN":
		return &widget.CustomTextGridStyle{FGColor: amberColor}
	case "ERROR":
		return &widget.CustomTextGridStyle{FGColor: redColor}
	case "FATAL":
		return &widget.CustomTextGridStyle{FGColor: whiteBrColor, BGColor: redBgColor}
	}
	return &widget.CustomTextGridStyle{FGColor: fgColor}
}

// ============ Main ============

func main() {
	parseConfig()

	a := app.NewWithID("fakesoc.app")
	a.Settings().SetTheme(fakeSocTheme{})

	w := a.NewWindow("fake-soc")
	w.SetFullScreen(true)
	w.CenterOnScreen()

	state := &appState{
		rng:           rand.New(rand.NewSource(time.Now().UnixNano())),
		scenario:      ScenarioNormal,
		scenarioIdx:   0,
		scenarioStart: time.Now(),
		leftLog:       &strings.Builder{},
		nextDialogAt:  time.Now().Add(15 * time.Second),
		logMax:        80,
	}

	leftGrid := widget.NewTextGrid()
	rightTopGrid := widget.NewTextGrid()
	rightBottomGrid := widget.NewTextGrid()

	// Set background color of each pane
	leftBg := canvas.NewRectangle(bgColor)
	rightTopBg := canvas.NewRectangle(bgColor)
	rightBottomBg := canvas.NewRectangle(bgColor)

	// Wrap each TextGrid in a Scroll container so its MinSize doesn't blow
	// out the HSplit/VSplit dividers when the content is wide / tall.
	leftScroll := container.NewScroll(container.NewPadded(leftGrid))
	rightTopScroll := container.NewScroll(container.NewPadded(rightTopGrid))
	rightBottomScroll := container.NewScroll(container.NewPadded(rightBottomGrid))

	leftPane := container.NewStack(leftBg, leftScroll)
	rightTopPane := container.NewStack(rightTopBg, rightTopScroll)
	rightBottomPane := container.NewStack(rightBottomBg, rightBottomScroll)

	rightSplit := container.NewVSplit(rightTopPane, rightBottomPane)
	rightSplit.SetOffset(0.5)

	main := container.NewHSplit(leftPane, rightSplit)
	main.SetOffset(0.5)

	w.SetContent(main)

	// Esc / Q to quit
	if d, ok := a.Driver().(desktop.Driver); ok {
		_ = d
	}
	w.Canvas().SetOnTypedKey(func(k *fyne.KeyEvent) {
		switch k.Name {
		case fyne.KeyEscape, fyne.KeyQ:
			a.Quit()
		}
	})

	// Initial spark size — modest values; will be replaced on first tick
	// once the actual pane size is known.
	state.recalcSparkSize(600, 400, charW, charH)

	// On first frame after the window settles, derive font size from the
	// canvas dimensions if we're in auto mode. Schedule via a one-shot.
	autoSized := false
	autoSize := func() {
		if fontMode != "auto" || autoSized {
			return
		}
		size := w.Canvas().Size()
		if size.Width <= 0 || size.Height <= 0 {
			return
		}
		// Targets: left pane = ~80 half-cells wide, right-bottom = ~28 lines.
		leftW := size.Width / 2
		rightBH := size.Height / 2
		byCols := leftW / 80 / 0.55
		byRows := rightBH / 28 / 1.25
		ns := byCols
		if byRows < ns {
			ns = byRows
		}
		if ns < 12 {
			ns = 12
		}
		if ns > 40 {
			ns = 40
		}
		fontSize = ns
		updateMetrics()
		a.Settings().SetTheme(fakeSocTheme{})
		// Reseed sparkline since char width changed
		size2 := rightTopGrid.Size()
		if size2.Width > 0 && size2.Height > 0 {
			state.recalcSparkSize(size2.Width, size2.Height, charW, charH)
		}
		autoSized = true
	}

	// Step ticker
	go func() {
		stepT := time.NewTicker(180 * time.Millisecond)
		defer stepT.Stop()
		streamT := time.NewTicker(50 * time.Millisecond)
		defer streamT.Stop()
		for {
			select {
			case <-stepT.C:
				state.advanceScenario()
				state.stepSparklines()
				state.stepStats()
				state.stepDialog()
				state.stepLogs()

				// recalc spark size from current grid size
				size := rightTopGrid.Size()
				if size.Width > 0 && size.Height > 0 {
					state.recalcSparkSize(size.Width, size.Height, charW, charH)
				}

				fyne.Do(func() {
					autoSize()
					// Compute max cols for left pane wrap (use scroll size, more reliable than grid)
					leftSize := leftScroll.Size()
					leftCols := int(leftSize.Width/charW) - 2
					if leftCols < 30 {
						leftCols = 80
					}
					setLeftGrid(leftGrid, state.leftLog.String()+state.activeStreamingTail(), state.dialogScenario, leftCols)
					// Auto-scroll left pane to bottom so new chunks are visible
					leftScroll.ScrollToBottom()

					renderRightTopGrid(rightTopGrid, state)

					rbSize := rightBottomScroll.Size()
					rbRows := int(rbSize.Height/charH) - 1
					if rbRows < 1 {
						rbRows = 1
					}
					rbCols := int(rbSize.Width/charW) - 2
					if rbCols < 30 {
						rbCols = 80
					}
					renderRightBottomGrid(rightBottomGrid, state, rbRows, rbCols)
					rightBottomScroll.ScrollToBottom()
				})

			case <-streamT.C:
				state.stepLeft()
			}
		}
	}()

	w.ShowAndRun()
}

func (s *appState) activeStreamingTail() string {
	if s.leftCurrent == nil {
		return ""
	}
	switch s.leftPhase {
	case 2:
		spinner := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		s.leftSpinIdx++
		return spinner[s.leftSpinIdx%len(spinner)] + " (考え中)\n"
	case 3:
		var b strings.Builder
		for i := 0; i < s.leftStreamIdx && i < len(s.leftCurrent.Chunks); i++ {
			b.WriteString(s.leftCurrent.Chunks[i])
		}
		return b.String()
	}
	return ""
}
