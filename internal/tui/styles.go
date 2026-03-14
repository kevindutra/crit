package tui

import (
	"fmt"
	"image/color"

	"charm.land/lipgloss/v2"
)

func tabBorderWithBottom(left, middle, right string) lipgloss.Border {
	border := lipgloss.RoundedBorder()
	border.BottomLeft = left
	border.Bottom = middle
	border.BottomRight = right
	return border
}

var (
	activeTabBorder   = tabBorderWithBottom("┘", " ", "└")
	inactiveTabBorder = tabBorderWithBottom("┴", "─", "┴")
)

// ANSI 16 named colors — these adapt to the user's terminal color scheme.
var (
	subtle  = lipgloss.BrightBlack
	accent  = lipgloss.Magenta
	success = lipgloss.Green
	warning = lipgloss.Yellow
	muted   = lipgloss.White // normal white (dimmer than BrightWhite)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.BrightWhite).
			Background(accent).
			Padding(0, 1)

	agentStatusBar = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.BrightWhite).
				Background(lipgloss.Red).
				Padding(0, 1)

	focusedBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(accent)

	blurredBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(subtle)

	commentStyle = lipgloss.NewStyle().
			Foreground(muted).
			PaddingLeft(1)

	commentLineStyle = lipgloss.NewStyle().
				Foreground(subtle)

	footerStyle = lipgloss.NewStyle().
			Foreground(subtle)

	footerKeyStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.BrightWhite)

	modalStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(accent).
			Padding(1, 2).
			Width(60)

	modalTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(accent).
			MarginBottom(1)

	lineNumStyle = lipgloss.NewStyle().
			Foreground(subtle).
			Width(5).
			Align(lipgloss.Right)

	cursorLineNumStyle = lipgloss.NewStyle().
				Foreground(warning).
				Bold(true).
				Width(5).
				Align(lipgloss.Right)

	selectedLineNumStyle = lipgloss.NewStyle().
				Foreground(lipgloss.BrightMagenta).
				Bold(true).
				Width(5).
				Align(lipgloss.Right)

	cursorMarker = lipgloss.NewStyle().
			Foreground(warning).
			Bold(true)

	selectedMarker = lipgloss.NewStyle().
			Foreground(lipgloss.BrightMagenta).
			Bold(true)

	// Inline annotation box styles
	inlineCommentBox = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Blue).
				Foreground(lipgloss.BrightCyan).
				PaddingLeft(1).
				PaddingRight(1)

	inlineLabelComment = lipgloss.NewStyle().
				Foreground(lipgloss.Blue).
				Bold(true)

	// Annotation gutter marker
	annotationGutter = lipgloss.NewStyle().
				Foreground(lipgloss.BrightCyan).
				Bold(true)

	gutterOverlap = lipgloss.NewStyle().
			Foreground(accent).
			Bold(true)

	// Continuation line gutter (for wrapped lines)
	continuationGutter = lipgloss.NewStyle().
				Foreground(subtle).
				Width(5).
				Align(lipgloss.Right).
				Render("↪")

	// Markdown syntax highlighting styles
	mdH1Style = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Magenta)

	mdH2Style = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.BrightMagenta)

	mdH3Style = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Cyan)

	mdH4Style = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Cyan).
			Italic(true)

	mdBoldStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.BrightWhite)

	mdItalicStyle = lipgloss.NewStyle().
			Italic(true).
			Foreground(muted)

	mdCodeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.BrightYellow)

	mdListMarkerStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Magenta).
				Bold(true)

	mdCheckboxOpen = lipgloss.NewStyle().
			Foreground(subtle)

	mdCheckboxDone = lipgloss.NewStyle().
			Foreground(success).
			Bold(true)

	mdCheckboxDoneText = lipgloss.NewStyle().
				Foreground(subtle).
				Strikethrough(true)

	mdBlockquoteBar = lipgloss.NewStyle().
			Foreground(lipgloss.Magenta).
			Bold(true)

	mdBlockquoteStyle = lipgloss.NewStyle().
				Foreground(subtle).
				Italic(true)

	mdHrStyle = lipgloss.NewStyle().
			Foreground(subtle)

	mdLinkStyle = lipgloss.NewStyle().
			Foreground(lipgloss.BrightCyan).
			Underline(true)

	mdTablePipe = lipgloss.NewStyle().
			Foreground(subtle)

	mdTableSepStyle = lipgloss.NewStyle().
			Foreground(subtle)

	mdTableHeaderStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Magenta).
				Bold(true)

	mdTableCellStyle = lipgloss.NewStyle().
			Foreground(muted)

	// Sidebar selected text (bright for contrast against highlight bg)
	sidebarSelectedText = lipgloss.NewStyle().
				Foreground(lipgloss.BrightWhite)

	// Modal button styles
	modalBtnLabel = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.BrightWhite)

	modalBtnHint = lipgloss.NewStyle().
			Italic(true).
			Foreground(subtle)

	modalBtnFocused = lipgloss.NewStyle().
			Reverse(true).
			Padding(0, 1)

	modalBtnNormal = lipgloss.NewStyle().
			Padding(0, 1)

	modalDeleteBtnLabel = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Red)

	modalDeleteBtnFocused = lipgloss.NewStyle().
				Background(lipgloss.Red).
				Foreground(lipgloss.BrightWhite).
				Bold(true).
				Padding(0, 1)

	// Tab bar styles — bordered tabs with open-bottom active tab
	inactiveTabStyle = lipgloss.NewStyle().
				Border(inactiveTabBorder, true).
				BorderForeground(accent).
				Foreground(muted).
				Padding(0, 1)

	activeTabStyle = lipgloss.NewStyle().
			Border(activeTabBorder, true).
			BorderForeground(accent).
			Bold(true).
			Foreground(lipgloss.BrightWhite).
			Padding(0, 1)

	tabSearchPromptStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(warning)

	// Diff gutter markers
	diffAddedGutter = lipgloss.NewStyle().
			Foreground(lipgloss.Green).
			Bold(true)

	diffDeletedGutter = lipgloss.NewStyle().
				Foreground(lipgloss.Red).
				Bold(true)

	diffDeletedLineNum = lipgloss.NewStyle().
				Foreground(lipgloss.Red).
				Width(5).
				Align(lipgloss.Right)

	// Change count in tab labels
	tabChangeCount = lipgloss.NewStyle().
			Foreground(lipgloss.Green)

	// Context box in comment/edit modals
	contextBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(subtle).
			Foreground(lipgloss.BrightWhite)

	// Background-dependent styles — initialized with dark defaults,
	// updated at runtime via tea.BackgroundColorMsg.
	selectedLineBg = lipgloss.NewStyle().
			Faint(true)

	sidebarHighlightBg = lipgloss.NewStyle().
				Reverse(true)

	diffChangedLineBg = lipgloss.NewStyle()
	diffDeletedLineBg = lipgloss.NewStyle()

	visualModeIndicator = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.BrightMagenta).
				Reverse(true).
				Padding(0, 1)
)

