package chatgpt

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/j178/llms/llms"
	"github.com/j178/llms/schema"
)

type ConversationManager struct {
	file          string
	conf          *GlobalConfig
	Conversations []*Conversation `json:"conversations"`
	Idx           int             `json:"last_idx"`
}

func NewConversationManager(conf *GlobalConfig, historyFile string) (*ConversationManager, error) {
	h := &ConversationManager{
		file: historyFile,
		conf: conf,
		Idx:  -1,
	}

	err := h.Load()
	if err != nil {
		return nil, fmt.Errorf("Failed to load conversation history: %w", err)
	}
	return h, nil
}

func (m *ConversationManager) Dump() error {
	if m.file == "" {
		return nil
	}
	err := CreateIfNotExists(m.file, false)
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
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
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

func (m *ConversationManager) FindByPrompt(prompt string) *Conversation {
	prompt = m.conf.LookupPrompt(prompt)
	for _, c := range m.Conversations {
		if m.conf.LookupPrompt(c.Config.Prompt) == prompt {
			return c
		}
	}
	return nil
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

func (m *ConversationManager) SetCurr(conv *Conversation) {
	idx := -1
	for i, c := range m.Conversations {
		if c == conv {
			idx = i
			break
		}
	}
	if idx == -1 {
		return
	}
	m.Idx = idx
}

func (m *ConversationManager) Len() int {
	return len(m.Conversations)
}

func (m *ConversationManager) Curr() *Conversation {
	if len(m.Conversations) == 0 {
		// create initial conversation using default config
		return m.New(m.conf.DefaultConversation)
	}
	return m.Conversations[m.Idx]
}

func (m *ConversationManager) Prev() *Conversation {
	if len(m.Conversations) == 0 {
		return nil
	}
	m.Idx--
	if m.Idx < 0 {
		m.Idx = 0 // don't wrap around
	}
	return m.Conversations[m.Idx]
}

func (m *ConversationManager) Next() *Conversation {
	if len(m.Conversations) == 0 {
		return nil
	}
	m.Idx++
	if m.Idx >= len(m.Conversations) {
		m.Idx = len(m.Conversations) - 1 // don't wrap around
	}
	return m.Conversations[m.Idx]
}

type QnA struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

type Conversation struct {
	manager       *ConversationManager
	contextTokens int
	Config        ConversationConfig `json:"config"`
	Forgotten     []QnA              `json:"forgotten,omitempty"`
	Context       []QnA              `json:"context,omitempty"`
	Pending       *QnA               `json:"pending,omitempty"`
}

func (c *Conversation) AddQuestion(q string) {
	c.Pending = &QnA{Question: q}
	c.contextTokens = 0
}

func (c *Conversation) UpdatePending(ans string, done bool) {
	if c.Pending == nil {
		return
	}
	c.Pending.Answer += ans
	if done {
		c.Context = append(c.Context, *c.Pending)
		c.contextTokens = 0
		if len(c.Context) > c.Config.ContextLength {
			c.Forgotten = append(c.Forgotten, c.Context[0])
			c.Context = c.Context[1:]
		}
		c.Pending = nil
	}
}

func (c *Conversation) GetContextMessages() []llms.MessageContent {
	messages := make([]llms.MessageContent, 0, 2*len(c.Context)+2)
	messages = append(
		messages, message(schema.ChatMessageTypeSystem, c.manager.conf.LookupPrompt(c.Config.Prompt)),
	)
	for _, qna := range c.Context {
		messages = append(messages, message(schema.ChatMessageTypeHuman, qna.Question))
		messages = append(messages, message(schema.ChatMessageTypeAI, qna.Answer))
	}
	if c.Pending != nil {
		messages = append(messages, message(schema.ChatMessageTypeHuman, c.Pending.Question))
	}
	return messages
}

func (c *Conversation) GetContextTokens() int {
	if c.contextTokens == 0 {
		// c.contextTokens = tokenizer.CountMessagesTokens(c.Config.Model, c.GetContextMessages())
	}
	return c.contextTokens
}

func (c *Conversation) ForgetContext() {
	c.Forgotten = append(c.Forgotten, c.Context...)
	c.Context = nil
	c.contextTokens = 0
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
	l := len(c.Forgotten) + len(c.Context)
	if c.Pending != nil {
		l++
	}
	return l
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
