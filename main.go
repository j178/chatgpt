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
	"time"

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
	gpt3 "github.com/sashabaranov/go-gpt3"
)

var (
	senderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))
	botStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	errorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
	debug       = os.Getenv("DEBUG") == "1"
)

type (
	errMsg         error
	deltaAnswerMsg string
)

var (
	endpoint         string
	maxConversations int
)

func main() {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("Missing OPENAI_API_KEY environment variable, you can find or create your API key here: https://platform.openai.com/account/api-keys")
	}
	flag.StringVar(&endpoint, "e", "https://api.openai.com/v1", "OpenAI API endpoint")
	flag.IntVar(&maxConversations, "m", 10, "max conversation limit")
	flag.Parse()
	if maxConversations < 2 {
		log.Fatal("conversation limit is too small")
	}

	bot := newChatGPT(apiKey, endpoint)
	p := tea.NewProgram(
		initialModel(bot),
		// enable mouse motion will make text not able to select
		// tea.WithMouseCellMotion(),
		// tea.WithAltScreen(),
	)
	if debug {
		f, _ := tea.LogToFile("chatgpt.log", "")
		defer f.Close()
	} else {
		log.SetOutput(io.Discard)
	}

	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
}

type chatGPT struct {
	client   *gpt3.Client
	messages []gpt3.ChatCompletionMessage
	// stream chat mode does not return token usage
	// totalTokens   int
	stream        *gpt3.ChatCompletionStream
	pendingAnswer []byte
	answering     bool
	renderer      *glamour.TermRenderer
}

func newChatGPT(apiKey string, baseURI string) *chatGPT {
	config := gpt3.DefaultConfig(apiKey)
	if baseURI != "" {
		config.BaseURL = baseURI
	}
	client := gpt3.NewClientWithConfig(config)
	renderer, _ := glamour.NewTermRenderer(
		glamour.WithEnvironmentConfig(),
		glamour.WithWordWrap(0), // we do hard-wrapping ourselves
	)
	return &chatGPT{
		client: client,
		messages: []gpt3.ChatCompletionMessage{
			{
				Role:    "system",
				Content: "You are ChatGPT, a large language model trained by OpenAI. Answer as concisely as possible.",
			},
		},
		renderer: renderer,
	}
}

func (c *chatGPT) send(input string) tea.Cmd {
	if input != "" {
		c.addMessage("user", input)
	}
	return func() tea.Msg {
		var content string
		err := retry.Do(
			func() error {
				stream, err := c.client.CreateChatCompletionStream(
					context.Background(),
					gpt3.ChatCompletionRequest{
						Model:       gpt3.GPT3Dot5Turbo,
						Messages:    c.messages,
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
			retry.Delay(500*time.Millisecond),
			retry.Attempts(3),
			retry.LastErrorOnly(true),
		)
		if err != nil {
			return errMsg(err)
		}
		return deltaAnswerMsg(content)
	}
}

func (c *chatGPT) addMessage(role, text string) {
	m := gpt3.ChatCompletionMessage{
		Role:    role,
		Content: text,
	}
	n := len(c.messages) - 1
	if n >= maxConversations {
		// Shift messages to the left
		copy(c.messages[1:], c.messages[2:])
		c.messages[n] = m
	} else {
		c.messages = append(c.messages, m)
	}
}

func (c *chatGPT) addDeltaAnswer(delta string) tea.Cmd {
	c.pendingAnswer = append(c.pendingAnswer, delta...)
	return func() tea.Msg {
		resp, err := c.stream.Recv()
		if err != nil {
			return errMsg(err)
		}
		content := resp.Choices[0].Delta.Content
		return deltaAnswerMsg(content)
	}
}

func (c *chatGPT) answerDone() {
	if c.stream != nil {
		c.stream.Close()
	}
	c.stream = nil
	c.answering = false
	if len(c.pendingAnswer) > 0 {
		c.addMessage("assistant", string(c.pendingAnswer))
		c.pendingAnswer = c.pendingAnswer[:0]
	}
}

func (c *chatGPT) clearAll() {
	c.messages = c.messages[:1]
}

func (c *chatGPT) View(maxWidth int) string {
	var sb strings.Builder
	for _, m := range c.messages[1:] {
		switch m.Role {
		case "user":
			sb.WriteString(senderStyle.Render("You: "))
			content := wrap.String(m.Content, maxWidth-5)
			content, _ = c.renderer.Render(content)
			sb.WriteString(ensureTrailingNewline(content))
		case "assistant":
			sb.WriteString(botStyle.Render("ChatGPT: "))
			content := wrap.String(m.Content, maxWidth-5)
			content, _ = c.renderer.Render(content)
			sb.WriteString(ensureTrailingNewline(content))
		}
	}
	if len(c.pendingAnswer) > 0 {
		sb.WriteString(botStyle.Render("ChatGPT: "))
		content := wrap.String(string(c.pendingAnswer), maxWidth-5)
		content, _ = c.renderer.Render(content)
		sb.WriteString(content)
	}
	return sb.String()
}

type keyMap struct {
	mode
	Clear        key.Binding
	Quit         key.Binding
	ViewPortKeys viewport.KeyMap
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Submit, k.Clear, k.Switch, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Submit, k.Clear, k.Switch, k.Quit},
		{k.ViewPortKeys.Up, k.ViewPortKeys.Down, k.ViewPortKeys.PageUp, k.ViewPortKeys.PageDown},
	}
}

type mode struct {
	Name    string
	Switch  key.Binding
	Submit  key.Binding
	NewLine key.Binding
}

var (
	SingleLine = mode{
		Name:    "SingleLine",
		Switch:  key.NewBinding(key.WithKeys("ctrl+t"), key.WithHelp("ctrl+t", "multiline mode")),
		Submit:  key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "submit")),
		NewLine: key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("ctrl+d", "insert new line")),
	}
	MultiLine = mode{
		Name:    "MultiLine",
		Switch:  key.NewBinding(key.WithKeys("ctrl+t"), key.WithHelp("ctrl+t", "single line mode")),
		Submit:  key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("ctrl+d", "submit")),
		NewLine: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "insert new line")),
	}
)