// initAdaptiveStyles updates background-dependent styles based on terminal background.
func initAdaptiveStyles(hasDarkBG bool) {
	ld := lipgloss.LightDark(hasDarkBG)

	selectedLineBg = lipgloss.NewStyle().
		Background(ld(lipgloss.Color("#E8E0F8"), lipgloss.Color("#2D2B55")))

	sidebarHighlightBg = lipgloss.NewStyle().
		Background(ld(lipgloss.Color("#D8E8F8"), lipgloss.Color("#1E3A5F"))).
		Foreground(ld(lipgloss.Color("#333333"), lipgloss.Color("#E0E0E0")))

	visualModeIndicator = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.BrightMagenta).
		Background(ld(lipgloss.Color("#E8E0F8"), lipgloss.Color("#2D2B55"))).
		Padding(0, 1)

	mdCodeStyle = lipgloss.NewStyle().
		Foreground(lipgloss.BrightYellow).
		Background(ld(lipgloss.Color("#E8E8E8"), lipgloss.Color("#3a3a3a")))

	diffChangedLineBg = lipgloss.NewStyle().
		Background(ld(lipgloss.Color("#D8F0D8"), lipgloss.Color("#1A3A1A")))

	diffDeletedLineBg = lipgloss.NewStyle().
		Background(ld(lipgloss.Color("#F0D8D8"), lipgloss.Color("#3A1A1A")))
}

// bgToAnsi converts a lipgloss color to a raw ANSI truecolor background escape sequence.
// Returns "" if the color is nil or zero-alpha.
func bgToAnsi(c color.Color) string {
	if c == nil {
		return ""
	}
	r, g, b, a := c.RGBA()
	if a == 0 {
		return ""
	}
	// RGBA() returns 16-bit values; scale to 8-bit.
	return fmt.Sprintf("\033[48;2;%d;%d;%dm", r>>8, g>>8, b>>8)
}
