package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"
	"github.com/jaypopat/duet/internal/ai"
	"github.com/jaypopat/duet/internal/room"
	"github.com/jaypopat/duet/internal/terminal"
)

const (
	MinWidthForSidebar  = 120
	MinHeightForSidebar = 24
)

type AIMessage = room.AIMessage

type Model struct {
	screen   Screen
	width    int
	height   int
	username string
	clientID string

	selected int
	input    textinput.Model

	roomID       string
	currentRoom  *room.Room
	terminal     *terminal.Terminal
	termUpdateCh chan struct{}
	termContent  string
	users        []string
	toasts       []toast
	inputMode    InputMode
	cmdInput     textinput.Model
	typingUser   string
	typingTime   time.Time

	showAISidebar    bool
	aiViewport       viewport.Model
	aiLoading        bool
	aiSpinner        spinner.Model
	lastPromptOffset int

	eventChan chan room.RoomEvent

	roomManager *room.Manager
	aiClient    *ai.Client
	renderer    *lipgloss.Renderer
	styles      *Styles
}

type toast struct {
	text    string
	expires time.Time
}

func New(renderer *lipgloss.Renderer, roomManager *room.Manager, workerURL, username string) *Model {
	ti := textinput.New()
	ti.CharLimit = 100
	ti.Width = 40

	cmdInput := textinput.New()
	cmdInput.CharLimit = 500
	cmdInput.Width = 60

	var aiClient *ai.Client
	if workerURL != "" {
		aiClient = ai.NewClient(workerURL)
	}

	styles := NewStyles(renderer)

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = styles.accentStyle

	if username == "" {
		username = "guest"
	}

	aiVP := viewport.New(40, 20)
	aiVP.Style = lipgloss.NewStyle()

	return &Model{
		screen:        ScreenLaunch,
		username:      username,
		clientID:      uuid.New().String(),
		input:         ti,
		cmdInput:      cmdInput,
		users:         []string{},
		toasts:        []toast{},
		inputMode:     ModeNormal,
		roomManager:   roomManager,
		aiClient:      aiClient,
		showAISidebar: true,
		aiViewport:    aiVP,
		aiSpinner:     s,
		aiLoading:     false,
		renderer:      renderer,
		styles:        styles,
	}
}

func (m *Model) Init() tea.Cmd {
	return tickCmd()
}

func (m *Model) roomLayout() (sidebarW, terminalW, aiSidebarW, mainH int) {
	sidebarW = m.width / 6
	if m.showAISidebar {
		aiSidebarW = m.width / 4
		terminalW = m.width - sidebarW - aiSidebarW - 2
	} else {
		aiSidebarW = 0
		terminalW = m.width - sidebarW - 1
	}
	mainH = m.height - 2
	return
}

