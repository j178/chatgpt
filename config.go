package chatgpt

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mitchellh/go-homedir"
	"github.com/sashabaranov/go-openai"

	"github.com/j178/chatgpt/tokenizer"
)

type ConversationConfig struct {
	Prompt        string  `json:"prompt"`
	ContextLength int     `json:"context_length"`
	Model         string  `json:"model"`
	Stream        bool    `json:"stream"`
	Temperature   float32 `json:"temperature"`
	MaxTokens     int     `json:"max_tokens"`
}

type KeyMapConfig struct {
	SwitchMultiline        []string `json:"switch_multiline"`
	Submit                 []string `json:"submit,omitempty"`
	MultilineSubmit        []string `json:"multiline_submit,omitempty"`
	InsertNewline          []string `json:"insert_newline,omitempty"`
	MultilineInsertNewLine []string `json:"multiline_insert_newline,omitempty"`
	Help                   []string `json:"help,omitempty"`
	Quit                   []string `json:"quit,omitempty"`
	CopyLastAnswer         []string `json:"copy_last_answer,omitempty"`
	PreviousQuestion       []string `json:"previous_question,omitempty"`
	NextQuestion           []string `json:"next_question,omitempty"`
	NewConversation        []string `json:"new_conversation,omitempty"`
	PreviousConversation   []string `json:"previous_conversation,omitempty"`
	NextConversation       []string `json:"next_conversation,omitempty"`
	RemoveConversation     []string `json:"remove_conversation,omitempty"`
	ForgetContext          []string `json:"forget_context,omitempty"`
}

type GlobalConfig struct {
	APIKey       string             `json:"api_key"`
	Endpoint     string             `json:"endpoint"`
	APIType      openai.APIType     `json:"api_type,omitempty"`
	APIVersion   string             `json:"api_version,omitempty"`   // required when APIType is APITypeAzure or APITypeAzureAD
	ModelMapping map[string]string  `json:"model_mapping,omitempty"` // required when APIType is APITypeAzure or APITypeAzureAD
	OrgID        string             `json:"org_id,omitempty"`
	Prompts      map[string]string  `json:"prompts"`
	Conversation ConversationConfig `json:"conversation"` // Default conversation config
	KeyMap       KeyMapConfig       `json:"key_map"`
}

func (c *GlobalConfig) LookupPrompt(key string) string {
	prompt := c.Prompts[key]
	if prompt == "" {
		return key
	}
	return prompt
}

func ConversationHistoryFile() string {
	dir := configDir()
	return filepath.Join(dir, "conversations.json")
}

func configDir() string {
	if dir := os.Getenv("CHATGPT_CONFIG_DIR"); dir != "" {
		return dir
	}
	home, _ := homedir.Dir()
	return filepath.Join(home, ".config", "chatgpt")
}

func readOrWriteConfig(conf *GlobalConfig) error {
	dir := configDir()
	path := filepath.Join(dir, "config.json")

	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			err = os.MkdirAll(filepath.Dir(path), 0o755)
			if err != nil {
				return fmt.Errorf("failed to create config dir: %w", err)
			}
			f, err = os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
			if err != nil {
				return fmt.Errorf("failed to create config file: %w", err)
			}
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

func defaultKeyMapConfig() KeyMapConfig {
	return KeyMapConfig{
		SwitchMultiline:        []string{"ctrl+j"},
		Submit:                 []string{"enter"},
		InsertNewline:          []string{"ctrl+d"},
		MultilineSubmit:        []string{"ctrl+d"},
		MultilineInsertNewLine: []string{"enter"},
		Help:                   []string{"ctrl+h"},
		Quit:                   []string{"esc", "ctrl+c"},
		CopyLastAnswer:         []string{"ctrl+y"},
		PreviousQuestion:       []string{"ctrl+p"},
		NextQuestion:           []string{"ctrl+n"},
		NewConversation:        []string{"ctrl+t"},
		PreviousConversation:   []string{"ctrl+left", "ctrl+g"},
		NextConversation:       []string{"ctrl+right", "ctrl+o"},
		RemoveConversation:     []string{"ctrl+r"},
		ForgetContext:          []string{"ctrl+x"},
	}
}

func InitConfig() (GlobalConfig, error) {
	conf := GlobalConfig{
		APIType:  openai.APITypeOpenAI,
		Endpoint: "https://api.openai.com/v1",
		Prompts: map[string]string{
			"default":    "You are ChatGPT, a large language model trained by OpenAI. Answer as concisely as possible.",
			"translator": "I want you to act as an English translator, spelling corrector and improver. I will speak to you in any language and you will detect the language, translate it and answer in the corrected and improved version of my text, in English. I want you to replace my simplified A0-level words and sentences with more beautiful and elegant, upper level English words and sentences. The translation should be natural, easy to understand, and concise. Keep the meaning same, but make them more literary. I want you to only reply the correction, the improvements and nothing else, do not write explanations.",
			"shell":      "Return a one-line bash command with the functionality I will describe. Return ONLY the command ready to run in the terminal. The command should do the following:",
		},
		Conversation: ConversationConfig{
			Model:         openai.GPT3Dot5Turbo,
			Prompt:        "default",
			ContextLength: 6,
			Stream:        true,
			Temperature:   1.0,
			MaxTokens:     1024,
		},
		KeyMap: defaultKeyMapConfig(),
	}
	err := readOrWriteConfig(&conf)
	if err != nil {
		return GlobalConfig{}, err
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
		return GlobalConfig{}, errors.New("Missing API key. Set it in `~/.config/chatgpt/config.json` or by setting the `OPENAI_API_KEY` environment variable. You can find or create your API key at https://platform.openai.com/account/api-keys.")
	}

	conf.APIType = openai.APIType(strings.ToUpper(string(conf.APIType)))
	switch conf.APIType {
	case openai.APITypeOpenAI, openai.APITypeAzure, openai.APITypeAzureAD:
	default:
		return GlobalConfig{}, fmt.Errorf("unknown API type: %s", conf.APIType)
	}

	_, err = tokenizer.ForModel(conf.Conversation.Model)
	if err != nil {
		return GlobalConfig{}, fmt.Errorf("invalid model %s", conf.Conversation.Model)
	}
	return conf, nil
}
