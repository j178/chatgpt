package tokenizer

import (
	"github.com/sashabaranov/go-openai"
)

func CountTokens(model, text string) int {
	enc, err := ForModel(model)
	if err != nil {
		panic(err)
	}
	ids, _, err := enc.Encode(text)
	if err != nil {
		panic(err)
	}
	return len(ids)
}

// CountMessagesTokens based on https://github.com/openai/openai-cookbook/blob/main/examples/How_to_count_tokens_with_tiktoken.ipynb
func CountMessagesTokens(model string, messages []openai.ChatCompletionMessage) int {
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
	case "gpt-3.5-turbo":
		// gpt-3.5-turbo may update over time. Returning num tokens assuming gpt-3.5-turbo-0613.
		return CountMessagesTokens("gpt-3.5-turbo-0613", messages)
	case "gpt-4":
		// gpt-4 may update over time. Returning num tokens assuming gpt-4-0613
		return CountMessagesTokens("gpt-4-0613", messages)
	default:
		// not implemented
		return 0
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
