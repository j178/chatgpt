package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"

	"github.com/charmbracelet/glamour"
	gpt3 "github.com/sashabaranov/go-gpt3"
)

const maxTokens = 4096

func main() {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("Missing OPENAI_API_KEY, you can find or create your API key here: https://platform.openai.com/account/api-keys")
	}

	client := gpt3.NewClient(apiKey)
	scanner := bufio.NewScanner(os.Stdin)
	messages := []gpt3.ChatCompletionMessage{
		{
			Role:    "system",
			Content: "You are ChatGPT, a large language model trained by OpenAI. Answer as concisely as possible.",
		},
	}
	totalTokens := 0

	for {
		fmt.Printf("(%d/%d)> ", totalTokens, maxTokens)
		if !scanner.Scan() {
			break
		}
		text := scanner.Text()
		messages = append(
			messages, gpt3.ChatCompletionMessage{
				Role:    "user",
				Content: text,
			},
		)
		resp, err := client.CreateChatCompletion(
			context.Background(),
			gpt3.ChatCompletionRequest{
				Model:       gpt3.GPT3Dot5Turbo,
				Messages:    messages,
				MaxTokens:   3000,
				Temperature: 0,
				N:           1,
			},
		)
		if err != nil {
			fmt.Println(err)
			continue
		}
		totalTokens = resp.Usage.TotalTokens
		message := resp.Choices[0].Message.Content

		rendered, _ := glamour.RenderWithEnvironmentConfig(message)
		fmt.Println(rendered)
		messages = append(
			messages, gpt3.ChatCompletionMessage{
				Role:    "assistant",
				Content: message,
			},
		)
	}
}
