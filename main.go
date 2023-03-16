package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/avast/retry-go"
	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wrap"
	"github.com/sashabaranov/go-openai"
)

var (
	senderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))
	botStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	errorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
)

var (
	debug            = os.Getenv("DEBUG") == "1"
	endpoint         string
	maxConversations uint
)

const defaultPrompt = "You are ChatGPT, a large language model trained by OpenAI. Answer as concisely as possible."

type (
	errMsg         error
	deltaAnswerMsg string
)

func main() {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("Missing OPENAI_API_KEY environment variable, you can find or create your API key here: https://platform.openai.com/account/api-keys")
	}
	endpoint := os.Getenv("OPENAI_API_ENDPOINT")
	flag.UintVar(&maxConversations, "m", 6, "max conversation limit")
	flag.Parse()

	bot := newChatGPT(apiKey, endpoint)
	history := newHistory(int(maxConversations), defaultPrompt)
	p := tea.NewProgram(
		initialModel(bot, history),
		// enable mouse motion will make text not able to select
		// tea.WithMouseCellMotion(),
		// tea.WithAltScreen(),
	)
	if debug {
		f, _ := tea.LogToFile("chatgpt.log", "")
		defer func() { _ = f.Close() }()
	} else {
		log.SetOutput(io.Discard)
	}

	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
}

type Role string

const (
	System    Role = "system"
	User      Role = "user"
	Assistant Role = "assistant"
)

type Conversation struct {
	Question string
	Answer   string
}

type History struct {
	Limit     int
	Prompt    string
	Forgotten []Conversation
	Context   []Conversation
	Pending   *Conversation
	renderer  *glamour.TermRenderer
}

func newHistory(limit int, prompt string) *History {
	renderer, _ := glamour.NewTermRenderer(
		glamour.WithEnvironmentConfig(),
		glamour.WithWordWrap(0), // we do hard-wrapping ourselves
	)
	return &History{
		Limit:    limit,
		Prompt:   prompt,
		renderer: renderer,
	}
}

func (h *History) AddQuestion(q string) {
	h.Pending = &Conversation{Question: q}
}

func (h *History) UpdatePending(ans string, done bool) {
	if h.Pending == nil {
		return
	}
	h.Pending.Answer += ans
	if done {
		h.Context = append(h.Context, *h.Pending)
		if len(h.Context) > h.Limit {
			h.Forgotten = append(h.Forgotten, h.Context[0])
			h.Context = h.Context[1:]
		}
		h.Pending = nil
	}
}

func (h *History) Clear() {
	h.Forgotten = h.Forgotten[:0]
	h.Context = h.Context[:0]
	h.Pending = nil
}

func (h *History) GetContext() []openai.ChatCompletionMessage {
	messages := make([]openai.ChatCompletionMessage, 0, 2*len(h.Context)+2)
	messages = append(
		messages, openai.ChatCompletionMessage{
			Role:    string(System),
			Content: h.Prompt,
		},
	)
	for _, c := range h.Context {
		messages = append(
			messages, openai.ChatCompletionMessage{
				Role:    string(User),
				Content: c.Question,
			},
		)
		messages = append(
			messages, openai.ChatCompletionMessage{
				Role:    string(Assistant),
				Content: c.Answer,
			},
		)
	}
	if h.Pending != nil {
		messages = append(
			messages, openai.ChatCompletionMessage{
				Role:    string(User),
				Content: h.Pending.Question,
			},
		)
	}
	return messages
}

func (h *History) PendingAnswer() string {
	if h.Pending == nil {
		return ""
	}
	return h.Pending.Answer
}

func (h *History) LastAnswer() string {
	if len(h.Context) == 0 {
		return ""
	}
	return h.Context[len(h.Context)-1].Answer
}

func (h *History) View(maxWidth int) string {
	var sb strings.Builder
	renderYou := func(content string) {
		sb.WriteString(senderStyle.Render("You: "))
		content = wrap.String(content, maxWidth-5)
		content, _ = h.renderer.Render(content)
		sb.WriteString(ensureTrailingNewline(content))
	}
	renderBot := func(content string) {
		if content == "" {
			return
		}
		sb.WriteString(botStyle.Render("ChatGPT: "))
		content = wrap.String(content, maxWidth-5)
		content, _ = h.renderer.Render(content)
		sb.WriteString(ensureTrailingNewline(content))
	}
	for _, m := range h.Forgotten {
		renderYou(m.Question)
		renderBot(m.Answer)
	}
	if len(h.Forgotten) > 0 {
		// TODO add a separator to indicate the previous messages are forgotten
	}
	for _, m := range h.Context {
		renderYou(m.Question)
		renderBot(m.Answer)
	}
	if h.Pending != nil {
		renderYou(h.Pending.Question)
		renderBot(h.Pending.Answer)
	}
	return sb.String()
}

