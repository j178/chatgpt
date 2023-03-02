package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	gpt3 "github.com/sashabaranov/go-gpt3"
)

const maxTokens = 4096

var (
	senderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))
	botStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
)

type (
	errMsg    error
	answerMsg string
)

func main() {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("Missing OPENAI_API_KEY, you can find or create your API key here: https://platform.openai.com/account/api-keys")
	}

	bot := newChatGPT(apiKey)
	p := tea.NewProgram(initialModel(bot))
	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
}

type chatGPT struct {
	client      *gpt3.Client
	messages    []gpt3.ChatCompletionMessage
	totalTokens int
}

func newChatGPT(apiKey string) *chatGPT {
	client := gpt3.NewClient(apiKey)
	return &chatGPT{
		client: client,
		messages: []gpt3.ChatCompletionMessage{
			{
				Role:    "system",
				Content: "You are ChatGPT, a large language model trained by OpenAI. Answer as concisely as possible.",
			},
		},
	}
}

func (c *chatGPT) Ask(text string) tea.Cmd {
	return func() tea.Msg {
		resp, err := c.client.CreateChatCompletion(
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
		c.totalTokens = resp.Usage.TotalTokens
		message := resp.Choices[0].Message.Content
		return answerMsg(message)
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

func (c *chatGPT) Clear() {
	c.messages = c.messages[:1]
}

func (c *chatGPT) View() string {
	var sb strings.Builder
	for _, m := range c.messages {
		switch m.Role {
		case "user":
			sb.WriteString(senderStyle.Render("You: "))
			sb.WriteString(m.Content)
		case "assistant":
			sb.WriteString(botStyle.Render("ChatGPT: "))
			sb.WriteString(m.Content)
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

type model struct {
	viewport viewport.Model
	textarea textarea.Model
	err      error
	bot      *chatGPT
}

func initialModel(bot *chatGPT) model {
	ta := textarea.New()
	ta.Placeholder = "Send a message..."
	ta.Focus()

	ta.Prompt = "┃ "

	ta.SetWidth(30)
	ta.SetHeight(1)

	// Remove cursor line styling
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.ShowLineNumbers = false

	vp := viewport.New(45, 5)
	ta.KeyMap.InsertNewline.SetEnabled(true)

	return model{
		textarea: ta,
		viewport: vp,
		err:      nil,
		bot:      bot,
	}
}

func (m model) Init() tea.Cmd {
	return textarea.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		tiCmd  tea.Cmd
		vpCmd  tea.Cmd
		askCmd tea.Cmd
	)

	m.textarea, tiCmd = m.textarea.Update(msg)
	m.viewport, vpCmd = m.viewport.Update(msg)

	// TODO add help, clear
	// TODO shift+enter for new line
	// TODO render markdown
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			fmt.Println(m.textarea.Value())
			return m, tea.Quit
		case tea.KeyEnter:
			input := m.textarea.Value()
			if input == "" {
				break
			}
			m.bot.AddMessage("user", input)
			m.viewport.SetContent(m.bot.View())
			m.viewport.GotoBottom()
			m.textarea.Reset()
			askCmd = m.bot.Ask(input)
		case tea.KeyCtrlR:
			m.bot.Clear()
		}
	case answerMsg:
		m.bot.AddMessage("assistant", string(msg))
		m.viewport.SetContent(m.bot.View())
	case errMsg:
		m.err = msg
		return m, nil
	}

	return m, tea.Batch(tiCmd, vpCmd, askCmd)
}

func (m model) View() string {
	return fmt.Sprintf(
		"%s\n\n%s",
		m.viewport.View(),
		m.textarea.View(),
	) + "\n\n"
}
