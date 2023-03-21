package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
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
	"github.com/mattn/go-isatty"
	"github.com/mitchellh/go-homedir"
	"github.com/muesli/reflow/wrap"
	"github.com/sashabaranov/go-openai"
)

var (
	senderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))
	botStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	errorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
)

var (
	debug     = os.Getenv("DEBUG") == "1"
	promptKey = flag.String("p", "default", "Key of prompt defined in config file, or prompt itself")
)

type (
	errMsg         error
	deltaAnswerMsg string
	answerMsg      string
)

// TODO support switch model in TUI
// TODO support switch prompt in TUI

func main() {
	flag.Parse()
	conf, err := getConfig()
	if err != nil {
		log.Fatal(err)
	}
	prompt := conf.Prompts[*promptKey]
	if prompt == "" {
		prompt = *promptKey
	}

	bot := newChatGPT(conf)
	// One-time ask-and-response mode
	if !isatty.IsTerminal(os.Stdin.Fd()) && !isatty.IsCygwinTerminal(os.Stdin.Fd()) {
		question, err := io.ReadAll(os.Stdin)
		if err != nil {
			log.Fatal(err)
		}
		answer, err := bot.ask(prompt, string(question))
		if err != nil {
			log.Fatal(err)
		}
		fmt.Print(answer)
		return
	}

	history := newHistory(conf.ContextLength, prompt)
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

type Config struct {
	APIKey        string            `json:"api_key,omitempty"`
	Endpoint      string            `json:"endpoint,omitempty"`
	Prompts       map[string]string `json:"prompts,omitempty"`
	ContextLength int               `json:"context_length,omitempty"`
	Model         string            `json:"model,omitempty"`
	Stream        bool              `json:"stream,omitempty"`
	Temperature   float32           `json:"temperature,omitempty"`
	MaxTokens     int               `json:"max_tokens,omitempty"`
}

func configDir() (string, error) {
	home, err := homedir.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "chatgpt"), nil
}

func readConfig(conf *Config) error {
	dir, err := configDir()
	if err != nil {
		return err
	}
	f, err := os.Open(filepath.Join(dir, "config.json"))
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	err = json.NewDecoder(f).Decode(conf)
	if err != nil {
		return err
	}
	return nil
}

func getConfig() (Config, error) {
	conf := Config{
		Endpoint: "https://api.openai.com/v1",
		Prompts: map[string]string{
			"default": "You are ChatGPT, a large language model trained by OpenAI. Answer as concisely as possible.",
		},
		Model:         openai.GPT3Dot5Turbo,
		ContextLength: 6,
		Stream:        true,
		Temperature:   0,
		MaxTokens:     1024,
	}
	err := readConfig(&conf)
	if err != nil {
		log.Println("Failed to read config file, using default config:", err)
	}
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey != "" {
		conf.APIKey = apiKey
	}
	endpoint := os.Getenv("OPENAI_API_ENDPOINT")
	if endpoint != "" {
		conf.Endpoint = endpoint
	}
	if conf.APIKey == "" {
		return Config{}, errors.New("Missing OPENAI_API_KEY environment variable, you can find or create your API key here: https://platform.openai.com/account/api-keys")
	}
	// TODO: support non-chat models
	switch conf.Model {
	case openai.GPT3Dot5Turbo0301, openai.GPT3Dot5Turbo, openai.GPT4, openai.GPT40314, openai.GPT432K0314, openai.GPT432K:
	default:
		return Config{}, errors.New("Invalid model, please choose one of the following: gpt-3.5-turbo-0301, gpt-3.5-turbo, gpt-4, gpt-4-0314, gpt-4-32k-0314, gpt-4-32k")
	}
	return conf, nil
}

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
			Role:    openai.ChatMessageRoleSystem,
			Content: h.Prompt,
		},
	)
	for _, c := range h.Context {
		messages = append(
			messages, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleUser,
				Content: c.Question,
			},
		)
		messages = append(
			messages, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleAssistant,
				Content: c.Answer,
			},
		)
	}
	if h.Pending != nil {
		messages = append(
			messages, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleUser,
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

func (h *History) Len() int {
	return len(h.Forgotten) + len(h.Context)
}

func (h *History) GetQuestion(idx int) string {
	if idx < 0 || idx >= h.Len() {
		return ""
	}
	if idx < len(h.Forgotten) {
		return h.Forgotten[idx].Question
	}
	return h.Context[idx-len(h.Forgotten)].Question
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
	conf      Config
	stream    *openai.ChatCompletionStream
	answering bool
}

func newChatGPT(conf Config) *ChatGPT {
	config := openai.DefaultConfig(conf.APIKey)
	if conf.Endpoint != "" {
		config.BaseURL = conf.Endpoint
	}
	client := openai.NewClientWithConfig(config)
	return &ChatGPT{
		client: client,
		conf:   conf,
	}
}

func (c *ChatGPT) ask(prompt string, question string) (string, error) {
	resp, err := c.client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: c.conf.Model,
			Messages: []openai.ChatCompletionMessage{
				{Role: openai.ChatMessageRoleSystem, Content: prompt},
				{Role: openai.ChatMessageRoleUser, Content: question},
			},
			MaxTokens:   c.conf.MaxTokens,
			Temperature: c.conf.Temperature,
			N:           1,
		},
	)
	if err != nil {
		return "", err
	}
	content := resp.Choices[0].Message.Content
	return content, nil
}

