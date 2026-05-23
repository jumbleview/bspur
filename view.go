package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// -----------------------------------------------------------------------
// View entry point
// -----------------------------------------------------------------------

func (m Model) View() string {
	switch m.currentFocus {
	case focusPasswordEnter:
		return m.viewOverlay(m.viewPasswordEnter())
	case focusPasswordNew:
		return m.viewOverlay(m.viewPasswordNew())
	case focusEdit:
		return m.viewOverlay(m.viewEditForm())
	case focusMode:
		return m.viewOverlay(m.viewModePicker())
	case focusConfirmWWW, focusConfirmDelete, focusConfirmSave, focusConfirmExit, focusConfirmGit:
		return m.viewOverlay(m.viewConfirm())
	case focusGitResult:
		return m.viewOverlay(m.viewGitResult())
	default:
		return m.viewMain()
	}
}

// -----------------------------------------------------------------------
// Main layout: top menu + table
// -----------------------------------------------------------------------

func (m Model) viewMain() string {
	menu := m.viewTopMenu()
	table := m.viewTable()
	return lipgloss.JoinVertical(lipgloss.Left, menu, table)
}

// -----------------------------------------------------------------------
// Top menu
// -----------------------------------------------------------------------

func (m Model) viewTopMenu() string {
	btns := m.menuButtons()
	var parts []string
	for i, label := range btns {
		s := lipgloss.NewStyle().
			Foreground(m.AccentFg).
			Background(m.MainBg).
			Padding(0, 1)
		if m.currentFocus == focusMenu && i == m.menuIndex {
			s = s.Reverse(true)
		}
		parts = append(parts, s.Render("[ "+label+" ]"))
	}
	bar := lipgloss.JoinHorizontal(lipgloss.Top, parts...)
	return lipgloss.NewStyle().
		Background(m.MainBg).
		Width(m.termWidth).
		Render(bar)
}

// -----------------------------------------------------------------------
// Column width computation — shared by viewTable and mouse handler
// -----------------------------------------------------------------------

// computeColWidths returns the display width for every column.
// col 0 = row number, col 1 = Record Name, col 2+ = value columns.
func (m Model) computeColWidths() []int {
	numDataCols := m.numDataCols()
	totalCols := numDataCols + 2

	colWidths := make([]int, totalCols)

	// Seed from header labels
	colWidths[0] = len(fmt.Sprintf("%d Rows", len(m.records)))
	if colWidths[0] < 5 {
		colWidths[0] = 5
	}
	colWidths[1] = len("Record Name")
	for c := 2; c < totalCols; c++ {
		colWidths[c] = len(fmt.Sprintf(FmtFieldTitle, c-1))
	}

	// Expand to fit actual content (add 2 chars of side padding)
	for _, key := range m.keys {
		if w := len(key) + 2; w > colWidths[1] {
			colWidths[1] = w
		}
		values := m.records[key]
		for c := 2; c < totalCols; c++ {
			idx := c - 2
			if idx < len(values) && len(values[idx]) > 0 {
				var w int
				if m.visibility[key] == "h" {
					w = len(hiddenText)
				} else {
					w = len(values[idx]) + 2
				}
				if w > colWidths[c] {
					colWidths[c] = w
				}
			}
		}
	}

	// Cap to prevent runaway widths
	const maxNameWidth = 28
	const maxValWidth = 36
	if colWidths[1] > maxNameWidth {
		colWidths[1] = maxNameWidth
	}
	for c := 2; c < totalCols; c++ {
		if colWidths[c] > maxValWidth {
			colWidths[c] = maxValWidth
		}
	}

	return colWidths
}

// -----------------------------------------------------------------------
// Table
// -----------------------------------------------------------------------

