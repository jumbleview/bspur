package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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
		parts = append(parts, s.Render(label))
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
	colWidths[0] = max(colWidths[0], 5)
	colWidths[1] = len("Record Name")
	for c := 2; c < totalCols; c++ {
		colWidths[c] = len(fmt.Sprintf(FmtFieldTitle, c-1))
	}

	// Expand to fit actual content (add 2 chars of side padding)
	for _, key := range m.keys {
		colWidths[1] = max(colWidths[1], len(key)+2)
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
				colWidths[c] = max(colWidths[c], w)
			}
		}
	}

	// Cap to prevent runaway widths
	const maxNameWidth = 28
	const maxValWidth = 36
	colWidths[1] = min(colWidths[1], maxNameWidth)
	for c := 2; c < totalCols; c++ {
		colWidths[c] = min(colWidths[c], maxValWidth)
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
	end := min(m.tableOffset+vis, len(m.keys))

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
		tableRow := renderRow(cells, colWidths, cellFg, cellBg, selCol)
		lines = append(lines, tableRow)
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

	tableWidth := 0
	for _, row := range lines {
		tableWidth = max(tableWidth, lipgloss.Width(row))
	}

	boxLines := strings.Split(box, "\n")

	boxH := len(boxLines)
	boxW := 0
	for _, l := range boxLines {
		boxW = max(boxW, lipgloss.Width(l))
	}

	startY := max((m.termHeight-boxH)/2, 0)
	startX := max((tableWidth-boxW)/2, 0)

	for i, bl := range boxLines {
		y := startY + i
		if y >= len(lines) {
			lines = append(lines, "")
		}
		line := lines[y]
		for lipgloss.Width(line) < startX {
			line += " "
		}
		before := ansi.Truncate(line, startX, "")
		after := ansi.TruncateLeft(line, startX+lipgloss.Width(bl), "")
		lines[y] = before + bl + after
	}
	return strings.Join(lines, "\n")
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
		cp := min(m.pwdTextCursor, len([]rune(m.pwdInputs[0])))
		fieldVal = strings.Repeat("*", cp) + "_" + strings.Repeat("*", len([]rune(m.pwdInputs[0]))-cp)
	}
	gap := lipgloss.NewStyle().Background(m.FormBg).Render(" ")
	field := lStyle.Render("Password:") + gap + fActive.Render(fieldVal)

	const pwdEnterWidth = 40
	lineStyle := lipgloss.NewStyle().Foreground(m.FormFg).Background(m.FormBg).Width(pwdEnterWidth)

	submitStyle := lipgloss.NewStyle().Foreground(m.FormFg).Background(m.FormBg).Padding(0, 1)
	cancelStyle := submitStyle
	if m.pwdCursor == 1 {
		submitStyle = submitStyle.Reverse(true)
	}
	if m.pwdCursor == 2 {
		cancelStyle = cancelStyle.Reverse(true)
	}
	sep := lipgloss.NewStyle().Background(m.FormBg).Render(" ")
	btns := lipgloss.JoinHorizontal(lipgloss.Top,
		submitStyle.Render("[ Submit ]"), sep, cancelStyle.Render("[ Cancel ]"))

	content := lipgloss.JoinVertical(lipgloss.Left,
		lineStyle.Render(""),
		lineStyle.Render(" "+field),
		lineStyle.Render(""),
		lineStyle.Render(" "+btns),
		lineStyle.Render(""),
	)
	return m.boxStyle(pwdEnterWidth, title).Render(content)
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
		r := []rune(inputs[i])
		val := strings.Repeat("*", len(r))
		fs := fStyle
		if m.pwdCursor == i {
			fs = fs.Reverse(true)
			cp := min(m.pwdTextCursor, len(r))
			val = strings.Repeat("*", cp) + "_" + strings.Repeat("*", len(r)-cp)
		}
		gap := lipgloss.NewStyle().Background(m.FormBg).Render(" ")
		fieldLines = append(fieldLines, " "+lStyle.Render(lbl)+gap+fs.Render(val))
	}

	const pwdNewWidth = 44
	lineStyle := lipgloss.NewStyle().Foreground(m.FormFg).Background(m.FormBg).Width(pwdNewWidth)

	submitStyle := lipgloss.NewStyle().Foreground(m.FormFg).Background(m.FormBg).Padding(0, 1)
	cancelStyle := submitStyle
	if m.pwdCursor == numFields {
		submitStyle = submitStyle.Reverse(true)
	}
	if m.pwdCursor == numFields+1 {
		cancelStyle = cancelStyle.Reverse(true)
	}
	sep := lipgloss.NewStyle().Background(m.FormBg).Render(" ")
	btns := lipgloss.JoinHorizontal(lipgloss.Top,
		submitStyle.Render("[ Submit ]"), sep, cancelStyle.Render("[ Cancel ]"))

	var lines []string
	lines = append(lines, lineStyle.Render(""))
	for _, fl := range fieldLines {
		lines = append(lines, lineStyle.Render(fl))
	}
	lines = append(lines, lineStyle.Render(""), lineStyle.Render(" "+btns), lineStyle.Render(""))
	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return m.boxStyle(pwdNewWidth, m.pwdTitle).Render(content)
}