func (c *ChatGPT) send(messages []openai.ChatCompletionMessage) tea.Cmd {
	return func() (msg tea.Msg) {
		err := retry.Do(
			func() error {
				if c.conf.Stream {
					stream, err := c.client.CreateChatCompletionStream(
						context.Background(),
						openai.ChatCompletionRequest{
							Model:       c.conf.Model,
							Messages:    messages,
							MaxTokens:   c.conf.MaxTokens,
							Temperature: c.conf.Temperature,
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
					content := resp.Choices[0].Delta.Content
					msg = deltaAnswerMsg(content)
				} else {
					resp, err := c.client.CreateChatCompletion(
						context.Background(),
						openai.ChatCompletionRequest{
							Model:       c.conf.Model,
							Messages:    messages,
							MaxTokens:   c.conf.MaxTokens,
							Temperature: c.conf.Temperature,
							N:           1,
						},
					)
					if err != nil {
						return errMsg(err)
					}
					content := resp.Choices[0].Message.Content
					msg = answerMsg(content)
				}
				return nil
			},
			retry.Attempts(3),
			retry.LastErrorOnly(true),
		)
		if err != nil {
			return errMsg(err)
		}
		return
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
	HistoryPrev  key.Binding
	HistoryNext  key.Binding
	ViewPortKeys viewport.KeyMap
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Help, k.Submit, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Help, k.Submit, k.Quit, k.Clear, k.Switch, k.Copy},
		{
			k.HistoryPrev,
			k.HistoryNext,
			k.ViewPortKeys.Up,
			k.ViewPortKeys.Down,
			k.ViewPortKeys.PageUp,
			k.ViewPortKeys.PageDown,
		},
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
		keyMode:     SingleLine,
		Help:        key.NewBinding(key.WithKeys("ctrl+h"), key.WithHelp("ctrl+h", "show help")),
		Clear:       key.NewBinding(key.WithKeys("ctrl+r"), key.WithHelp("ctrl+r", "restart the chat")),
		Quit:        key.NewBinding(key.WithKeys("esc", "ctrl+c"), key.WithHelp("esc", "quit")),
		Copy:        key.NewBinding(key.WithKeys("ctrl+y"), key.WithHelp("ctrl+y", "copy last answer")),
		HistoryPrev: key.NewBinding(key.WithKeys("up", "ctrl+p"), key.WithHelp("↑/ctrl+p", "previous question")),
		HistoryNext: key.NewBinding(key.WithKeys("down", "ctrl+n"), key.WithHelp("↓/ctrl+n", "next question")),
		ViewPortKeys: viewport.KeyMap{
			PageDown: key.NewBinding(
				key.WithKeys("pgdown"),
				key.WithHelp("pgdn", "page down"),
			),
			PageUp: key.NewBinding(
				key.WithKeys("pgup"),
				key.WithHelp("pgup", "page up"),
			),
			HalfPageUp:   key.NewBinding(key.WithDisabled()),
			HalfPageDown: key.NewBinding(key.WithDisabled()),
			Up: key.NewBinding(
				key.WithKeys("ctrl+up"),
				key.WithHelp("ctrl+up", "up"),
			),
			Down: key.NewBinding(
				key.WithKeys("ctrl+down"),
				key.WithHelp("ctrl+down", "down"),
			),
		},
	}
}

type model struct {
	viewport   viewport.Model
	textarea   textarea.Model
	help       help.Model
	err        error
	chatgpt    *ChatGPT
	history    *History
	keymap     keyMap
	width      int
	height     int
	historyIdx int
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
			m.historyIdx = m.history.Len() + 1
		case key.Matches(msg, m.keymap.Clear):
			if m.chatgpt.answering {
				break
			}
			m.err = nil
			m.history.Clear()
			m.viewport.SetContent(m.history.View(m.viewport.Width))
			m.historyIdx = 0
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
		case key.Matches(msg, m.keymap.HistoryNext):
			if m.chatgpt.answering {
				break
			}
			idx := m.historyIdx + 1
			if idx >= m.history.Len() {
				m.historyIdx = m.history.Len()
				m.textarea.SetValue("")
			} else {
				m.textarea.SetValue(m.history.GetQuestion(idx))
				m.historyIdx = idx
			}
		case key.Matches(msg, m.keymap.HistoryPrev):
			if m.chatgpt.answering {
				break
			}
			idx := m.historyIdx - 1
			if idx < 0 {
				idx = 0
			}
			q := m.history.GetQuestion(idx)
			m.textarea.SetValue(q)
			m.historyIdx = idx
		case key.Matches(msg, m.keymap.Quit):
			return m, tea.Quit
		}
	case deltaAnswerMsg:
		m.history.UpdatePending(string(msg), false)
		cmds = append(cmds, m.chatgpt.recv())
		m.err = nil
		m.viewport.SetContent(m.history.View(m.viewport.Width))
		m.viewport.GotoBottom()
	case answerMsg:
		m.history.UpdatePending(string(msg), true)
		m.chatgpt.done()
		m.viewport.SetContent(m.history.View(m.viewport.Width))
		m.viewport.GotoBottom()
		m.textarea.Placeholder = "Send a message..."
		m.textarea.Focus()
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