// aiViewportInnerSize returns the usable content area inside the AI sidebar.
// we account for: border (1), padding (1 each side), header lines (3).
func (m *Model) aiViewportInnerSize(aiW, mainH int) (w, h int) {
	w = aiW - 4
	h = mainH - 6
	if w < 10 {
		w = 10
	}
	if h < 5 {
		h = 5
	}
	return
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg{}
	})
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.cmdInput.Width = m.width - 16

		_, terminalW, aiSidebarW, mainH := m.roomLayout()

		if m.terminal != nil {
			m.terminal.Resize(terminalW, mainH-4)
		}

		if m.showAISidebar && aiSidebarW > 0 {
			vpW, vpH := m.aiViewportInnerSize(aiSidebarW, mainH)
			m.aiViewport.Width = vpW
			m.aiViewport.Height = vpH
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case spinner.TickMsg:
		if m.aiLoading {
			var cmd tea.Cmd
			m.aiSpinner, cmd = m.aiSpinner.Update(msg)
			return m, cmd
		}

	case tickMsg:
		m.expireToasts()
		if m.typingUser != "" && time.Since(m.typingTime) > 2*time.Second {
			m.typingUser = ""
		}
		return m, tickCmd()

	case terminalUpdateMsg:
		if m.terminal != nil {
			m.termContent = m.terminal.Render()
		}
		return m, m.waitForTerminalUpdate()

	case roomEventMsg:
		switch msg.Event.Type {
		case "join":
			m.users = append(m.users, msg.Event.Username)
			if msg.Event.Username != m.username {
				m.addToast(fmt.Sprintf("%s joined", msg.Event.Username))
			}
		case "leave":
			m.users = removeUser(m.users, msg.Event.Username)
			m.addToast(fmt.Sprintf("%s left", msg.Event.Username))
		case "typing":
			m.typingUser = msg.Event.Username
			m.typingTime = time.Now()
		case "ai_sync":
			// Another client updated AI messages - refresh viewport from shared Room
			m.syncAIViewportContent()
			m.scrollToLastPrompt()
		}
		return m, m.listenForRoomEvents()

	case GotoScreenMsg:
		return m.gotoScreen(msg.Screen)

	case RoomCreatedMsg:
		m.roomID = msg.RoomID
		m.currentRoom = msg.Room
		m.screen = ScreenRoomCreated
		m.users = []string{m.username + " (host)"}
		return m, nil

	case RoomJoinedMsg:
		m.roomID = msg.RoomID
		m.currentRoom = msg.Room
		m.screen = ScreenRoom
		m.users = m.getUserList()

		// Sync AI viewport with existing room messages (history for late joiners)
		m.syncAIViewportContent()
		m.aiViewport.GotoBottom() // For history, show the most recent

		// start terminal and event listening
		return m, tea.Batch(
			m.startTerminal(),
			m.listenForRoomEvents(),
		)

	case ToastMsg:
		m.addToast(msg.Text)
		return m, nil

	case ErrorMsg:
		m.addToast("Error: " + msg.Err.Error())
		m.aiLoading = false
		return m, nil

	case AIResponseMsg:
		if m.currentRoom != nil {
			m.currentRoom.SetAIMessages(msg.Messages)
			// Notify other clients to sync their viewport
			m.currentRoom.BroadcastEvent(room.RoomEvent{
				Type: "ai_sync",
			}, m.clientID)
		}
		m.syncAIViewportContent()
		m.scrollToLastPrompt()

		m.aiLoading = false
		return m, nil

	case SandboxResultMsg:
		output := msg.Output
		if output == "" {
			output = "[no output]"
		}
		m.addToast(fmt.Sprintf("$ %s → %s", msg.Cmd, truncate(output, 60)))
		return m, nil
	}

	if m.screen == ScreenCreate || m.screen == ScreenJoin {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	if m.screen == ScreenRoom {
		if m.inputMode != ModeNormal {
			var cmd tea.Cmd
			m.cmdInput, cmd = m.cmdInput.Update(msg)
			return m, cmd
		}
	}

	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if key == "ctrl+c" && m.screen != ScreenRoom {
		m.cleanup()
		return m, tea.Quit
	}

	switch m.screen {
	case ScreenLaunch:
		switch key {
		case "up", "k":
			if m.selected > 0 {
				m.selected--
			}
		case "down", "j":
			if m.selected < 1 {
				m.selected++
			}
		case "c", "C":
			return m, gotoScreen(ScreenCreate)
		case "J":
			return m, gotoScreen(ScreenJoin)
		case "enter":
			if m.selected == 0 {
				return m, gotoScreen(ScreenCreate)
			}
			return m, gotoScreen(ScreenJoin)
		case "q", "esc":
			return m, tea.Quit
		}

	case ScreenCreate:
		switch key {
		case "enter":
			return m, m.createRoom
		case "esc":
			return m, gotoScreen(ScreenLaunch)
		default:
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}

	case ScreenJoin:
		switch key {
		case "enter":
			return m, m.joinRoom
		case "esc":
			return m, gotoScreen(ScreenLaunch)
		default:
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}

	case ScreenRoomCreated:
		switch key {
		case "enter":
			m.screen = ScreenRoom
			return m, tea.Batch(
				m.startTerminal(),
				m.listenForRoomEvents(),
			)
		case "esc":
			m.cleanup()
			return m, gotoScreen(ScreenLaunch)
		}

	case ScreenRoom:
		return m.handleRoomKey(key, msg)
	}

	return m, nil
}

