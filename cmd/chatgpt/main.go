package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"
	"github.com/postfinance/single"

	"github.com/j178/chatgpt"
	"github.com/j178/chatgpt/ui"
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

	ui.Debug = debug
	ui.DetachMode = *detachMode

	conf, err := chatgpt.InitConfig()
	if err != nil {
		exit(err)
	}

	// Set default prompt (for new conversations)
	if *promptKey != "" {
		conf.Conversation.Prompt = *promptKey
	}

	bot := chatgpt.NewChatGPT(conf)
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
		err := bot.Ask(conversationConf, question, os.Stdout)
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
		defer func() { _ = lockFile.Unlock() }()
	}

	conversations, err := chatgpt.NewConversationManager(conf, chatgpt.ConversationHistoryFile())
	if err != nil {
		exit(err)
	}

	if *startNewConversation {
		conversations.New(conf.Conversation)
	} else if *promptKey != "" {
		// If prompt is specified, try to find conversation with the same prompt.
		// If not found, start a new conversation
		conv := conversations.FindByPrompt(*promptKey)
		if conv == nil {
			conversations.New(conf.Conversation)
		} else {
			conversations.SetCurr(conv)
		}
	}

	p := tea.NewProgram(
		ui.InitialModel(conf, bot, conversations),
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