type ChatGPT struct {
	client    *openai.Client
	stream    *openai.ChatCompletionStream
	answering bool
}

func newChatGPT(apiKey string, baseURI string) *ChatGPT {
	config := openai.DefaultConfig(apiKey)
	if baseURI != "" {
		config.BaseURL = baseURI
	}
	client := openai.NewClientWithConfig(config)
	return &ChatGPT{client: client}
}

func (c *ChatGPT) send(messages []openai.ChatCompletionMessage) tea.Cmd {
	return func() tea.Msg {
		var content string
		err := retry.Do(
			func() error {
				stream, err := c.client.CreateChatCompletionStream(
					context.Background(),
					openai.ChatCompletionRequest{
						Model:       openai.GPT3Dot5Turbo,
						Messages:    messages,
						MaxTokens:   1000,
						Temperature: 0,
						N:           1,
					},
				)
				c.answering = true
				c.stream = stream
				if err != nil {
					return errMsg(err)
				}
				resp, err := stream.Recv()
				if err != nil {
					return err
				}
				content = resp.Choices[0].Delta.Content
				return nil
			},
			retry.Attempts(3),
			retry.LastErrorOnly(true),
		)
		if err != nil {
			return errMsg(err)
		}
		return deltaAnswerMsg(content)
	}
}

func (c *ChatGPT) recv() tea.Cmd {
	return func() tea.Msg {
		resp, err := c.stream.Recv()
		if err != nil {
			return errMsg(err)
		}
		content := resp.Choices[0].Delta.Content
		return deltaAnswerMsg(content)
	}
}

func (c *ChatGPT) done() {
	if c.stream != nil {
		c.stream.Close()
	}
	c.stream = nil
	c.answering = false
}

type keyMap struct {
	keyMode
	Help         key.Binding
	Clear        key.Binding
	Quit         key.Binding
	Copy         key.Binding
	ViewPortKeys viewport.KeyMap
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Help, k.Submit, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Quit, k.Submit, k.Clear, k.Switch, k.Copy},
		{k.ViewPortKeys.Up, k.ViewPortKeys.Down, k.ViewPortKeys.PageUp, k.ViewPortKeys.PageDown},
	}
}

type keyMode struct {
	Name    string
	Switch  key.Binding
	Submit  key.Binding
	NewLine key.Binding
}

var (
	SingleLine = keyMode{
		Name:    "SingleLine",
		Switch:  key.NewBinding(key.WithKeys("ctrl+t"), key.WithHelp("ctrl+t", "multiline mode")),
		Submit:  key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "submit")),
		NewLine: key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("ctrl+d", "insert new line")),
	}
	MultiLine = keyMode{
		Name:    "MultiLine",
		Switch:  key.NewBinding(key.WithKeys("ctrl+t"), key.WithHelp("ctrl+t", "single line mode")),
		Submit:  key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("ctrl+d", "submit")),
		NewLine: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "insert new line")),
	}
)

func defaultKeyMap() keyMap {
	return keyMap{
		keyMode: SingleLine,
		Help:    key.NewBinding(key.WithKeys("ctrl+h"), key.WithHelp("ctrl+h", "show help")),
		Clear:   key.NewBinding(key.WithKeys("ctrl+r"), key.WithHelp("ctrl+r", "restart the chat")),
		Quit:    key.NewBinding(key.WithKeys("esc", "ctrl+c"), key.WithHelp("esc", "quit")),
		Copy:    key.NewBinding(key.WithKeys("ctrl+y"), key.WithHelp("ctrl+y", "copy last answer")),
		ViewPortKeys: viewport.KeyMap{
			PageDown: key.NewBinding(
				key.WithKeys("pgdown"),
				key.WithHelp("pgdn", "page down"),
			),
			PageUp: key.NewBinding(
				key.WithKeys("pgup"),
				key.WithHelp("pgup", "page up"),
			),
			HalfPageUp: key.NewBinding(
				key.WithKeys("ctrl+u"),
				key.WithHelp("ctrl+u", "½ page up"),
			),
			HalfPageDown: key.NewBinding(
				key.WithKeys("ctrl+d"),
				key.WithHelp("ctrl+d", "½ page down"),
			),
			Up: key.NewBinding(
				key.WithKeys("up"),
				key.WithHelp("↑", "up"),
			),
			Down: key.NewBinding(
				key.WithKeys("down"),
				key.WithHelp("↓", "down"),
			),
		},
	}
}

type model struct {
	viewport viewport.Model
	textarea textarea.Model
	help     help.Model
	err      error
	chatgpt  *ChatGPT
	history  *History
	keymap   keyMap
	width    int
	height   int
}

