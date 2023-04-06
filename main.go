package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"
	"github.com/mitchellh/go-homedir"
	"github.com/postfinance/single"
	"github.com/sashabaranov/go-openai"
)

var (
	version              = "dev"
	date                 = "unknown"
	commit               = "HEAD"
	debug                = os.Getenv("DEBUG") == "1"
	promptKey            = flag.String("p", "", "Key of prompt defined in config file, or prompt itself")
	showVersion          = flag.Bool("v", false, "Show version")
	startNewConversation = flag.Bool("n", false, "Start new conversation")
	detachMode           = flag.Bool("d", false, "Run in detach mode, conversation will not be saved")
)

// TODO support switch model in TUI
// TODO support switch prompt in TUI

func main() {
	log.SetFlags(0)
	flag.Parse()
	if *showVersion {
		fmt.Print(buildVersion())
		return
	}

	conf, err := initConfig()
	if err != nil {
		exit(err)
	}
	if *promptKey != "" {
		conf.Conversation.Prompt = *promptKey
	}

	chatgpt := newChatGPT(conf)
	// One-time ask-and-response mode
	args := flag.Args()
	pipeIn := !isatty.IsTerminal(os.Stdin.Fd()) && !isatty.IsCygwinTerminal(os.Stdin.Fd())
	if pipeIn || len(args) > 0 {
		var question string
		if pipeIn {
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				exit(err)
			}
			question = string(data)
		} else {
			question = strings.Join(args, " ")
		}

		conversationConf := conf.Conversation
		err := chatgpt.ask(conversationConf, question)
		if err != nil {
			exit(err)
		}
		return
	}

	if !*detachMode {
		lockFile, _ := single.New("chatgpt")
		if err := lockFile.Lock(); err != nil {
			exit(
				fmt.Errorf(
					"Another chatgpt instance is running, chatgpt works not well with multiple instances, "+
						"please close the other one first. \n"+
						"If you are sure there is no other chatgpt instance running, please delete the lock file: %s\n"+
						"You can also try `chatgpt -d` to run in detach mode, this check will be skipped, but conversation will not be saved.",
					lockFile.Lockfile(),
				),
			)
		}
		defer lockFile.Unlock()
	}

	conversations := NewConversationManager(conf)

	if *startNewConversation {
		conversations.New(conf.Conversation)
	}

	p := tea.NewProgram(
		initialModel(chatgpt, conversations),
		// enable mouse motion will make text not able to select
		// tea.WithMouseCellMotion(),
		tea.WithAltScreen(),
	)
	if debug {
		f, _ := tea.LogToFile("chatgpt.log", "")
		defer func() { _ = f.Close() }()
	} else {
		log.SetOutput(io.Discard)
	}

	if _, err := p.Run(); err != nil {
		exit(err)
	}
}

func exit(err error) {
	_, _ = fmt.Fprintf(
		os.Stderr,
		"%s: %s\n",
		lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render("Error"),
		err.Error(),
	)
	os.Exit(1)
}

func buildVersion() string {
	result := version
	if commit != "" {
		result = fmt.Sprintf("%s\ncommit: %s", result, commit)
	}
	if date != "" {
		result = fmt.Sprintf("%s\nbuilt at: %s", result, date)
	}
	result = fmt.Sprintf("%s\ngoos: %s\ngoarch: %s", result, runtime.GOOS, runtime.GOARCH)
	return result
}

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
	Conversation ConversationConfig `json:"conversation"`
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

func containsCJK(s string) bool {
	for _, r := range s {
		if unicode.In(r, unicode.Han, unicode.Hangul, unicode.Hiragana, unicode.Katakana) {
			return true
		}
	}
	return false
}
