package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"
	"github.com/jaypopat/duet/internal/ai"
	"github.com/jaypopat/duet/internal/room"
	"github.com/jaypopat/duet/internal/terminal"
)

// Minimum terminal size for 3-column layout
const (
	MinWidthForSidebar  = 120
	MinHeightForSidebar = 24
)

// AIMessage represents a message in the AI conversation
type AIMessage struct {
	Role   string // "user" or "agent"
	UserID string // Who sent it (for user messages)
	Text   string
	Ts     int64 // Timestamp
}

// this is the root application model
type Model struct {
	screen   Screen
	width    int
	height   int
	username string
	clientID string // Unique client ID

	selected    int                  // 0=create, 1=join, 2+=rejoin rooms
	activeRooms []*room.RoomMetadata // Active rooms user can rejoin

	// create/join screen
	input textinput.Model

	// room screen
	roomID      string
	currentRoom *room.Room
	terminal    *terminal.Terminal
	termContent string // rendered terminal content
	users       []string
	toasts      []toast         // notifications to show
	inputMode   InputMode       // normal/AI chat/sandbox cmd
	cmdInput    textinput.Model // AI/sandbox input
	typingUser  string          // who is currently typing
	typingTime  time.Time       // when they started typing

	// AI sidebar
	aiMessages    []AIMessage // shared AI conversation history
	showAISidebar bool        // whether AI sidebar is visible
	aiScrollPos   int         // scroll position in AI messages
	aiLoading     bool        // whether waiting for AI response
	aiSpinner     spinner.Model

	// event handling ie typing/join/leave
	eventChan chan room.RoomEvent

	// Dependencies
	roomManager *room.Manager
	aiClient    *ai.Client
	renderer    *lipgloss.Renderer
	styles      *Styles // session styles (each terminal may have different colors)
}

type toast struct {
	text    string
	expires time.Time
}

// New creates a new model
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

	// create renderer specific styles
	styles := NewStyles(renderer)

	// initialize spinner with renderer styles
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = styles.accentStyle

	if username == "" {
		username = "guest"
	}

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
		activeRooms:   roomManager.ListActiveRooms(),
		aiMessages:    []AIMessage{},
		showAISidebar: true,
		aiSpinner:     s,
		aiLoading:     false,
		renderer:      renderer,
		styles:        styles,
	}
}

