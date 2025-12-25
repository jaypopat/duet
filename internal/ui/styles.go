package ui

import "github.com/charmbracelet/lipgloss"

const (
	colorAccent  = lipgloss.Color("6") // Cyan (ANSI 6)
	colorDim     = lipgloss.Color("8") // Bright black/gray (ANSI 8)
	colorText    = lipgloss.Color("7") // White (ANSI 7)
	colorBorder  = lipgloss.Color("8") // Bright black/gray (ANSI 8)
	colorToast   = lipgloss.Color("3") // Yellow (ANSI 3)
	colorError   = lipgloss.Color("1") // Red (ANSI 1)
	colorSuccess = lipgloss.Color("2") // Green (ANSI 2)
)

// Styles struct holds renderer-aware styles for a session
type Styles struct {
	baseStyle        lipgloss.Style
	titleStyle       lipgloss.Style
	textStyle        lipgloss.Style
	dimStyle         lipgloss.Style
	accentStyle      lipgloss.Style
	errorStyle       lipgloss.Style
	successStyle     lipgloss.Style
	buttonStyle      lipgloss.Style
	buttonActive     lipgloss.Style
	sidebarStyle     lipgloss.Style
	terminalStyle    lipgloss.Style
	aiSidebarStyle   lipgloss.Style
	helpStyle        lipgloss.Style
	inputPrefixStyle lipgloss.Style
	logoStyle        lipgloss.Style
	inputBoxStyle    lipgloss.Style
	bottomBarStyle   lipgloss.Style
}

// NewStyles creates renderer-aware styles for the given renderer
func NewStyles(renderer *lipgloss.Renderer) *Styles {
	if renderer == nil {
		renderer = lipgloss.DefaultRenderer()
	}

	baseStyle := renderer.NewStyle()

	return &Styles{
		baseStyle: baseStyle,
		titleStyle: baseStyle.
			Foreground(colorAccent).
			Bold(true),
		textStyle: baseStyle.
			Foreground(colorText),
		dimStyle: baseStyle.
			Foreground(colorDim),
		accentStyle: baseStyle.
			Foreground(colorAccent),
		errorStyle: baseStyle.
			Foreground(colorError),
		successStyle: baseStyle.
			Foreground(colorSuccess),
		buttonStyle: baseStyle.
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Padding(0, 3).
			MarginTop(1),
		buttonActive: baseStyle.
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorAccent).
			Foreground(colorAccent).
			Padding(0, 3).
			MarginTop(1),
		sidebarStyle: baseStyle.
			BorderStyle(lipgloss.NormalBorder()).
			BorderRight(true).
			BorderForeground(colorBorder).
			Padding(1),
		terminalStyle: baseStyle.
			Padding(1),
		aiSidebarStyle: baseStyle.
			BorderStyle(lipgloss.NormalBorder()).
			BorderLeft(true).
			BorderForeground(colorBorder).
			Padding(1),
		helpStyle: baseStyle.
			Foreground(colorDim).
			MarginTop(2),
		inputPrefixStyle: baseStyle.
			Foreground(colorAccent).
			Bold(true).
			PaddingLeft(1),
		logoStyle: baseStyle.
			Foreground(colorAccent).
			Bold(true).
			Align(lipgloss.Center),
		inputBoxStyle: baseStyle.
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorAccent).
			Padding(0, 2),
		bottomBarStyle: baseStyle.
			BorderStyle(lipgloss.NormalBorder()).
			BorderTop(true).
			BorderForeground(colorBorder).
			PaddingTop(1).
			Height(1),
	}
}

// ASCII art for landing
var asciiLogo = `
    ██████╗ ██╗   ██╗███████╗████████╗
    ██╔══██╗██║   ██║██╔════╝╚══██╔══╝
    ██║  ██║██║   ██║█████╗     ██║
    ██║  ██║██║   ██║██╔══╝     ██║
    ██████╔╝╚██████╔╝███████╗   ██║
    ╚═════╝  ╚═════╝ ╚══════╝   ╚═╝
       pair programming over ssh
`
