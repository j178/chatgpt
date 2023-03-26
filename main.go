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
	promptKey = flag.String("p", "", "Key of prompt defined in config file, or prompt itself")
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
	if *promptKey != "" {
		conf.Conversation.Prompt = *promptKey
	}

	chatgpt := newChatGPT(conf)
	// One-time ask-and-response mode
	if !isatty.IsTerminal(os.Stdin.Fd()) && !isatty.IsCygwinTerminal(os.Stdin.Fd()) {
		question, err := io.ReadAll(os.Stdin)
		if err != nil {
			log.Fatal(err)
		}
		conversationConf := conf.Conversation
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

type GlobalConfig struct {
	APIKey       string             `json:"api_key,omitempty"`
	Endpoint     string             `json:"endpoint,omitempty"`
	Prompts      map[string]string  `json:"prompts,omitempty"`
	Conversation ConversationConfig `json:"conversation,omitempty"`
}

func (c GlobalConfig) LookupPrompt(key string) string {
	prompt := c.Prompts[key]
	if prompt == "" {
		return key
	}
	return prompt
}

func configDir() (string, error) {
	home, err := homedir.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "chatgpt"), nil
}

func readOrWriteConfig(conf *GlobalConfig) error {
	dir, err := configDir()
	if err != nil {
		return fmt.Errorf("failed to get config dir: %w", err)
	}
	path := filepath.Join(dir, "config.json")

	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			err = os.MkdirAll(filepath.Dir(path), 0o755)
			if err != nil {
				return fmt.Errorf("failed to create config dir: %w", err)
			}
			f, err = os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
			defer func() { _ = f.Close() }()
			enc := json.NewEncoder(f)
			enc.SetIndent("", "  ")
			err = enc.Encode(conf)
			if err != nil {
				return fmt.Errorf("failed to write config file: %w", err)
			}
			return nil
		}
		return fmt.Errorf("failed to open config file: %w", err)
	}
	defer func() { _ = f.Close() }()
	err = json.NewDecoder(f).Decode(conf)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}
	return nil
}

func initConfig() (GlobalConfig, error) {
	conf := GlobalConfig{
		Endpoint: "https://api.openai.com/v1",
		Prompts: map[string]string{
			"default": "You are ChatGPT, a large language model trained by OpenAI. Answer as concisely as possible.",
		},
		Conversation: ConversationConfig{
			Model:         openai.GPT3Dot5Turbo,
			Prompt:        "default",
			ContextLength: 6,
			Stream:        true,
			Temperature:   0,
			MaxTokens:     1024,
		},
	}
	err := readOrWriteConfig(&conf)
	if err != nil {
		log.Println(err)
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
		return GlobalConfig{}, errors.New("Missing OPENAI_API_KEY environment variable, you can find or create your API key here: https://platform.openai.com/account/api-keys")
	}
	// TODO: support non-chat models
	switch conf.Conversation.Model {
	case openai.GPT3Dot5Turbo0301, openai.GPT3Dot5Turbo, openai.GPT4, openai.GPT40314, openai.GPT432K0314, openai.GPT432K:
	default:
		return GlobalConfig{}, errors.New("Invalid model, please choose one of the following: gpt-3.5-turbo-0301, gpt-3.5-turbo, gpt-4, gpt-4-0314, gpt-4-32k-0314, gpt-4-32k")
	}
	return conf, nil
}

type ConversationManager struct {
	file          string
	globalConf    GlobalConfig
	Conversations []*Conversation `json:"conversations"`
	Idx           int             `json:"last_idx"`
}