func (m Model) viewTable() string {
	numDataCols := m.numDataCols()
	totalCols := numDataCols + 2 // col0=row#, col1=Name, col2..=values

	colWidths := m.computeColWidths()

	// --- Styling ---
	borderStyle := lipgloss.NewStyle().Foreground(m.AccentFg)
	bordChar := borderStyle.Render("│")
	hdrFg := m.AccentFg
	cellFg := m.MainFg
	cellBg := m.MainBg

	// renderRow renders a single table row.
	// selCol >= 0 highlights that column index (0-based into cells slice).
	// selCol == -1 means no cell is highlighted.
	renderRow := func(cells []string, widths []int, fg, bg lipgloss.Color, selCol int) string {
		var cols []string
		for i, cell := range cells {
			text := centerPad(cell, widths[i])
			var style lipgloss.Style
			if i == selCol {
				// Highlight only the active cell: TrackingFg + reverse
				style = lipgloss.NewStyle().
					Foreground(m.TrackingFg).
					Background(bg).
					Bold(true).
					Reverse(true)
			} else {
				style = lipgloss.NewStyle().Foreground(fg).Background(bg)
			}
			cols = append(cols, style.Render(text))
		}
		return bordChar + strings.Join(cols, bordChar) + bordChar
	}

	// Build a horizontal border line (top / separator / bottom).
	makeBorderLine := func(left, mid, right string) string {
		var sb strings.Builder
		sb.WriteString(left)
		for c, w := range colWidths {
			sb.WriteString(strings.Repeat("─", w))
			if c < totalCols-1 {
				sb.WriteString(mid)
			}
		}
		sb.WriteString(right)
		return borderStyle.Render(sb.String())
	}
	topLine := makeBorderLine("┌", "┬", "┐")
	sepLine := makeBorderLine("├", "┼", "┤")
	botLine := makeBorderLine("└", "┴", "┘")

	// Header row (no selection highlight)
	hdrCells := make([]string, totalCols)
	hdrCells[0] = fmt.Sprintf("%d Rows", len(m.records))
	hdrCells[1] = "Record Name"
	for c := 2; c < totalCols; c++ {
		hdrCells[c] = fmt.Sprintf(FmtFieldTitle, c-1)
	}
	hdr := renderRow(hdrCells, colWidths, hdrFg, m.MainBg, -1)

	var lines []string
	lines = append(lines, topLine)
	lines = append(lines, hdr)
	lines = append(lines, sepLine)

	vis := m.visibleRows()
	end := m.tableOffset + vis
	if end > len(m.keys) {
		end = len(m.keys)
	}

	for r := m.tableOffset; r < end; r++ {
		dataRow := r + 1 // 1-based
		key := m.keys[r]
		values := m.records[key]
		rowVis := m.visibility[key]

		cells := make([]string, totalCols)
		cells[0] = fmt.Sprintf("%d", dataRow)
		cells[1] = key

		for c := 2; c < totalCols; c++ {
			idx := c - 2
			val := ""
			if idx < len(values) {
				val = values[idx]
			}
			if rowVis == "h" && len(val) > 0 {
				// Reveal the active cell's real value in the appropriate modes
				if (m.mode == ModeVisibleSelect ||
					(m.mode == ModeVisibleEnter && m.currentFocus == focusTable)) &&
					dataRow == m.activeRow && c == m.activeColumn {
					// show real value
				} else {
					val = hiddenText
				}
			}
			cells[c] = val
		}

		// selCol: highlight the active cell only when the table has focus.
		// m.activeColumn maps directly to the cells index:
		//   activeColumn=1 (Name)  → cells[1]
		//   activeColumn=2 (val 1) → cells[2], etc.
		selCol := -1
		if m.currentFocus == focusTable && dataRow == m.activeRow {
			selCol = m.activeColumn
		}

		lines = append(lines, renderRow(cells, colWidths, cellFg, cellBg, selCol))
	}

	// Pad remaining rows so the table height stays constant
	emptyRow := bordChar
	for c, w := range colWidths {
		emptyRow += strings.Repeat(" ", w)
		if c < totalCols-1 {
			emptyRow += bordChar
		}
	}
	emptyRow += bordChar
	for len(lines)-3 < vis { // -3 for topLine / hdr / sepLine
		lines = append(lines, emptyRow)
	}

	lines = append(lines, botLine)

	tbl := strings.Join(lines, "\n")
	return lipgloss.NewStyle().Foreground(m.MainFg).Background(m.MainBg).Render(tbl)
}

// centerPad centres s within a field of the given width.
// Uses lipgloss.Width so ANSI escape codes don't distort the count.
func centerPad(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		// Truncate by bytes as a best-effort fallback for plain ASCII content
		if len(s) >= width {
			return s[:width]
		}
		return s
	}
	total := width - w
	left := total / 2
	right := total - left
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", right)
}

// -----------------------------------------------------------------------
// Overlay wrapper — centres a box on top of the dimmed main view
// -----------------------------------------------------------------------

