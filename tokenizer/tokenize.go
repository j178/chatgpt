package tokenizer

import (
	"github.com/j178/llms/llms"
	"github.com/j178/tiktoken-go"
)

func CountTokens(model, text string) int {
	enc, err := tiktoken.ForModel(model)
	if err != nil {
		panic(err)
	}
	cnt, err := enc.Count(text)
	if err != nil {
		panic(err)
	}
	return cnt
}

// CountMessagesTokens based on https://github.com/openai/openai-cookbook/blob/main/examples/How_to_count_tokens_with_tiktoken.ipynb
func CountMessagesTokens(model string, messages []llms.MessageContent) int {
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

	for _ = range messages {
		tokens += tokensPerMessage
		// if _, ok := messages[k].
		//
		// 	tokens += CountTokens(model, messages[k].Role)
		// 	tokens += CountTokens(model, messages[k].Content)
		// 	tokens += CountTokens(model, messages[k].Name)
		// 	if messages[k].Name != "" {
		tokens += tokensPerName
		// 	}
	}

	tokens += 3 // every reply is primed with <|start|>assistant<|message|>

	return tokens
}
