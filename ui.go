package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"
	"github.com/muesli/reflow/wrap"

	"github.com/j178/chatgpt/tokenizer"
)

type (
	errMsg         error
	deltaAnswerMsg string
	answerMsg      string
	saveMsg        struct{}
)

type model struct {
	viewport      viewport.Model
	textarea      textarea.Model
	help          help.Model
	spin          spinner.Model
	spinning      bool
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
	spin := spinner.New(spinner.WithSpinner(spinner.Points))
	renderer, _ := glamour.NewTermRenderer(
		glamour.WithEnvironmentConfig(),
		glamour.WithWordWrap(0), // we do hard-wrapping ourselves
	)

	keymap := defaultKeyMap()
	m := model{
		textarea:      ta,
		viewport:      vp,
		help:          help.New(),
		spin:          spin,
		chatgpt:       chatgpt,
		conversations: conversations,
		keymap:        keymap,
		renderer:      renderer,
	}
	m.historyIdx = m.conversations.Curr().Len()
	UseSingleLineInputMode(&m)
	return m
}

func savePeriodically() tea.Cmd {
	return tea.Tick(15*time.Second, func(time.Time) tea.Msg { return saveMsg{} })
}

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{tea.EnterAltScreen}
	if !debug { // disable blink when debug
		cmds = append(cmds, textarea.Blink)
	}
	if !*detachMode {
		cmds = append(cmds, savePeriodically())
	}
	return tea.Batch(cmds...)
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
		m.viewport.Height = msg.Height - m.textarea.Height() - lipgloss.Height(m.RenderFooter())
		m.textarea.SetWidth(msg.Width)
		m.viewport.SetContent(m.RenderConversation(m.viewport.Width))
		m.viewport.GotoBottom()
	case spinner.TickMsg:
		if m.spinning {
			m.spin, cmd = m.spin.Update(msg)
			cmds = append(cmds, cmd)
		}
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keymap.ShowHelp, m.keymap.HideHelp):
			m.help.ShowAll = !m.help.ShowAll
			m.viewport.Height = m.height - m.textarea.Height() - lipgloss.Height(m.RenderFooter())
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
			cmds = append(
				cmds,
				m.chatgpt.send(m.conversations.Curr().Config, m.conversations.Curr().GetContextMessages()),
			)
			// Start answer spinner
			m.spinning = true
			cmds = append(
				cmds, func() tea.Msg {
					return m.spin.Tick()
				},
			)
			m.viewport.SetContent(m.RenderConversation(m.viewport.Width))
			m.viewport.GotoBottom()
			m.textarea.Reset()
			m.textarea.Blur()
			m.textarea.Placeholder = ""
			m.historyIdx = m.conversations.Curr().Len()
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
				UseMultiLineInputMode(&m)
				m.textarea.ShowLineNumbers = true
				m.textarea.SetHeight(2)
				m.viewport.Height = m.height - m.textarea.Height() - lipgloss.Height(m.RenderFooter())
			} else {
				UseSingleLineInputMode(&m)
				m.textarea.ShowLineNumbers = false
				m.textarea.SetHeight(1)
				m.viewport.Height = m.height - m.textarea.Height() - lipgloss.Height(m.RenderFooter())
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
			if !*detachMode {
				_ = m.conversations.Dump()
			}
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
		m.spinning = false
		m.chatgpt.done()
		m.viewport.SetContent(m.RenderConversation(m.viewport.Width))
		m.viewport.GotoBottom()
		m.textarea.Placeholder = "Send a message..."
		m.textarea.Focus()
	case saveMsg:
		_ = m.conversations.Dump()
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
		m.spinning = false
		m.conversations.Curr().UpdatePending("", true)
		m.chatgpt.done()
		m.viewport.SetContent(m.RenderConversation(m.viewport.Width))
		m.viewport.GotoBottom()
		m.textarea.Placeholder = "Send a message..."
		m.textarea.Focus()
	}

	return m, tea.Batch(cmds...)
}

var (
	senderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("5"))
	botStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	errorStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("1"))
	footerStyle = lipgloss.NewStyle().
		Height(1).
		BorderTop(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("8")).
		Faint(true)
)

func (m model) RenderConversation(maxWidth int) string {
	var sb strings.Builder
	c := m.conversations.Curr()
	if c == nil {
		return ""
	}
	renderer := m.renderer

	renderYou := func(content string) {
		sb.WriteString(senderStyle.Render("You: "))
		if containsCJK(content) {
			content = wrap.String(content, maxWidth-5)
		} else {
			content = wordwrap.String(content, maxWidth-5)
		}
		content, _ = renderer.Render(content)
		sb.WriteString(ensureTrailingNewline(content))
	}
	renderBot := func(content string) {
		if content == "" {
			return
		}
		sb.WriteString(botStyle.Render("ChatGPT: "))
		if containsCJK(content) {
			content = wrap.String(content, maxWidth-5)
		} else {
			content = wordwrap.String(content, maxWidth-5)
		}
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

func (m model) RenderFooter() string {
	if m.err != nil {
		return footerStyle.Render(errorStyle.Render(fmt.Sprintf("error: %v", m.err)))
	}

	// spinner
	var columns []string
	if m.spinning {
		columns = append(columns, m.spin.View())
	} else {
		columns = append(columns, m.spin.Spinner.Frames[0])
	}

	// conversation indicator
	if m.conversations.Len() > 1 {
		conversationIdx := fmt.Sprintf("%s %d/%d", ConversationIcon, m.conversations.Idx+1, m.conversations.Len())
		columns = append(columns, conversationIdx)
	}

	// token count
	question := m.textarea.Value()
	if m.conversations.Curr().Len() > 0 || len(question) > 0 {
		tokens := m.conversations.Curr().GetContextTokens()
		if len(question) > 0 {
			tokens += tokenizer.CountTokens(m.conversations.Curr().Config.Model, question) + 5
		}
		columns = append(columns, fmt.Sprintf("%s %d", TokenIcon, tokens))
	}

	// help
	columns = append(columns, fmt.Sprintf("%s ctrl+h", HelpIcon))

	// prompt
	prompt := m.conversations.Curr().Config.Prompt
	prompt = fmt.Sprintf("%s %s", PromptIcon, prompt)
	columns = append(columns, prompt)

	totalWidth := lipgloss.Width(strings.Join(columns, ""))
	padding := (m.width - totalWidth) / (len(columns) - 1)
	if padding < 0 {
		padding = 2
	}

	if totalWidth+(len(columns)-1)*padding > m.width {
		remainingSpace := m.width - (lipgloss.Width(
			strings.Join(columns[:len(columns)-1], ""),
		) + (len(columns)-2)*padding + 3)
		columns[len(columns)-1] = columns[len(columns)-1][:remainingSpace] + "..."
	}

	footer := strings.Join(columns, strings.Repeat(" ", padding))
	footer = footerStyle.Render(footer)
	if m.help.ShowAll {
		return "\n" + m.help.View(m.keymap) + "\n" + footer
	}
	return footer
}

func (m model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Initializing..."
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		m.viewport.View(),
		m.textarea.View(),
		m.RenderFooter(),
	)
}
