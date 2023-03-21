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
	"time"

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
	saveMsg        struct{}
)

// TODO support switch model in TUI
// TODO support switch prompt in TUI

func main() {
	flag.Parse()
	conf, err := initConfig()
	if err != nil {
		log.Fatal(err)
	}
	prompt := conf.Prompts[*promptKey]
	if prompt == "" {
		prompt = *promptKey
	}
	conf.Default.Prompt = prompt

	chatgpt := newChatGPT(conf.APIKey, conf.Endpoint)
	// One-time ask-and-response mode
	if !isatty.IsTerminal(os.Stdin.Fd()) && !isatty.IsCygwinTerminal(os.Stdin.Fd()) {
		question, err := io.ReadAll(os.Stdin)
		if err != nil {
			log.Fatal(err)
		}
		conversationConf := conf.Default
		answer, err := chatgpt.ask(conversationConf, string(question))
		if err != nil {
			log.Fatal(err)
		}
		fmt.Print(answer)
		return
	}

	conversations := NewConversationManager(conf)
	p := tea.NewProgram(
		initialModel(chatgpt, conversations),
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

type ConversationConfig struct {
	Prompt        string  `json:"prompt,omitempty"`
	ContextLength int     `json:"context_length,omitempty"`
	Model         string  `json:"model,omitempty"`
	Stream        bool    `json:"stream,omitempty"`
	Temperature   float32 `json:"temperature,omitempty"`
	MaxTokens     int     `json:"max_tokens,omitempty"`
}

type Config struct {
	APIKey   string             `json:"api_key,omitempty"`
	Endpoint string             `json:"endpoint,omitempty"`
	Prompts  map[string]string  `json:"prompts,omitempty"`
	Default  ConversationConfig `json:"default,omitempty"`
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

// TODO create config
func initConfig() (Config, error) {
	conf := Config{
		Endpoint: "https://api.openai.com/v1",
		Prompts: map[string]string{
			"default": "You are ChatGPT, a large language model trained by OpenAI. Answer as concisely as possible.",
		},
		Default: ConversationConfig{
			Model:         openai.GPT3Dot5Turbo,
			Prompt:        "default",
			ContextLength: 6,
			Stream:        true,
			Temperature:   0,
			MaxTokens:     1024,
		},
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
	switch conf.Default.Model {
	case openai.GPT3Dot5Turbo0301, openai.GPT3Dot5Turbo, openai.GPT4, openai.GPT40314, openai.GPT432K0314, openai.GPT432K:
	default:
		return Config{}, errors.New("Invalid model, please choose one of the following: gpt-3.5-turbo-0301, gpt-3.5-turbo, gpt-4, gpt-4-0314, gpt-4-32k-0314, gpt-4-32k")
	}
	return conf, nil
}

type ConversationManager struct {
	file          string
	conf          Config
	Conversations []*Conversation `json:"conversations"`
	Idx           int             `json:"last_idx"`
}

func NewConversationManager(conf Config) *ConversationManager {
	h := &ConversationManager{
		conf: conf,
	}
	dir, err := configDir()
	if err != nil {
		log.Println("Failed to get config dir:", err)
		return h
	}
	h.file = filepath.Join(dir, "conversations.json")
	err = h.Load()
	if err != nil {
		log.Println("Failed to load history:", err)
	}
	return h
}

func (h *ConversationManager) Dump() error {
	if h.file == "" {
		return nil
	}
	err := createIfNotExists(h.file, false)
	if err != nil {
		return err
	}
	f, err := os.Create(h.file)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	err = enc.Encode(h)
	return err
}

func (h *ConversationManager) Load() error {
	if h.file == "" {
		return nil
	}
	f, err := os.Open(h.file)
	if err != nil {
		return err
	}
	err = json.NewDecoder(f).Decode(h)
	if err != nil {
		return err
	}
	return nil
}

func (h *ConversationManager) New(conf ConversationConfig) *Conversation {
	c := &Conversation{
		Config: conf,
	}
	h.Conversations = append(h.Conversations, c)
	h.Idx = len(h.Conversations) - 1
	return c
}

func (h *ConversationManager) RemoveCurr() {
	if len(h.Conversations) == 0 {
		return
	}
	h.Conversations = append(h.Conversations[:h.Idx], h.Conversations[h.Idx+1:]...)
	if h.Idx >= len(h.Conversations) {
		h.Idx = len(h.Conversations) - 1
	}
}

func (h *ConversationManager) Len() int {
	return len(h.Conversations)
}

func (h *ConversationManager) Get(idx int) *Conversation {
	if idx < 0 || idx >= len(h.Conversations) {
		return nil
	}
	return h.Conversations[idx]
}

func (h *ConversationManager) GetIndex(c *Conversation) int {
	for i, c2 := range h.Conversations {
		if c == c2 {
			return i
		}
	}
	return -1
}

func (h *ConversationManager) Curr() *Conversation {
	if len(h.Conversations) == 0 {
		// create initial conversation using default config
		return h.New(h.conf.Default)
	}
	return h.Conversations[h.Idx]
}

func (h *ConversationManager) Prev() *Conversation {
	if len(h.Conversations) == 0 {
		return nil
	}
	h.Idx--
	if h.Idx < 0 {
		h.Idx = len(h.Conversations) - 1
	}
	return h.Conversations[h.Idx]
}

func (h *ConversationManager) Next() *Conversation {
	if len(h.Conversations) == 0 {
		return nil
	}
	h.Idx++
	if h.Idx >= len(h.Conversations) {
		h.Idx = 0
	}
	return h.Conversations[h.Idx]
}

type QnA struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

type Conversation struct {
	Config    ConversationConfig `json:"config"`
	Forgotten []QnA              `json:"forgotten,omitempty"`
	Context   []QnA              `json:"context,omitempty"`
	Pending   *QnA               `json:"pending,omitempty"`
}

func (h *Conversation) AddQuestion(q string) {
	h.Pending = &QnA{Question: q}
}

func (h *Conversation) UpdatePending(ans string, done bool) {
	if h.Pending == nil {
		return
	}
	h.Pending.Answer += ans
	if done {
		h.Context = append(h.Context, *h.Pending)
		if len(h.Context) > h.Config.ContextLength {
			h.Forgotten = append(h.Forgotten, h.Context[0])
			h.Context = h.Context[1:]
		}
		h.Pending = nil
	}
}

func (h *Conversation) GetContext() []openai.ChatCompletionMessage {
	messages := make([]openai.ChatCompletionMessage, 0, 2*len(h.Context)+2)
	messages = append(
		messages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: h.Config.Prompt,
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

func (h *Conversation) PendingAnswer() string {
	if h.Pending == nil {
		return ""
	}
	return h.Pending.Answer
}

func (h *Conversation) LastAnswer() string {
	if len(h.Context) == 0 {
		return ""
	}
	return h.Context[len(h.Context)-1].Answer
}

func (h *Conversation) Len() int {
	return len(h.Forgotten) + len(h.Context)
}

func (h *Conversation) GetQuestion(idx int) string {
	if idx < 0 || idx >= h.Len() {
		return ""
	}
	if idx < len(h.Forgotten) {
		return h.Forgotten[idx].Question
	}
	return h.Context[idx-len(h.Forgotten)].Question
}

type ChatGPT struct {
	client    *openai.Client
	stream    *openai.ChatCompletionStream
	answering bool
}

func newChatGPT(apiKey, endpoint string) *ChatGPT {
	config := openai.DefaultConfig(apiKey)
	if endpoint != "" {
		config.BaseURL = endpoint
	}
	client := openai.NewClientWithConfig(config)
	return &ChatGPT{client: client}
}

func (c *ChatGPT) ask(conf ConversationConfig, question string) (string, error) {
	resp, err := c.client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: conf.Model,
			Messages: []openai.ChatCompletionMessage{
				{Role: openai.ChatMessageRoleSystem, Content: conf.Prompt},
				{Role: openai.ChatMessageRoleUser, Content: question},
			},
			MaxTokens:   conf.MaxTokens,
			Temperature: conf.Temperature,
			N:           1,
		},
	)
	if err != nil {
		return "", err
	}
	content := resp.Choices[0].Message.Content
	return content, nil
}

func (c *ChatGPT) send(conf ConversationConfig, messages []openai.ChatCompletionMessage) tea.Cmd {
	return func() (msg tea.Msg) {
		err := retry.Do(
			func() error {
				if conf.Stream {
					stream, err := c.client.CreateChatCompletionStream(
						context.Background(),
						openai.ChatCompletionRequest{
							Model:       conf.Model,
							Messages:    messages,
							MaxTokens:   conf.MaxTokens,
							Temperature: conf.Temperature,
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
							Model:       conf.Model,
							Messages:    messages,
							MaxTokens:   conf.MaxTokens,
							Temperature: conf.Temperature,
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
	Help               key.Binding
	Quit               key.Binding
	Copy               key.Binding
	PrevHistory        key.Binding
	NextHistory        key.Binding
	NewConversation    key.Binding
	RemoveConversation key.Binding
	PrevConversation   key.Binding
	NextConversation   key.Binding
	ViewPortKeys       viewport.KeyMap
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Help, k.Submit, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Help, k.Submit, k.Quit, k.SwitchMultiline, k.Copy},
		{k.NewConversation, k.RemoveConversation, k.PrevConversation, k.NextConversation},
		{
			k.PrevHistory,
			k.NextHistory,
			k.ViewPortKeys.Up,
			k.ViewPortKeys.Down,
			k.ViewPortKeys.PageUp,
			k.ViewPortKeys.PageDown,
		},
	}
}

type keyMode struct {
	Name            string
	SwitchMultiline key.Binding
	Submit          key.Binding
	NewLine         key.Binding
}

var (
	SingleLine = keyMode{
		Name:            "SingleLine",
		SwitchMultiline: key.NewBinding(key.WithKeys("ctrl+t"), key.WithHelp("ctrl+t", "multiline mode")),
		Submit:          key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "submit")),
		NewLine:         key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("ctrl+d", "insert new line")),
	}
	MultiLine = keyMode{
		Name:            "MultiLine",
		SwitchMultiline: key.NewBinding(key.WithKeys("ctrl+t"), key.WithHelp("ctrl+t", "single line mode")),
		Submit:          key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("ctrl+d", "submit")),
		NewLine:         key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "insert new line")),
	}
)

func defaultKeyMap() keyMap {
	return keyMap{
		keyMode:         SingleLine,
		Help:            key.NewBinding(key.WithKeys("ctrl+h"), key.WithHelp("ctrl+h", "show help")),
		Quit:            key.NewBinding(key.WithKeys("esc", "ctrl+c"), key.WithHelp("esc", "quit")),
		Copy:            key.NewBinding(key.WithKeys("ctrl+y"), key.WithHelp("ctrl+y", "copy last answer")),
		PrevHistory:     key.NewBinding(key.WithKeys("up"), key.WithHelp("↑", "previous question")),
		NextHistory:     key.NewBinding(key.WithKeys("down"), key.WithHelp("↓", "next question")),
		NewConversation: key.NewBinding(key.WithKeys("ctrl+n"), key.WithHelp("ctrl+n", "new conversation")),
		RemoveConversation: key.NewBinding(
			key.WithKeys("ctrl+r"),
			key.WithHelp("ctrl+r", "remove current conversation"),
		),
		PrevConversation: key.NewBinding(
			key.WithKeys("ctrl+left"),
			key.WithHelp("ctrl+left", "previous conversation"),
		),
		NextConversation: key.NewBinding(key.WithKeys("ctrl+right"), key.WithHelp("ctrl+right", "next conversation")),
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
	viewport      viewport.Model
	textarea      textarea.Model
	help          help.Model
	err           error
	chatgpt       *ChatGPT
	conversations *ConversationManager
	keymap        keyMap
	width         int
	height        int
	historyIdx    int
	renderer      *glamour.TermRenderer
}

func initialModel(chatgpt *ChatGPT, conversations *ConversationManager) model {
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

	renderer, _ := glamour.NewTermRenderer(
		glamour.WithEnvironmentConfig(),
		glamour.WithWordWrap(0), // we do hard-wrapping ourselves
	)

	return model{
		textarea:      ta,
		viewport:      vp,
		help:          help.New(),
		chatgpt:       chatgpt,
		conversations: conversations,
		keymap:        keymap,
		renderer:      renderer,
	}
}

func savePeriodically() tea.Cmd {
	return tea.Tick(15*time.Second, func(time.Time) tea.Msg { return saveMsg{} })
}

func (m model) Init() tea.Cmd {
	if !debug { // disable blink when debug
		return tea.Batch(textarea.Blink, savePeriodically())
	}
	return savePeriodically()
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
		m.viewport.SetContent(m.RenderConversation(m.viewport.Width))
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keymap.Help):
			m.help.ShowAll = !m.help.ShowAll
			m.viewport.Height = m.height - m.textarea.Height() - lipgloss.Height(m.bottomLine())
			m.viewport.SetContent(m.RenderConversation(m.viewport.Width))
		case key.Matches(msg, m.keymap.Submit):
			if m.chatgpt.answering {
				break
			}
			input := strings.TrimSpace(m.textarea.Value())
			if input == "" {
				break
			}
			m.conversations.Curr().AddQuestion(input)
			cmds = append(cmds, m.chatgpt.send(m.conversations.Curr().Config, m.conversations.Curr().GetContext()))
			m.viewport.SetContent(m.RenderConversation(m.viewport.Width))
			m.viewport.GotoBottom()
			m.textarea.Reset()
			m.textarea.Blur()
			m.textarea.Placeholder = ""
			m.historyIdx = m.conversations.Curr().Len() + 1
		case key.Matches(msg, m.keymap.NewConversation):
			m.err = nil
			// TODO change config when creating new conversation
			m.conversations.New(m.conversations.conf.Default)
			m.viewport.SetContent(m.RenderConversation(m.viewport.Width))
			m.viewport.GotoBottom()
		case key.Matches(msg, m.keymap.RemoveConversation):
			if m.chatgpt.answering {
				break
			}
			m.err = nil
			m.conversations.RemoveCurr()
			m.viewport.SetContent(m.RenderConversation(m.viewport.Width))
			m.historyIdx = 0
		case key.Matches(msg, m.keymap.PrevConversation):
			m.err = nil
			m.conversations.Prev()
			m.viewport.SetContent(m.RenderConversation(m.viewport.Width))
			m.viewport.GotoBottom()
		case key.Matches(msg, m.keymap.NextConversation):
			m.err = nil
			m.conversations.Next()
			m.viewport.SetContent(m.RenderConversation(m.viewport.Width))
			m.viewport.GotoBottom()
		case key.Matches(msg, m.keymap.SwitchMultiline):
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
			m.viewport.SetContent(m.RenderConversation(m.viewport.Width))
		case key.Matches(msg, m.keymap.Copy):
			if m.chatgpt.answering || m.conversations.Curr().LastAnswer() == "" {
				break
			}
			_ = clipboard.WriteAll(m.conversations.Curr().LastAnswer())
		case key.Matches(msg, m.keymap.NextHistory):
			if m.chatgpt.answering {
				break
			}
			idx := m.historyIdx + 1
			if idx >= m.conversations.Curr().Len() {
				m.historyIdx = m.conversations.Curr().Len()
				m.textarea.SetValue("")
			} else {
				m.textarea.SetValue(m.conversations.Curr().GetQuestion(idx))
				m.historyIdx = idx
			}
		case key.Matches(msg, m.keymap.PrevHistory):
			if m.chatgpt.answering {
				break
			}
			idx := m.historyIdx - 1
			if idx < 0 {
				idx = 0
			}
			q := m.conversations.Curr().GetQuestion(idx)
			m.textarea.SetValue(q)
			m.historyIdx = idx
		case key.Matches(msg, m.keymap.Quit):
			_ = m.conversations.Dump()
			return m, tea.Quit
		}
	case deltaAnswerMsg:
		m.conversations.Curr().UpdatePending(string(msg), false)
		cmds = append(cmds, m.chatgpt.recv())
		m.err = nil
		m.viewport.SetContent(m.RenderConversation(m.viewport.Width))
		m.viewport.GotoBottom()
	case answerMsg:
		m.conversations.Curr().UpdatePending(string(msg), true)
		m.chatgpt.done()
		m.viewport.SetContent(m.RenderConversation(m.viewport.Width))
		m.viewport.GotoBottom()
		m.textarea.Placeholder = "Send a message..."
		m.textarea.Focus()
	case saveMsg:
		m.err = m.conversations.Dump()
		cmds = append(cmds, savePeriodically())
	case errMsg:
		// Network problem or answer completed, can't tell
		if msg == io.EOF {
			if m.conversations.Curr().PendingAnswer() == "" {
				m.err = errors.New("unexpected EOF, please try again")
			}
		} else {
			m.err = msg
		}
		m.conversations.Curr().UpdatePending("", true)
		m.chatgpt.done()
		m.textarea.Placeholder = "Send a message..."
		m.textarea.Focus()
	}

	return m, tea.Batch(cmds...)
}