func NewConversationManager(conf GlobalConfig) *ConversationManager {
	h := &ConversationManager{
		globalConf: conf,
		Idx:        -1,
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

func (m *ConversationManager) Dump() error {
	if m.file == "" {
		return nil
	}
	err := createIfNotExists(m.file, false)
	if err != nil {
		return err
	}
	f, err := os.Create(m.file)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	err = enc.Encode(m)
	return err
}

func (m *ConversationManager) Load() error {
	if m.file == "" {
		return nil
	}
	f, err := os.Open(m.file)
	if err != nil {
		return err
	}
	err = json.NewDecoder(f).Decode(m)
	if err != nil {
		return err
	}
	for _, c := range m.Conversations {
		c.manager = m
	}
	return nil
}

func (m *ConversationManager) New(conf ConversationConfig) *Conversation {
	c := &Conversation{
		manager: m,
		Config:  conf,
	}
	m.Conversations = append(m.Conversations, c)
	m.Idx = len(m.Conversations) - 1
	return c
}

func (m *ConversationManager) RemoveCurr() {
	if len(m.Conversations) == 0 {
		return
	}
	m.Conversations = append(m.Conversations[:m.Idx], m.Conversations[m.Idx+1:]...)
	if m.Idx >= len(m.Conversations) {
		m.Idx = len(m.Conversations) - 1
	}
}

func (m *ConversationManager) Len() int {
	return len(m.Conversations)
}

func (m *ConversationManager) GetIndex(c *Conversation) int {
	for i, c2 := range m.Conversations {
		if c == c2 {
			return i
		}
	}
	return -1
}

func (m *ConversationManager) Curr() *Conversation {
	if len(m.Conversations) == 0 {
		// create initial conversation using default config
		return m.New(m.globalConf.Conversation)
	}
	return m.Conversations[m.Idx]
}

func (m *ConversationManager) Prev() *Conversation {
	if len(m.Conversations) == 0 {
		return nil
	}
	m.Idx--
	if m.Idx < 0 {
		m.Idx = 0 // dont wrap around
	}
	return m.Conversations[m.Idx]
}

func (m *ConversationManager) Next() *Conversation {
	if len(m.Conversations) == 0 {
		return nil
	}
	m.Idx++
	if m.Idx >= len(m.Conversations) {
		m.Idx = len(m.Conversations) - 1 // dont wrap around
	}
	return m.Conversations[m.Idx]
}

type QnA struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

type Conversation struct {
	manager   *ConversationManager
	Config    ConversationConfig `json:"config"`
	Forgotten []QnA              `json:"forgotten,omitempty"`
	Context   []QnA              `json:"context,omitempty"`
	Pending   *QnA               `json:"pending,omitempty"`
}

func (c *Conversation) AddQuestion(q string) {
	c.Pending = &QnA{Question: q}
}

func (c *Conversation) UpdatePending(ans string, done bool) {
	if c.Pending == nil {
		return
	}
	c.Pending.Answer += ans
	if done {
		c.Context = append(c.Context, *c.Pending)
		if len(c.Context) > c.Config.ContextLength {
			c.Forgotten = append(c.Forgotten, c.Context[0])
			c.Context = c.Context[1:]
		}
		c.Pending = nil
	}
}

func (c *Conversation) GetContext() []openai.ChatCompletionMessage {
	messages := make([]openai.ChatCompletionMessage, 0, 2*len(c.Context)+2)
	messages = append(
		messages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: c.manager.globalConf.LookupPrompt(c.Config.Prompt),
		},
	)
	for _, c := range c.Context {
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
	if c.Pending != nil {
		messages = append(
			messages, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleUser,
				Content: c.Pending.Question,
			},
		)
	}
	return messages
}

func (c *Conversation) ForgetContext() {
	c.Forgotten = append(c.Forgotten, c.Context...)
	c.Context = nil
}

func (c *Conversation) PendingAnswer() string {
	if c.Pending == nil {
		return ""
	}
	return c.Pending.Answer
}

func (c *Conversation) LastAnswer() string {
	if len(c.Context) == 0 {
		return ""
	}
	return c.Context[len(c.Context)-1].Answer
}

func (c *Conversation) Len() int {
	return len(c.Forgotten) + len(c.Context)
}

func (c *Conversation) GetQuestion(idx int) string {
	if idx < 0 || idx >= c.Len() {
		return ""
	}
	if idx < len(c.Forgotten) {
		return c.Forgotten[idx].Question
	}
	return c.Context[idx-len(c.Forgotten)].Question
}

type ChatGPT struct {
	globalConf GlobalConfig
	client     *openai.Client
	stream     *openai.ChatCompletionStream
	answering  bool
}

func newChatGPT(conf GlobalConfig) *ChatGPT {
	config := openai.DefaultConfig(conf.APIKey)
	if conf.Endpoint != "" {
		config.BaseURL = conf.Endpoint
	}
	client := openai.NewClientWithConfig(config)
	return &ChatGPT{globalConf: conf, client: client}
}

func (c *ChatGPT) ask(conf ConversationConfig, question string) (string, error) {
	resp, err := c.client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: conf.Model,
			Messages: []openai.ChatCompletionMessage{
				{Role: openai.ChatMessageRoleSystem, Content: c.globalConf.LookupPrompt(conf.Prompt)},
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
	c.answering = true
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

type InputMode int

const (
	InputModelSingleLine InputMode = iota
	InputModelMultiLine
)

type model struct {
	viewport      viewport.Model
	textarea      textarea.Model
	help          help.Model
	inputMode     InputMode
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
	renderer, _ := glamour.NewTermRenderer(
		glamour.WithEnvironmentConfig(),
		glamour.WithWordWrap(0), // we do hard-wrapping ourselves
	)

	keymap := defaultKeyMap()
	m := model{
		textarea:      ta,
		viewport:      vp,
		help:          help.New(),
		inputMode:     InputModelSingleLine,
		chatgpt:       chatgpt,
		conversations: conversations,
		keymap:        keymap,
		renderer:      renderer,
	}
	m.historyIdx = m.conversations.Curr().Len()
	UseSingleLine(&m)
	return m
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
		m.viewport.GotoBottom()
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keymap.ShowHelp, m.keymap.HideHelp):
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
			if m.chatgpt.answering {
				break
			}
			m.err = nil
			// TODO change config when creating new conversation
			m.conversations.New(m.conversations.globalConf.Conversation)
			m.viewport.SetContent(m.RenderConversation(m.viewport.Width))
			m.viewport.GotoBottom()
			m.historyIdx = 0
		case key.Matches(msg, m.keymap.ForgetContext):
			if m.chatgpt.answering {
				break
			}
			m.err = nil
			m.conversations.Curr().ForgetContext()
			m.viewport.SetContent(m.RenderConversation(m.viewport.Width))
			m.viewport.GotoBottom()
		case key.Matches(msg, m.keymap.RemoveConversation):
			if m.chatgpt.answering {
				break
			}
			m.err = nil
			m.conversations.RemoveCurr()
			m.viewport.SetContent(m.RenderConversation(m.viewport.Width))
			m.historyIdx = m.conversations.Curr().Len()
		case key.Matches(msg, m.keymap.PrevConversation):
			if m.chatgpt.answering {
				break
			}
			m.err = nil
			m.conversations.Prev()
			m.viewport.SetContent(m.RenderConversation(m.viewport.Width))
			m.viewport.GotoBottom()
			m.historyIdx = m.conversations.Curr().Len()
		case key.Matches(msg, m.keymap.NextConversation):
			if m.chatgpt.answering {
				break
			}
			m.err = nil
			m.conversations.Next()
			m.viewport.SetContent(m.RenderConversation(m.viewport.Width))
			m.viewport.GotoBottom()
			m.historyIdx = m.conversations.Curr().Len()
		case key.Matches(msg, m.keymap.SwitchMultiline):
			if m.inputMode == InputModelSingleLine {
				m.inputMode = InputModelMultiLine
				UseMultiLine(&m)
				m.textarea.ShowLineNumbers = true
				m.textarea.SetHeight(2)
				m.viewport.Height--
			} else {
				m.inputMode = InputModelSingleLine
				UseSingleLine(&m)
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
		sb.WriteString(lipgloss.NewStyle().PaddingLeft(5).Faint(true).Render("----- New Session -----"))
		sb.WriteString("\n")
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
	var conversationIndicator string
	if m.conversations.Len() > 1 {
		conversationIdx := m.conversations.Idx
		conversationIndicator = fmt.Sprintf("(%d/%d) ", conversationIdx+1, m.conversations.Len())
	}
	if m.help.ShowAll {
		conversationIndicator = ""
	}

	bottomLine = conversationIndicator + bottomLine
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