func defaultKeyMap() keyMap {
	return keyMap{
		mode:  SingleLine,
		Clear: key.NewBinding(key.WithKeys("ctrl+r"), key.WithHelp("ctrl+r", "restart the chat")),
		Quit:  key.NewBinding(key.WithKeys("esc", "ctrl+c"), key.WithHelp("esc", "quit")),
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
	bot      *chatGPT
	keymap   keyMap
}

func initialModel(bot *chatGPT) model {
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

	// use enter to send messages, alt+enter for new line
	keys := defaultKeyMap()
	vp.KeyMap = keys.ViewPortKeys
	ta.KeyMap.InsertNewline = keys.mode.NewLine
	ta.KeyMap.TransposeCharacterBackward.SetEnabled(false)

	return model{
		textarea: ta,
		viewport: vp,
		help:     help.New(),
		bot:      bot,
		keymap:   keys,
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
	// TODO paste multiple lines
	// TODO copy without space padding
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.help.Width = msg.Width
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - m.textarea.Height() - 2
		m.textarea.SetWidth(msg.Width)
		m.viewport.SetContent(m.bot.View(m.viewport.Width))
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keymap.Submit):
			if msg.Alt {
				break
			}
			if m.bot.answering {
				break
			}
			input := m.textarea.Value()
			if strings.TrimSpace(input) == "" {
				break
			}
			cmds = append(cmds, m.bot.send(input))
			m.viewport.SetContent(m.bot.View(m.viewport.Width))
			m.viewport.GotoBottom()
			m.textarea.Reset()
			m.textarea.Blur()
			m.textarea.Placeholder = ""
		case key.Matches(msg, m.keymap.Clear):
			if m.bot.answering {
				break
			}
			m.err = nil
			m.bot.clearAll()
			m.viewport.SetContent(m.bot.View(m.viewport.Width))
		case key.Matches(msg, m.keymap.Switch):
			if m.keymap.Name == "SingleLine" {
				m.keymap.mode = MultiLine
				m.textarea.KeyMap.InsertNewline = MultiLine.NewLine
				m.textarea.ShowLineNumbers = true
				m.textarea.SetHeight(2)
				m.viewport.Height--
				m.viewport.SetContent(m.bot.View(m.viewport.Width))
			} else {
				m.keymap.mode = SingleLine
				m.textarea.KeyMap.InsertNewline = SingleLine.NewLine
				m.textarea.ShowLineNumbers = false
				m.textarea.SetHeight(1)
				m.viewport.Height++
				m.viewport.SetContent(m.bot.View(m.viewport.Width))
			}
		case key.Matches(msg, m.keymap.Quit):
			return m, tea.Quit
		}
	case deltaAnswerMsg:
		cmds = append(cmds, m.bot.addDeltaAnswer(string(msg)))
		m.err = nil
		m.viewport.SetContent(m.bot.View(m.viewport.Width))
		m.viewport.GotoBottom()
	case errMsg:
		// Network problem or answer completed, can't tell
		if msg == io.EOF {
			if len(m.bot.pendingAnswer) == 0 {
				m.err = errors.New("unexpected EOF, please try again")
			}
		} else {
			m.err = msg
		}
		m.bot.answerDone()
		m.textarea.Placeholder = "Send a message..."
		m.textarea.Focus()
	}

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	var bottomLine string
	if m.err != nil {
		bottomLine = errorStyle.Render(fmt.Sprintf("error: %v", m.err))
	}
	if bottomLine == "" {
		bottomLine = m.help.View(m.keymap)
	}
	return lipgloss.JoinVertical(
		lipgloss.Left,
		m.viewport.View(),
		m.textarea.View(),
		lipgloss.NewStyle().PaddingTop(1).Render(bottomLine),
	)
}

func ensureTrailingNewline(s string) string {
	if !strings.HasSuffix(s, "\n") {
		return s + "\n"
	}
	return s
}
