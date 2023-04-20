package chatgpt

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/mitchellh/go-homedir"
	"github.com/sashabaranov/go-openai"
)

type ConversationConfig struct {
	Prompt        string  `json:"prompt"`
	ContextLength int     `json:"context_length"`
	Model         string  `json:"model"`
	Stream        bool    `json:"stream"`
	Temperature   float32 `json:"temperature"`
	MaxTokens     int     `json:"max_tokens"`
}

type GlobalConfig struct {
	APIKey       string             `json:"api_key"`
	Endpoint     string             `json:"endpoint"`
	APIType      openai.APIType     `json:"api_type,omitempty"`
	APIVersion   string             `json:"api_version,omitempty"` // required when APIType is APITypeAzure or APITypeAzureAD
	Engine       string             `json:"engine,omitempty"`      // required when APIType is APITypeAzure or APITypeAzureAD
	OrgID        string             `json:"org_id,omitempty"`
	Prompts      map[string]string  `json:"prompts"`
	Conversation ConversationConfig `json:"conversation"` // Default conversation config
}

func (c GlobalConfig) LookupPrompt(key string) string {
	prompt := c.Prompts[key]
	if prompt == "" {
		return key
	}
	return prompt
}

func configDir() (string, error) {
	if dir := os.Getenv("CHATGPT_CONFIG_DIR"); dir != "" {
		return dir, nil
	}

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
		return GlobalConfig{}, errors.New("Missing API key. Set it in `~/.config/chatgpt/config.json` or by setting the `OPENAI_API_KEY` environment variable. You can find or create your API key at https://platform.openai.com/account/api-keys.")
	}
	// TODO: support non-chat models
	switch conf.Conversation.Model {
	case openai.GPT3Dot5Turbo0301, openai.GPT3Dot5Turbo, openai.GPT4, openai.GPT40314, openai.GPT432K0314, openai.GPT432K:
	default:
		return GlobalConfig{}, errors.New("Invalid model, please choose one of the following: gpt-3.5-turbo-0301, gpt-3.5-turbo, gpt-4, gpt-4-0314, gpt-4-32k-0314, gpt-4-32k")
	}
	return conf, nil
}
