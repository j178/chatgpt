package tokenizer

import (
	"github.com/j178/llms/llms"
	"github.com/j178/tiktoken-go"
)

const countAsModel = "gpt-4"

func CountTokens(text string) int {
	enc, err := tiktoken.ForModel(countAsModel)
	if err != nil {
		return 0
	}
	cnt, _ := enc.Count(text)
	return cnt
}

// CountMessagesTokens based on https://github.com/openai/openai-cookbook/blob/main/examples/How_to_count_tokens_with_tiktoken.ipynb
// But simplified to only count as GPT-4 series models.
func CountMessagesTokens(messages []llms.MessageContent) (tokens int) {
	for _, message := range messages {
		tokens += 3
		tokens += CountTokens(string(message.Role))
		for _, part := range message.Parts {
			// TODO how to count binary and image URL parts?
			if text, ok := part.(llms.TextContent); ok {
				tokens += CountTokens(text.Text)
			}
		}
	}

	tokens += 3 // every reply is primed with <|start|>assistant<|message|>

	return tokens
}
