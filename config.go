package chatgpt

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/iancoleman/orderedmap"
	"github.com/j178/llms/llms/openai"
	"github.com/mitchellh/go-homedir"
)

const currentConfigVersion = 2
const defaultProviderName = "openai-1"
const defaultModel = "gpt-3.5-turbo"

type ProviderType string

const (
	ProviderOpenAI      ProviderType = "openai"
	ProviderGemini      ProviderType = "gemini"
	ProviderClaude      ProviderType = "claude"
	ProviderOllama      ProviderType = "ollama"
	ProviderCohere      ProviderType = "cohere"
	ProviderErnie       ProviderType = "ernie"
	ProviderHuggingFace ProviderType = "huggingface"
)

type ProviderConfig struct {
	Name string
	Type ProviderType
	KVs  map[string]any
}

func (c *ProviderConfig) MarshalJSON() ([]byte, error) {
	kvs := orderedmap.New()
	kvs.Set("type", string(c.Type))
	kvs.Set("name", c.Name)
	for k, v := range c.KVs {
		if !reflect.ValueOf(v).IsZero() {
			kvs.Set(k, v)
		}
	}
	return json.Marshal(kvs)
}

func getStr(kvs map[string]any, key string) (string, error) {
	v, ok := kvs[key]
	if !ok {
		return "", nil
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("invalid value type for key %s, expected string", key)
	}
	return s, nil
}

func (c *ProviderConfig) UnmarshalJSON(data []byte) error {
	var kvs map[string]any
	err := json.Unmarshal(data, &kvs)
	if err != nil {
		return err
	}
	ty, err := getStr(kvs, "type")
	if err != nil {
		return err
	}
	c.Type = ProviderType(ty)
	c.Name, err = getStr(kvs, "name")
	if err != nil {
		return err
	}
	delete(kvs, "type")
	delete(kvs, "name")

	c.KVs = kvs
	return nil
}

type ConversationConfig struct {
	Provider      string  `json:"provider"`
	Model         string  `json:"model,omitempty"`
	Prompt        string  `json:"prompt"`
	ContextLength int     `json:"context_length"`
	Stream        bool    `json:"stream"`
	Temperature   float64 `json:"temperature"`
	MaxTokens     int     `json:"max_tokens"`
}