// -----------------------------------------------------------------------
// Edit form overlay
// -----------------------------------------------------------------------

func (m Model) viewEditForm() string {
	fStyle := lipgloss.NewStyle().Foreground(m.FormFg).Background(m.FormInputBg).Width(28)
	lStyle := lipgloss.NewStyle().Foreground(m.FormFg)

	renderField := func(label, val string, active bool, hidden bool, textCursor int) string {
		r := []rune(val)
		display := string(r)
		if hidden {
			display = strings.Repeat("*", len(r))
		}
		fs := fStyle
		if active {
			fs = fs.Reverse(true)
			cp := min(textCursor, len(r))
			d := []rune(display)
			d = append(d[:cp], append([]rune{'_'}, d[cp:]...)...)
			display = string(d)
		}
		return " " + lStyle.Render(label) + " " + fs.Render(display)
	}

	var lines []string
	lines = append(lines, "")
	lines = append(lines, renderField("Record Name ", m.editKey, m.editCursor == 0, false, m.editTextCursor))

	isHidden := m.editVisibility == "h"
	for i, val := range m.editValues {
		label := fmt.Sprintf("Field   %d   ", i+1)
		if i == len(m.editValues)-1 {
			label = "     +      "
		}
		tc := 0
		if m.editCursor == i+1 {
			tc = m.editTextCursor
		}
		lines = append(lines, renderField(label, val, m.editCursor == i+1, isHidden, tc))
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
		sm := s.Render(mode)
		lines = append(lines, " "+sm)
	}
	lines = append(lines, "")
	content := lipgloss.JoinVertical(lipgloss.Right, lines...)
	rc := m.boxStyle(len(ModeClipSelect)+6, " Mode: ").Render(content)
	return rc
}

// -----------------------------------------------------------------------
// Confirm overlay
// -----------------------------------------------------------------------

func (m Model) viewConfirm() string {
	msgLines := strings.Split(m.confirmText, "\n")
	w := 28
	for _, l := range msgLines {
		if len(l)+4 > w {
			w = len(l) + 4
		}
	}

	// Each text line fills the full box width so no dark gap appears on the right.
	lineStyle := lipgloss.NewStyle().Foreground(m.FormFg).Background(m.FormBg).Width(w)
	btnStyle := lipgloss.NewStyle().Foreground(m.FormFg).Background(m.FormBg).Padding(0, 1)

	okStyle := btnStyle
	cancelStyle := btnStyle
	if m.confirmCursor == 0 {
		okStyle = okStyle.Reverse(true)
	} else {
		cancelStyle = cancelStyle.Reverse(true)
	}
	okBtn := okStyle.Render("[ " + m.confirmOK + " ]")
	var btns string
	if m.confirmCancel != "" {
		cancelBtn := cancelStyle.Render("[ " + m.confirmCancel + " ]")
		sep := lipgloss.NewStyle().Background(m.FormBg).Render(" ")
		btns = lipgloss.JoinHorizontal(lipgloss.Top, okBtn, sep, cancelBtn)
	} else {
		btns = okBtn
	}

	var lines []string
	lines = append(lines, lineStyle.Render(""))
	for _, ml := range msgLines {
		lines = append(lines, lineStyle.Render("  "+ml))
	}
	lines = append(lines, lineStyle.Render(""))
	lines = append(lines, lineStyle.Render("  "+btns))
	lines = append(lines, lineStyle.Render(""))

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return m.boxStyle(w, "").Render(content)
}

// -----------------------------------------------------------------------
// Git result overlay
// -----------------------------------------------------------------------

func (m Model) viewGitResult() string {
	msgStyle := lipgloss.NewStyle().Foreground(m.FormFg).Background(m.FormBg)
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
