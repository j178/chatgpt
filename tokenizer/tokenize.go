package tokenizer

import (
	"strings"

	"github.com/pkoukk/tiktoken-go"
	tiktokenloader "github.com/pkoukk/tiktoken-go-loader"
	"github.com/sashabaranov/go-openai"
)

var encodings = map[string]*tiktoken.Tiktoken{}

func init() {
	tiktoken.SetBpeLoader(tiktokenloader.NewOfflineLoader())
}

func getEncoding(model string) (*tiktoken.Tiktoken, error) {
	enc, ok := encodings[model]
	if !ok {
		var err error
		enc, err = tiktoken.EncodingForModel(model)
		if err != nil {
			return nil, err
		}
		encodings[model] = enc
	}
	return enc, nil
}

func CountTokens(model, text string) int {
	enc, err := getEncoding(model)
	if err != nil {
		return 0
	}
	cnt := len(enc.Encode(text, nil, nil))
	return cnt
}

// CountMessagesTokens based on https://github.com/openai/openai-cookbook/blob/main/examples/How_to_count_tokens_with_tiktoken.ipynb
func CountMessagesTokens(model string, messages []openai.ChatCompletionMessage) int {
	enc, err := getEncoding(model)
	if err != nil {
		return 0
	}

	var (
		tokens           int
		tokensPerMessage int
		tokensPerName    int
	)

	switch model {
	case "gpt-3.5-turbo-0613",
		"gpt-3.5-turbo-16k-0613",
		"gpt-4-0314",
		"gpt-4-32k-0314",
		"gpt-4-0613",
		"gpt-4-32k-0613":
		tokensPerMessage = 3
		tokensPerName = 1
	case "gpt-3.5-turbo-0301":
		tokensPerMessage = 4 // every message follows <|start|>{role/name}\n{content}<|end|>\n
		tokensPerName = -1   // if there's a name, the role is omitted
	default:
		switch {
		case strings.Contains(model, "gpt-3.5-turbo"):
			// gpt-3.5-turbo may update over time. Returning num tokens assuming gpt-3.5-turbo-0613.
			return CountMessagesTokens("gpt-3.5-turbo-0613", messages)
		case strings.Contains(model, "gpt-4"):
			// gpt-4 may update over time. Returning num tokens assuming gpt-4-0613.
			return CountMessagesTokens("gpt-4-0613", messages)
		default:
			return 0
		}
	}

	for k := range messages {
		tokens += tokensPerMessage

		tokens += len(enc.Encode(messages[k].Role, nil, nil))
		tokens += len(enc.Encode(messages[k].Content, nil, nil))
		tokens += len(enc.Encode(messages[k].Name, nil, nil))
		if messages[k].Name != "" {
			tokens += tokensPerName
		}
	}

	tokens += 3 // every reply is primed with <|start|>assistant<|message|>

	return tokens
}