func (m Model) viewOverlay(box string) string {
	bg := m.viewMain()
	lines := strings.Split(bg, "\n")
	boxLines := strings.Split(box, "\n")

	boxH := len(boxLines)
	boxW := 0
	for _, l := range boxLines {
		if lipgloss.Width(l) > boxW {
			boxW = lipgloss.Width(l)
		}
	}

	startY := (m.termHeight - boxH) / 2
	startX := (m.termWidth - boxW) / 2
	if startY < 0 {
		startY = 0
	}
	if startX < 0 {
		startX = 0
	}

	for i, bl := range boxLines {
		y := startY + i
		if y >= len(lines) {
			lines = append(lines, "")
		}
		line := lines[y]
		for lipgloss.Width(line) < startX {
			line += " "
		}
		before := truncateToWidth(line, startX)
		lines[y] = before + bl
	}

	return strings.Join(lines, "\n")
}

func truncateToWidth(s string, width int) string {
	result := ""
	w := 0
	for _, r := range []rune(s) {
		if w >= width {
			break
		}
		result += string(r)
		w++
	}
	for w < width {
		result += " "
		w++
	}
	return result
}

// -----------------------------------------------------------------------
// Password — enter existing password overlay
// -----------------------------------------------------------------------

func (m Model) viewPasswordEnter() string {
	title := m.pwdTitle
	fieldVal := strings.Repeat("*", len(m.pwdInputs[0]))

	fStyle := lipgloss.NewStyle().Foreground(m.FormFg).Background(m.FormInputBg).Width(24)
	lStyle := lipgloss.NewStyle().Foreground(m.FormFg)

	// Highlight the field when cursor is on it
	fActive := fStyle
	if m.pwdCursor == 0 {
		fActive = fStyle.Reverse(true)
	}
	field := lStyle.Render("Password:") + " " + fActive.Render(fieldVal+"_")

	submitStyle := lipgloss.NewStyle().Foreground(m.FormFg).Background(m.FormBg).Padding(0, 1)
	cancelStyle := submitStyle
	if m.pwdCursor == 1 {
		submitStyle = submitStyle.Reverse(true)
	}
	if m.pwdCursor == 2 {
		cancelStyle = cancelStyle.Reverse(true)
	}
	btns := lipgloss.JoinHorizontal(lipgloss.Top,
		submitStyle.Render("[ Submit ]"), " ", cancelStyle.Render("[ Cancel ]"))

	content := lipgloss.JoinVertical(lipgloss.Left,
		"",
		" "+field,
		"",
		" "+btns,
		"",
	)
	return m.boxStyle(40, title).Render(content)
}

// -----------------------------------------------------------------------
// Password — new / change password overlay
// -----------------------------------------------------------------------

func (m Model) viewPasswordNew() string {
	numFields := 2
	if m.needOldPassword {
		numFields = 3
	}

	labels := []string{"New Password:", "New Password:"}
	if m.needOldPassword {
		labels = []string{"Old Password:", "New Password:", "New Password:"}
	}
	inputs := m.pwdInputs[:numFields]

	fStyle := lipgloss.NewStyle().Foreground(m.FormFg).Background(m.FormInputBg).Width(20)
	lStyle := lipgloss.NewStyle().Foreground(m.FormFg)

	var fieldLines []string
	for i, lbl := range labels {
		val := strings.Repeat("*", len(inputs[i]))
		fs := fStyle
		if m.pwdCursor == i {
			fs = fs.Reverse(true)
		}
		fieldLines = append(fieldLines, " "+lStyle.Render(lbl)+" "+fs.Render(val+"_"))
	}

	submitStyle := lipgloss.NewStyle().Foreground(m.FormFg).Background(m.FormBg).Padding(0, 1)
	cancelStyle := submitStyle
	if m.pwdCursor == numFields {
		submitStyle = submitStyle.Reverse(true)
	}
	if m.pwdCursor == numFields+1 {
		cancelStyle = cancelStyle.Reverse(true)
	}
	btns := lipgloss.JoinHorizontal(lipgloss.Top,
		submitStyle.Render("[ Submit ]"), " ", cancelStyle.Render("[ Cancel ]"))

	lines := append([]string{""}, fieldLines...)
	lines = append(lines, "", " "+btns, "")
	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return m.boxStyle(44, m.pwdTitle).Render(content)
}

// -----------------------------------------------------------------------
// Edit form overlay
// -----------------------------------------------------------------------