func (m *Model) handleRoomKey(key string, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.inputMode != ModeNormal {
		switch key {
		case "enter":
			return m.submitInput()
		case "esc":
			m.inputMode = ModeNormal
			m.cmdInput.Reset()
			return m, nil
		default:
			var cmd tea.Cmd
			m.cmdInput, cmd = m.cmdInput.Update(msg)
			return m, cmd
		}
	}

	switch key {
	case "ctrl+g":
		if m.aiClient == nil {
			m.addToast("AI not configured (no worker URL)")
			return m, nil
		}
		m.inputMode = ModeAI
		m.cmdInput.Reset()
		m.cmdInput.Placeholder = "Ask the AI..."
		m.cmdInput.Focus()
		return m, textinput.Blink
	case "ctrl+r":
		if m.aiClient == nil {
			m.addToast("Sandbox not configured (no worker URL)")
			return m, nil
		}
		m.inputMode = ModeSandbox
		m.cmdInput.Reset()
		m.cmdInput.Placeholder = "Command to run..."
		m.cmdInput.Focus()
		return m, textinput.Blink
	case "ctrl+a":
		m.showAISidebar = !m.showAISidebar
		return m, nil
	case "ctrl+j":
		if m.showAISidebar {
			m.aiViewport.ScrollDown(3)
		}
		return m, nil
	case "ctrl+k":
		if m.showAISidebar {
			m.aiViewport.ScrollUp(3)
		}
		return m, nil
	case "ctrl+l":
		m.cleanup()
		return m, gotoScreen(ScreenLaunch)
	}

	if m.terminal != nil {
		var data []byte
		switch key {
		case "enter":
			data = []byte("\r")
		case "backspace":
			data = []byte{127}
		case "tab":
			data = []byte("\t")
		case "up":
			data = []byte("\x1b[A")
		case "down":
			data = []byte("\x1b[B")
		case "right":
			data = []byte("\x1b[C")
		case "left":
			data = []byte("\x1b[D")
		case "home":
			data = []byte("\x1b[H")
		case "end":
			data = []byte("\x1b[F")
		case "delete":
			data = []byte("\x1b[3~")
		case "esc":
			data = []byte("\x1b")
		default:
			// Regular characters
			if len(key) == 1 {
				data = []byte(key)
			} else if len(msg.Runes) > 0 {
				data = []byte(string(msg.Runes))
			}
		}

		if len(data) > 0 {
			m.terminal.Write(data)

			// broadcast typing event to other users - debouncing it here as well
			if m.currentRoom != nil && time.Since(m.typingTime) > 500*time.Millisecond {
				m.currentRoom.BroadcastEvent(room.RoomEvent{
					Type:     "typing",
					Username: m.username,
				}, m.clientID)
				m.typingTime = time.Now()
			}
		}
	}

	return m, nil
}

func (m *Model) submitInput() (tea.Model, tea.Cmd) {
	text := m.cmdInput.Value()
	if text == "" {
		m.inputMode = ModeNormal
		return m, nil
	}

	mode := m.inputMode
	m.inputMode = ModeNormal
	m.cmdInput.Reset()

	if mode == ModeAI {
		m.aiLoading = true
		spinnerCmd := func() tea.Msg { return m.aiSpinner.Tick() }
		return m, tea.Batch(spinnerCmd, m.sendAIMessage(text))
	}

	if mode == ModeSandbox {
		m.addToast(fmt.Sprintf("Running: %s", truncate(text, 30)))
		return m, m.execSandboxCmd(text)
	}

	return m, nil
}