type KeyMapConfig struct {
	SwitchMultiline        []string `json:"switch_multiline,omitempty"`
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

type LegacyV0Config struct {
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

type GlobalConfig struct {
	Version             int                `json:"version"`
	Providers           []ProviderConfig   `json:"providers"`
	DefaultConversation ConversationConfig `json:"default_conversation"`
	Prompts             map[string]string  `json:"prompts"`
	KeyMap              KeyMapConfig       `json:"key_map"`
}

func (c *GlobalConfig) LookupPrompt(key string) string {
	prompt := c.Prompts[key]
	if prompt == "" {
		return key
	}
	return prompt
}

func ConversationsFile() string {
	dir := ConfigDir()
	return filepath.Join(dir, "conversations.json")
}

func ConfigFile() string {
	dir := ConfigDir()
	return filepath.Join(dir, "config.json")
}

func ConfigDir() string {
	if dir := os.Getenv("CHATGPT_CONFIG_DIR"); dir != "" {
		return dir
	}
	home, _ := homedir.Dir()
	return filepath.Join(home, ".config", "chatgpt")
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

func defaultConfig() *GlobalConfig {
	return &GlobalConfig{
		Version: currentConfigVersion,
		Providers: []ProviderConfig{
			{
				Type: ProviderOpenAI,
				Name: defaultProviderName,
				KVs:  map[string]any{"base_url": "https://api.openai.com/v1"},
			},
		},
		DefaultConversation: ConversationConfig{
			Provider:      defaultProviderName,
			Model:         defaultModel,
			Prompt:        "default",
			ContextLength: 6,
			Stream:        true,
			Temperature:   1.0,
			MaxTokens:     4096,
		},
		KeyMap: defaultKeyMapConfig(),
		Prompts: map[string]string{
			"default":    "You are ChatGPT, a large language model trained by OpenAI. Answer as concisely as possible.",
			"translator": "I want you to act as an English translator, spelling corrector and improver. I will speak to you in any language and you will detect the language, translate it and answer in the corrected and improved version of my text, in English. I want you to replace my simplified A0-level words and sentences with more beautiful and elegant, upper level English words and sentences. The translation should be natural, easy to understand, and concise. Keep the meaning same, but make them more literary. I want you to only reply the correction, the improvements and nothing else, do not write explanations.",
			"shell":      "Return a one-line bash command with the functionality I will describe. Return ONLY the command ready to run in the terminal. The command should do the following:",
		},
	}
}

func defaultV0Config() *LegacyV0Config {
	return &LegacyV0Config{
		APIType:  openai.APITypeOpenAI,
		Endpoint: "https://api.openai.com/v1",
		Prompts: map[string]string{
			"default":    "You are ChatGPT, a large language model trained by OpenAI. Answer as concisely as possible.",
			"translator": "I want you to act as an English translator, spelling corrector and improver. I will speak to you in any language and you will detect the language, translate it and answer in the corrected and improved version of my text, in English. I want you to replace my simplified A0-level words and sentences with more beautiful and elegant, upper level English words and sentences. The translation should be natural, easy to understand, and concise. Keep the meaning same, but make them more literary. I want you to only reply the correction, the improvements and nothing else, do not write explanations.",
			"shell":      "Return a one-line bash command with the functionality I will describe. Return ONLY the command ready to run in the terminal. The command should do the following:",
		},
		Conversation: ConversationConfig{
			Model:         defaultModel,
			Prompt:        "default",
			ContextLength: 6,
			Stream:        true,
			Temperature:   1.0,
			MaxTokens:     4096,
		},
		KeyMap: defaultKeyMapConfig(),
	}
}

func readConfig() (*GlobalConfig, error) {
	path := ConfigFile()
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}

	// Read version first
	version := struct {
		Version int `json:"version"`
	}{}
	err = json.Unmarshal(content, &version)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	if version.Version == 0 || version.Version == 1 {
		err = migrateV1Config(content)
		if err != nil {
			return nil, fmt.Errorf("failed to migrate config file: %w", err)
		}
	}

	// Read again
	content, err = os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	var conf GlobalConfig
	err = json.Unmarshal(content, &conf)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	return &conf, nil
}

func writeConfig(conf *GlobalConfig) error {
	path := ConfigFile()
	err := CreateIfNotExists(path, false)
	if err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return fmt.Errorf("failed to open config file: %w", err)
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

func isAzure(apiType openai.APIType) bool {
	apiType = openai.APIType(strings.ToUpper(string(apiType)))
	return apiType == openai.APITypeAzure || apiType == openai.APITypeAzureAD
}

func convertModelToAzureDeployment(model string, mapping map[string]string) string {
	m, ok := mapping[model]
	if ok {
		return m
	}
	// Fallback to use model name (without . or : ) as deployment name.
	return regexp.MustCompile(`[.:]`).ReplaceAllString(model, "")
}

func migrateV1Config(data []byte) error {
	conf := defaultConfig()
	v0 := defaultV0Config()
	err := json.Unmarshal(data, v0)
	if err != nil {
		return err
	}

	modelKey := "default_model"
	model := v0.Conversation.Model
	azure := isAzure(v0.APIType)
	if azure {
		// For azure, in new version, instead of mapping model to deployment,
		// we use explicit deployment parameter, and model is not used at all.
		modelKey = "deployment"
		model = convertModelToAzureDeployment(model, v0.ModelMapping)
		v0.Conversation.Model = ""
	}
	v0.Conversation.Provider = defaultProviderName
	if v0.APIType == openai.APITypeOpenAI {
		v0.APIType = ""
	} else {
		v0.APIType = openai.APIType(strings.ToUpper(string(v0.APIType)))
	}

	conf.Version = currentConfigVersion
	conf.Prompts = v0.Prompts
	conf.DefaultConversation = v0.Conversation
	conf.KeyMap = v0.KeyMap
	conf.Providers = []ProviderConfig{
		{
			Type: ProviderOpenAI,
			Name: defaultProviderName,
			KVs: map[string]any{
				"base_url":     v0.Endpoint,
				"api_key":      v0.APIKey,
				"api_type":     v0.APIType,
				"api_version":  v0.APIVersion,
				"organization": v0.OrgID,
				modelKey:       model,
			},
		},
	}
	err = writeConfig(conf)
	if err != nil {
		return err
	}

	// Migrate conversation config
	conversations, err := NewConversationManager(conf, ConversationsFile())
	if errors.Is(err, os.ErrNotExist) {
		err = nil
	}
	if err != nil {
		return err
	}
	for _, conv := range conversations.Conversations {
		conv.Config.Provider = defaultProviderName
		if azure {
			// For azure, we use `deployment` in new version, `model` is no effect.
			conv.Config.Model = ""
		}
	}
	err = conversations.Dump()
	return err
}

func InitConfig() (*GlobalConfig, error) {
	conf, err := readConfig()
	if errors.Is(err, os.ErrNotExist) {
		conf = defaultConfig()
		err = writeConfig(conf)
		return conf, err
	}

	names := map[string]struct{}{}
	for i, provider := range conf.Providers {
		if provider.Type == "" {
			return nil, fmt.Errorf("type of provider %d is empty", i+1)
		}
		if provider.Name == "" {
			return nil, fmt.Errorf("name of provider %d is empty", i+1)
		}
		if _, ok := names[provider.Name]; ok {
			return nil, fmt.Errorf("duplicate provider name: %s", provider.Name)
		}
		names[provider.Name] = struct{}{}
	}

	return conf, err
}
