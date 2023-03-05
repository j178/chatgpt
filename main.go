package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	gpt3 "github.com/sashabaranov/go-gpt3"
)

const maxTokens = 4096

var (
	senderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))
	botStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	errorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
	debug       = os.Getenv("DEBUG") == "1"
)

type (
	errMsg         error
	deltaAnswerMsg string
	answerDoneMsg  struct{}
)

func main() {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("Missing OPENAI_API_KEY, you can find or create your API key here: https://platform.openai.com/account/api-keys")
	}
	baseURI := os.Getenv("OPENAI_API_ENDPOINT")
	bot := newChatGPT(apiKey, baseURI)
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
	renderer      *glamour.TermRenderer
	stream        *gpt3.ChatCompletionStream
	pendingAnswer []byte
	answering     bool
}

func newChatGPT(apiKey string, baseURI string) *chatGPT {
	config := gpt3.DefaultConfig(apiKey)
	if baseURI != "" {
		config.BaseURL = baseURI
	}
	client := gpt3.NewClientWithConfig(config)
	renderer, _ := glamour.NewTermRenderer(
		glamour.WithEnvironmentConfig(),
	)
	return &chatGPT{
		client:   client,
		renderer: renderer,
		messages: []gpt3.ChatCompletionMessage{
			{
				Role:    "system",
				Content: "You are ChatGPT, a large language model trained by OpenAI. Answer as concisely as possible.",
			},
		},
	}
}

func (c *chatGPT) Ask(input string) tea.Cmd {
	c.AddMessage("user", input)
	return func() tea.Msg {
		stream, err := c.client.CreateChatCompletionStream(
			context.Background(),
			gpt3.ChatCompletionRequest{
				Model:       gpt3.GPT3Dot5Turbo,
				Messages:    c.messages,
				MaxTokens:   3000,
				Temperature: 0,
				N:           1,
			},
		)
		if err != nil {
			return errMsg(err)
		}
		c.answering = true
		c.stream = stream
		resp, err := stream.Recv()
		if err != nil {
			return errMsg(err)
		}
		content := resp.Choices[0].Delta.Content
		return deltaAnswerMsg(content)
	}
}

func (c *chatGPT) AddMessage(role, text string) {
	c.messages = append(
		c.messages, gpt3.ChatCompletionMessage{
			Role:    role,
			Content: text,
		},
	)
}

func (c *chatGPT) AddDeltaAnswer(delta string) tea.Cmd {
	c.pendingAnswer = append(c.pendingAnswer, delta...)
	return func() tea.Msg {
		resp, err := c.stream.Recv()
		if err == io.EOF {
			c.stream.Close()
			c.stream = nil
			c.answering = false
			c.AddMessage("assistant", string(c.pendingAnswer))
			c.pendingAnswer = c.pendingAnswer[:0]
			return answerDoneMsg{}
		}
		if err != nil {
			return errMsg(err)
		}
		content := resp.Choices[0].Delta.Content
		return deltaAnswerMsg(content)
	}
}

func (c *chatGPT) Clear() {
	c.messages = c.messages[:1]
}

func (c *chatGPT) View() string {
	var sb strings.Builder
	for _, m := range c.messages[1:] {
		switch m.Role {
		case "user":
			sb.WriteString(senderStyle.Render("You: "))
			content, _ := c.renderer.Render(m.Content)
			sb.WriteString(ensureTrailingNewline(content))
		case "assistant":
			sb.WriteString(botStyle.Render("ChatGPT: "))
			content, _ := c.renderer.Render(m.Content)
			sb.WriteString(ensureTrailingNewline(content))
		}
	}
	if len(c.pendingAnswer) > 0 {
		sb.WriteString(botStyle.Render("ChatGPT: "))
		content, _ := c.renderer.Render(string(c.pendingAnswer))
		sb.WriteString(content)
	}
	return sb.String()
}

type keyMap struct {
	Quit  key.Binding
	Clear key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Clear, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Clear, k.Quit},
	}
}

var keys = keyMap{
	Quit:  key.NewBinding(key.WithKeys("esc", "ctrl+c"), key.WithHelp("esc", "quit")),
	Clear: key.NewBinding(key.WithKeys("ctrl+r"), key.WithHelp("ctrl+r", "clear the chat")),
}

type model struct {
	viewport viewport.Model
	textarea textarea.Model
	help     help.Model
	err      error
	bot      *chatGPT
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

	ta.SetWidth(30)
	ta.SetHeight(1)

	// Remove cursor line styling
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.ShowLineNumbers = false

	vp := viewport.New(45, 5)
	// use enter to send messages, alt+enter for new line
	ta.KeyMap.InsertNewline.SetKeys("alt+enter")

	return model{
		textarea: ta,
		viewport: vp,
		help:     help.New(),
		bot:      bot,
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
	log.Printf("msg: %v", msg)

	m.textarea, cmd = m.textarea.Update(msg)
	cmds = append(cmds, cmd)
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	// TODO add help, clear, status bar
	// TODO shift+enter for new line
	// TODO viewport auto width, height
	// TODO 如何让输入框自动增加高度
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.help.Width = msg.Width
		m.viewport.Width = msg.Width
		// todo 更精确的计算
		m.viewport.Height = msg.Height - m.textarea.Height() - 2
		m.textarea.SetWidth(msg.Width)
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
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
			cmds = append(cmds, m.bot.Ask(input))
			m.viewport.SetContent(m.bot.View())
			m.viewport.GotoBottom()
			m.textarea.Reset()
			m.textarea.Blur()
			m.textarea.Placeholder = ""
		case tea.KeyCtrlR:
			if m.bot.answering {
				break
			}
			m.err = nil
			m.bot.Clear()
			m.viewport.SetContent(m.bot.View())
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
		}
	case deltaAnswerMsg:
		cmds = append(cmds, m.bot.AddDeltaAnswer(string(msg)))
		m.err = nil
		m.viewport.SetContent(m.bot.View())
		m.viewport.GotoBottom()
	case answerDoneMsg:
		m.err = nil
		m.textarea.Placeholder = "Send a message..."
		m.textarea.Focus()
		cmds = append(cmds, textarea.Blink)
	case errMsg:
		m.err = msg
		m.textarea.Placeholder = "Send a message..."
		m.textarea.Focus()
		cmds = append(cmds, textarea.Blink)
	}

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	var bottomLine string
	if m.err != nil {
		bottomLine = errorStyle.Render(fmt.Sprintf("\nerror: %v", m.err))
	}
	if bottomLine == "" {
		bottomLine = m.help.View(keys)
	}
	return fmt.Sprintf(
		"%s\n%s\n\n%s",
		m.viewport.View(),
		m.textarea.View(),
		bottomLine,
	)
}

func ensureTrailingNewline(s string) string {
	if !strings.HasSuffix(s, "\n") {
		return s + "\n"
	}
	return s
}