func (m *Model) Init() tea.Cmd {
	m.activeRooms = m.roomManager.ListActiveRooms()
	return tickCmd()
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
		m.cmdInput.Width = m.width - 10

		if m.terminal != nil {
			termW := m.width * 4 / 5
			termH := m.height - 6
			m.terminal.Resize(termW, termH)
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case spinner.TickMsg:
		//spinner tick for AI loading animation
		if m.aiLoading {
			var cmd tea.Cmd
			m.aiSpinner, cmd = m.aiSpinner.Update(msg)
			return m, cmd
		}

	case tickMsg:
		m.expireToasts()
		// clear typing indicator after 2 seconds of no activity
		if m.typingUser != "" && time.Since(m.typingTime) > 2*time.Second {
			m.typingUser = ""
		}
		return m, tickCmd()

	case terminalUpdateMsg:
		// terminal content changed - render and listen for next update
		if m.terminal != nil {
			m.termContent = m.terminal.Render()
		}
		return m, m.waitForTerminalUpdate()

	case roomEventMsg:
		// Handle room event from channel
		switch msg.Event.Type {
		case "join":
			m.users = append(m.users, msg.Event.Username)
			// only show join toast for other users, not for yourself
			if msg.Event.Username != m.username {
				m.addToast(fmt.Sprintf("%s joined", msg.Event.Username))
			}
		case "leave":
			m.users = removeUser(m.users, msg.Event.Username)
			m.addToast(fmt.Sprintf("%s left", msg.Event.Username))
		case "typing":
			m.typingUser = msg.Event.Username
			m.typingTime = time.Now()
		case "ai_messages":
			// Received AI conversation update from another user
			m.parseAIMessagesFromEvent(msg.Event.Data)
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
		m.aiMessages = convertChatMessages(msg.Messages)
		// auto-scroll to bottom
		m.aiScrollPos = len(m.aiMessages)
		// stop loading
		m.aiLoading = false
		// broadcast to other users in the room
		if m.currentRoom != nil {
			m.broadcastAIMessages()
		}
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

	// Ctrl+C only quits from launch screen, in rooms it's forwarded to terminal
	if key == "ctrl+c" && m.screen != ScreenRoom {
		m.cleanup()
		return m, tea.Quit
	}

	switch m.screen {
	case ScreenLaunch:
		maxSelection := 1 + len(m.activeRooms) // 0 = create, 1 = join, 2+ =  joinable rooms
		switch key {
		case "up", "k":
			if m.selected > 0 {
				m.selected--
			}
		case "down", "j":
			if m.selected < maxSelection {
				m.selected++
			}
		case "c", "C":
			return m, gotoScreen(ScreenCreate)
		case "J":
			return m, gotoScreen(ScreenJoin)
		case "enter":
			switch m.selected {
			case 0:
				return m, gotoScreen(ScreenCreate)
			case 1:
				return m, gotoScreen(ScreenJoin)
			default:
				// Rejoin selected room
				roomIdx := m.selected - 2
				if roomIdx >= 0 && roomIdx < len(m.activeRooms) {
					roomMeta := m.activeRooms[roomIdx]
					return m, m.rejoinRoom(roomMeta.ID)
				}
			}
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
			m.addToast("Entered room")
			// Start terminal and event listening
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
	// handle input modes first
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

	// check for mode switches
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
		// toggle AI sidebar
		m.showAISidebar = !m.showAISidebar
		return m, nil
	case "ctrl+l":
		// leave room
		m.cleanup()
		m.activeRooms = m.roomManager.ListActiveRooms()
		return m, gotoScreen(ScreenLaunch)
	}

	// forward all other keys to terminal
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

func (m *Model) rejoinRoom(roomID string) tea.Cmd {
	return func() tea.Msg {
		r, err := m.roomManager.GetRoom(roomID)
		if err != nil {
			return ErrorMsg{err}
		}
		m.registerAsClient(r, false)

		return RoomJoinedMsg{RoomID: roomID, Room: r}
	}
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
	if m.currentRoom != nil {
		m.currentRoom.RemoveClient(m.clientID)

		// only close terminal if we're the last client
		if m.currentRoom.ClientCount() == 0 && m.terminal != nil {
			m.terminal.Close()
			m.currentRoom.Terminal = nil
		}

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
			m.termContent = m.terminal.Render()
			return terminalUpdateMsg{} // start listening for updates
		}

		// Calculate terminal size
		termW := m.width * 4 / 5
		termH := m.height - 6
		if termW < 40 {
			termW = 80
		}
		if termH < 10 {
			termH = 24
		}

		m.terminal = terminal.New(termW, termH)

		if err := m.terminal.Start(); err != nil {
			return ErrorMsg{err}
		}

		if m.currentRoom != nil {
			m.currentRoom.Terminal = m.terminal
		}

		m.termContent = m.terminal.Render()
		return terminalUpdateMsg{} // start listening for updates
	}
}

// we listens for terminal updates via channel
func (m *Model) waitForTerminalUpdate() tea.Cmd {
	if m.terminal == nil || m.terminal.Updates == nil {
		return nil
	}
	return func() tea.Msg {
		_, ok := <-m.terminal.Updates
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

// AI sidebar helpers

func convertChatMessages(msgs []AIMessage) []AIMessage {
	return msgs
}

func (m *Model) broadcastAIMessages() {
	if m.currentRoom == nil {
		return
	}
	// Serialize messages as JSON for transport
	data, err := m.serializeAIMessages()
	if err != nil {
		return
	}
	m.currentRoom.BroadcastEvent(room.RoomEvent{
		Type:     "ai_messages",
		Username: m.username,
		Data:     data,
	}, m.clientID)
}

func (m *Model) serializeAIMessages() (string, error) {
	data, err := json.Marshal(m.aiMessages)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (m *Model) parseAIMessagesFromEvent(data string) {
	if data == "" {
		return
	}
	var msgs []AIMessage
	if err := json.Unmarshal([]byte(data), &msgs); err != nil {
		return
	}
	m.aiMessages = msgs
	m.aiScrollPos = len(m.aiMessages)
}