func (m model) RenderConversation(maxWidth int) string {
	var sb strings.Builder
	c := m.conversations.Curr()
	if c == nil {
		return ""
	}
	renderer := m.renderer

	renderYou := func(content string) {
		sb.WriteString(senderStyle.Render("You: "))
		content = wrap.String(content, maxWidth-5)
		content, _ = renderer.Render(content)
		sb.WriteString(ensureTrailingNewline(content))
	}
	renderBot := func(content string) {
		if content == "" {
			return
		}
		sb.WriteString(botStyle.Render("ChatGPT: "))
		content = wrap.String(content, maxWidth-5)
		content, _ = renderer.Render(content)
		sb.WriteString(ensureTrailingNewline(content))
	}
	for _, m := range c.Forgotten {
		renderYou(m.Question)
		renderBot(m.Answer)
	}
	if len(c.Forgotten) > 0 {
		// TODO add a separator to indicate the previous messages are forgotten
	}
	for _, q := range c.Context {
		renderYou(q.Question)
		renderBot(q.Answer)
	}
	if c.Pending != nil {
		renderYou(c.Pending.Question)
		renderBot(c.Pending.Answer)
	}
	return sb.String()
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

func createIfNotExists(path string, isDir bool) error {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			if isDir {
				return os.MkdirAll(path, 0o755)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o666); err != nil {
				return err
			}
			f, err := os.OpenFile(path, os.O_CREATE, 0o666)
			if err != nil {
				return err
			}
			_ = f.Close()
		}
	}
	return nil
}
