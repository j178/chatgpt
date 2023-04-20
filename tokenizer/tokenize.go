package tokenizer

import (
	"github.com/pkoukk/tiktoken-go"
	"github.com/sashabaranov/go-openai"
)

var encoders = map[string]*tiktoken.Tiktoken{}

func CountTokens(model, text string) int {
	enc, ok := encoders[model]
	if !ok {
		enc, _ = tiktoken.EncodingForModel(model)
		encoders[model] = enc
	}
	return len(enc.Encode(text, nil, nil))
}

// CountMessagesTokens based on https://github.com/openai/openai-cookbook/blob/main/examples/How_to_count_tokens_with_tiktoken.ipynb
func CountMessagesTokens(model string, messages []openai.ChatCompletionMessage) int {
	var tokens int
	var tokensPerMessage int
	var tokensPerName int

	switch model {
	case openai.GPT3Dot5Turbo, openai.GPT3Dot5Turbo0301:
		tokensPerMessage = 4 // every message follows <|start|>{role/name}\n{content}<|end|>\n
		tokensPerName = -1   // if there's a name, the role is omitted
	case openai.GPT4, openai.GPT40314, openai.GPT432K, openai.GPT432K0314:
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
