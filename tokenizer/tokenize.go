package tokenizer

import (
	"strings"

	"github.com/pkoukk/tiktoken-go"
	"github.com/sashabaranov/go-openai"
)

var encoders = map[string]*tiktoken.Tiktoken{}

func getEncoding(model string) (*tiktoken.Tiktoken, error) {
	enc, ok := encoders[model]
	var err error
	if !ok {
		enc, err = tiktoken.EncodingForModel(model)
		if err != nil {
			return nil, err
		}
		encoders[model] = enc
	}
	return enc, nil
}

func CheckModel(model string) error {
	_, err := getEncoding(model)
	return err
}

func CountTokens(model, text string) int {
	enc, err := getEncoding(model)
	if err != nil {
		panic(err)
	}
	return len(enc.Encode(text, nil, nil))
}

// CountMessagesTokens based on https://github.com/openai/openai-cookbook/blob/main/examples/How_to_count_tokens_with_tiktoken.ipynb
func CountMessagesTokens(model string, messages []openai.ChatCompletionMessage) int {
	var (
		tokens           int
		tokensPerMessage int
		tokensPerName    int
	)

	if strings.HasPrefix(model, "gpt-3.5") {
		tokensPerMessage = 4 // every message follows <|start|>{role/name}\n{content}<|end|>\n
		tokensPerName = -1   // if there's a name, the role is omitted
	} else if strings.HasPrefix(model, "gpt-4") {
		tokensPerMessage = 3
		tokensPerName = 1
	}

	for k := range messages {
		tokens += tokensPerMessage

		tokens += CountTokens(model, messages[k].Role)
		tokens += CountTokens(model, messages[k].Content)
		tokens += CountTokens(model, messages[k].Name)
		if messages[k].Name != "" {
			tokens += tokensPerName
		}
	}

	tokens += 3 // every reply is primed with <|start|>assistant<|message|>

	return tokens
}