func (m Model) viewEditForm() string {
	fStyle := lipgloss.NewStyle().Foreground(m.FormFg).Background(m.FormInputBg).Width(28)
	lStyle := lipgloss.NewStyle().Foreground(m.FormFg)

	renderField := func(label, val string, cursor bool, hidden bool) string {
		display := val
		if hidden {
			display = strings.Repeat("*", len(val))
		}
		fs := fStyle
		if cursor {
			fs = fs.Reverse(true)
		}
		return " " + lStyle.Render(label) + " " + fs.Render(display+"_")
	}

	var lines []string
	lines = append(lines, "")
	lines = append(lines, renderField("Record Name", m.editKey, m.editCursor == 0, false))

	isHidden := m.editVisibility == "h"
	for i, val := range m.editValues {
		label := fmt.Sprintf("Field %d ", i+1)
		if i == len(m.editValues)-1 {
			label = "+ "
		}
		lines = append(lines, renderField(label, val, m.editCursor == i+1, isHidden))
	}

	numFields := m.editNumFields()
	btnLabels := []string{"Submit", "Hide/Reveal", "Clear", "Cancel"}
	var btns []string
	for i, bl := range btnLabels {
		s := lipgloss.NewStyle().Foreground(m.FormFg).Background(m.FormBg).Padding(0, 1)
		if m.editCursor == numFields+i {
			s = s.Reverse(true)
		}
		btns = append(btns, s.Render("[ "+bl+" ]"))
	}
	lines = append(lines, "")
	lines = append(lines, " "+lipgloss.JoinHorizontal(lipgloss.Top, btns...))
	lines = append(lines, "")

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	h := len(lines) + 2
	return m.boxStyle(56, " Edit Record ").Height(h).Render(content)
}

// -----------------------------------------------------------------------
// Mode picker overlay
// -----------------------------------------------------------------------

func (m Model) viewModePicker() string {
	var lines []string
	lines = append(lines, "")
	for i, mode := range modeSet {
		s := lipgloss.NewStyle().
			Foreground(m.FormFg).
			Background(m.FormBg).
			Width(len(ModeClipSelect) + 2).
			Align(lipgloss.Center)
		if i == m.modeCursor {
			s = s.Reverse(true)
		}
		lines = append(lines, " "+s.Render(mode))
	}
	lines = append(lines, "")
	content := lipgloss.JoinVertical(lipgloss.Right, lines...)
	return m.boxStyle(len(ModeClipSelect)+6, " Mode: ").Render(content)
}

// -----------------------------------------------------------------------
// Confirm overlay
// -----------------------------------------------------------------------

func (m Model) viewConfirm() string {
	msgStyle := lipgloss.NewStyle().Foreground(m.FormFg)
	btnStyle := lipgloss.NewStyle().Foreground(m.FormFg).Background(m.FormBg).Padding(0, 1)

	okBtn := btnStyle.Render("[ " + m.confirmOK + " ]")
	var btns string
	if m.confirmCancel != "" {
		cancelBtn := btnStyle.Render("[ " + m.confirmCancel + " ]")
		btns = lipgloss.JoinHorizontal(lipgloss.Top, okBtn, " ", cancelBtn)
	} else {
		btns = okBtn
	}

	msgLines := strings.Split(m.confirmText, "\n")
	var lines []string
	lines = append(lines, "")
	for _, ml := range msgLines {
		lines = append(lines, " "+msgStyle.Render(ml))
	}
	lines = append(lines, "")
	lines = append(lines, " "+btns)
	lines = append(lines, "")

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	w := 36
	for _, l := range msgLines {
		if len(l)+6 > w {
			w = len(l) + 6
		}
	}
	return m.boxStyle(w, "").Render(content)
}

// -----------------------------------------------------------------------
// Git result overlay
// -----------------------------------------------------------------------

func (m Model) viewGitResult() string {
	msgStyle := lipgloss.NewStyle().Foreground(m.FormFg)
	btnStyle := lipgloss.NewStyle().Foreground(m.FormFg).Background(m.FormBg).Padding(0, 1)

	content := lipgloss.JoinVertical(lipgloss.Left,
		"",
		" "+msgStyle.Render(m.gitResultText),
		"",
		" "+btnStyle.Render("[ OK ]"),
		"",
	)
	return m.boxStyle(len(m.gitResultText)+8, " Git ").Render(content)
}

// -----------------------------------------------------------------------
// Box style helper
// -----------------------------------------------------------------------

func (m Model) boxStyle(width int, _ string) lipgloss.Style {
	return lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(m.AccentFg).
		Background(m.FormBg).
		Foreground(m.FormFg).
		Width(width)
}