func (m *Model) sendAIMessage(text string) tea.Cmd {
	return func() tea.Msg {
		if m.aiClient == nil {
			return ErrorMsg{fmt.Errorf("AI client not configured")}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		resp, err := m.aiClient.SendMessage(ctx, m.roomID, text, m.username)
		if err != nil {
			return ErrorMsg{err}
		}
		var msgs []AIMessage
		for _, m := range resp.Messages {
			msgs = append(msgs, AIMessage{
				Role:   m.Role,
				UserID: m.UserID,
				Text:   m.Text,
				Ts:     m.Ts,
			})
		}

		return AIResponseMsg{Reply: resp.Reply, Messages: msgs}
	}
}

func (m *Model) execSandboxCmd(cmd string) tea.Cmd {
	return func() tea.Msg {
		if m.aiClient == nil {
			return ErrorMsg{fmt.Errorf("AI client not configured")}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		resp, err := m.aiClient.ExecCommand(ctx, m.roomID, cmd)
		if err != nil {
			return ErrorMsg{err}
		}

		output := resp.Result.Stdout
		if output == "" {
			output = resp.Result.Stderr
		}

		return SandboxResultMsg{Output: output, Cmd: cmd}
	}
}

func (m *Model) gotoScreen(s Screen) (tea.Model, tea.Cmd) {
	m.screen = s
	m.inputMode = ModeNormal
	if s == ScreenCreate {
		m.input.Reset()
		m.input.Placeholder = "Room description..."
		m.input.Focus()
		return m, textinput.Blink
	}
	if s == ScreenJoin {
		m.input.Reset()
		m.input.Placeholder = "Room ID..."
		m.input.Focus()
		return m, textinput.Blink
	}
	return m, nil
}

func (m *Model) createRoom() tea.Msg {
	desc := strings.TrimSpace(m.input.Value())
	r, err := m.roomManager.CreateRoom(m.username, desc)
	if err != nil {
		return ErrorMsg{err}
	}
	m.registerAsClient(r, true)

	return RoomCreatedMsg{RoomID: r.ID, Room: r}
}

func (m *Model) joinRoom() tea.Msg {
	id := strings.TrimSpace(m.input.Value())
	r, err := m.roomManager.GetRoom(id)
	if err != nil {
		return ErrorMsg{err}
	}
	m.registerAsClient(r, false)

	return RoomJoinedMsg{RoomID: id, Room: r}
}

func (m *Model) registerAsClient(r *room.Room, isHost bool) {
	m.eventChan = make(chan room.RoomEvent, 10)

	client := &room.Client{
		ID:       m.clientID,
		Username: m.username,
		IsHost:   isHost,
		Events:   m.eventChan,
	}
	r.AddClient(client)
}

func (m *Model) getUserList() []string {
	if m.currentRoom == nil {
		return []string{m.username}
	}

	clients := m.currentRoom.GetClients()
	users := make([]string, 0, len(clients))
	for _, c := range clients {
		name := c.Username
		if c.IsHost {
			name += " (host)"
		}
		if c.Username == m.username {
			name += " (you)"
		}
		users = append(users, name)
	}
	return users
}

func (m *Model) cleanup() {
	if m.terminal != nil && m.termUpdateCh != nil {
		m.terminal.Unsubscribe(m.termUpdateCh)
		m.termUpdateCh = nil
	}

	if m.currentRoom != nil && m.roomID != "" {
		m.roomManager.LeaveRoom(m.roomID, m.clientID)
		m.currentRoom = nil
	}

	m.terminal = nil
	m.termContent = ""
	m.roomID = ""
	m.users = []string{}
}

func (m *Model) startTerminal() tea.Cmd {
	return func() tea.Msg {
		if m.currentRoom != nil && m.currentRoom.Terminal != nil {
			m.terminal = m.currentRoom.Terminal
			m.termUpdateCh = m.terminal.Subscribe()
			m.termContent = m.terminal.Render()
			return terminalUpdateMsg{} // start listening for updates
		}

		_, terminalW, _, mainH := m.roomLayout()
		termH := mainH - 4 // account for header and padding

		if terminalW < 40 {
			terminalW = 80
		}
		if termH < 10 {
			termH = 24
		}

		m.terminal = terminal.New(terminalW, termH)

		if err := m.terminal.Start(); err != nil {
			return ErrorMsg{err}
		}

		if m.currentRoom != nil {
			m.currentRoom.Terminal = m.terminal
		}

		// Subscribe to terminal updates (per-client channel)
		m.termUpdateCh = m.terminal.Subscribe()
		m.termContent = m.terminal.Render()
		return terminalUpdateMsg{} // start listening for updates
	}
}

// listens for terminal updates via per-client subscription
func (m *Model) waitForTerminalUpdate() tea.Cmd {
	if m.terminal == nil || m.termUpdateCh == nil {
		return nil
	}
	return func() tea.Msg {
		_, ok := <-m.termUpdateCh
		if !ok {
			return nil // Channel closed
		}
		return terminalUpdateMsg{}
	}
}

func (m *Model) listenForRoomEvents() tea.Cmd {
	if m.eventChan == nil {
		return nil
	}
	return func() tea.Msg {
		event, ok := <-m.eventChan
		if !ok {
			return nil
		}
		return roomEventMsg{Event: event}
	}
}

func (m *Model) addToast(text string) {
	m.toasts = append(m.toasts, toast{
		text:    text,
		expires: time.Now().Add(1 * time.Second),
	})
	if len(m.toasts) > 3 {
		m.toasts = m.toasts[len(m.toasts)-3:]
	}
}

func (m *Model) expireToasts() {
	now := time.Now()
	var active []toast
	for _, t := range m.toasts {
		if t.expires.After(now) {
			active = append(active, t)
		}
	}
	m.toasts = active
}

func (m *Model) View() string {
	if m.width == 0 {
		return ""
	}
	switch m.screen {
	case ScreenLaunch:
		return m.viewLaunch()
	case ScreenCreate:
		return m.viewCreate()
	case ScreenJoin:
		return m.viewJoin()
	case ScreenRoomCreated:
		return m.viewRoomCreated()
	case ScreenRoom:
		return m.viewRoom()
	}
	return ""
}

// Helpers

func gotoScreen(s Screen) tea.Cmd {
	return func() tea.Msg { return GotoScreenMsg{s} }
}

func removeUser(users []string, name string) []string {
	for i, u := range users {
		if u == name {
			return append(users[:i], users[i+1:]...)
		}
	}
	return users
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

// some helpers for the ai sidebar

// rebuilds the viewport content from Room's AI messages.
func (m *Model) syncAIViewportContent() {
	content, promptOffset := m.buildAIContent(m.aiViewport.Width)
	m.aiViewport.SetContent(content)
	m.lastPromptOffset = promptOffset
}

// scrolls the AI viewport to show the last user prompt
func (m *Model) scrollToLastPrompt() {
	m.aiViewport.SetYOffset(m.lastPromptOffset)
}

// returns AI messages from the current room, or empty slice if no room.
func (m *Model) getAIMessages() []AIMessage {
	if m.currentRoom == nil {
		return nil
	}
	return m.currentRoom.GetAIMessages()
}