func initialModel(chatgpt *ChatGPT, history *History) model {
	ta := textarea.New()
	ta.Placeholder = "Send a message..."
	ta.Focus()

	ta.Prompt = "┃ "
	ta.CharLimit = -1
	if debug {
		ta.Cursor.SetMode(cursor.CursorStatic)
	}
	ta.SetWidth(50)
	ta.SetHeight(1)

	// Remove cursor line styling
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.ShowLineNumbers = false

	vp := viewport.New(50, 5)

	keymap := defaultKeyMap()
	vp.KeyMap = keymap.ViewPortKeys
	ta.KeyMap.InsertNewline = keymap.keyMode.NewLine
	ta.KeyMap.TransposeCharacterBackward.SetEnabled(false)

	return model{
		textarea: ta,
		viewport: vp,
		help:     help.New(),
		chatgpt:  chatgpt,
		history:  history,
		keymap:   keymap,
	}
}

func (m model) Init() tea.Cmd {
	if !debug { // disable blink when debug
		return textarea.Blink
	}
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)
	log.Printf("msg: %#v", msg)

	m.textarea, cmd = m.textarea.Update(msg)
	cmds = append(cmds, cmd)
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	// TODO auto height for textinput
	// TODO copy without space padding
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.Width = msg.Width
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - m.textarea.Height() - lipgloss.Height(m.bottomLine())
		m.textarea.SetWidth(msg.Width)
		m.viewport.SetContent(m.history.View(m.viewport.Width))
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keymap.Help):
			m.help.ShowAll = !m.help.ShowAll
			m.viewport.Height = m.height - m.textarea.Height() - lipgloss.Height(m.bottomLine())
			m.viewport.SetContent(m.history.View(m.viewport.Width))
		case key.Matches(msg, m.keymap.Submit):
			if m.chatgpt.answering {
				break
			}
			input := strings.TrimSpace(m.textarea.Value())
			if input == "" {
				break
			}
			m.history.AddQuestion(input)
			cmds = append(cmds, m.chatgpt.send(m.history.GetContext()))
			m.viewport.SetContent(m.history.View(m.viewport.Width))
			m.viewport.GotoBottom()
			m.textarea.Reset()
			m.textarea.Blur()
			m.textarea.Placeholder = ""
		case key.Matches(msg, m.keymap.Clear):
			if m.chatgpt.answering {
				break
			}
			m.err = nil
			m.history.Clear()
			m.viewport.SetContent(m.history.View(m.viewport.Width))
		case key.Matches(msg, m.keymap.Switch):
			if m.keymap.Name == "SingleLine" {
				m.keymap.keyMode = MultiLine
				m.textarea.KeyMap.InsertNewline = MultiLine.NewLine
				m.textarea.ShowLineNumbers = true
				m.textarea.SetHeight(2)
				m.viewport.Height--
			} else {
				m.keymap.keyMode = SingleLine
				m.textarea.KeyMap.InsertNewline = SingleLine.NewLine
				m.textarea.ShowLineNumbers = false
				m.textarea.SetHeight(1)
				m.viewport.Height++
			}
			m.viewport.SetContent(m.history.View(m.viewport.Width))
		case key.Matches(msg, m.keymap.Copy):
			if m.chatgpt.answering || m.history.LastAnswer() == "" {
				break
			}
			_ = clipboard.WriteAll(m.history.LastAnswer())
		case key.Matches(msg, m.keymap.Quit):
			return m, tea.Quit
		}
	case deltaAnswerMsg:
		m.history.UpdatePending(string(msg), false)
		cmds = append(cmds, m.chatgpt.recv())
		m.err = nil
		m.viewport.SetContent(m.history.View(m.viewport.Width))
		m.viewport.GotoBottom()
	case errMsg:
		// Network problem or answer completed, can't tell
		if msg == io.EOF {
			if m.history.PendingAnswer() == "" {
				m.err = errors.New("unexpected EOF, please try again")
			}
		} else {
			m.err = msg
		}
		m.history.UpdatePending("", true)
		m.chatgpt.done()
		m.textarea.Placeholder = "Send a message..."
		m.textarea.Focus()
	}

	return m, tea.Batch(cmds...)
}

func (m model) bottomLine() string {
	var bottomLine string
	if m.err != nil {
		bottomLine = errorStyle.Render(fmt.Sprintf("error: %v", m.err))
	}
	if bottomLine == "" {
		bottomLine = m.help.View(m.keymap)
	}
	return lipgloss.NewStyle().PaddingTop(1).Render(bottomLine)
}

func (m model) View() string {
	return lipgloss.JoinVertical(
		lipgloss.Left,
		m.viewport.View(),
		m.textarea.View(),
		m.bottomLine(),
	)
}

func ensureTrailingNewline(s string) string {
	if !strings.HasSuffix(s, "\n") {
		return s + "\n"
	}
	return s
}
