package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/PullRequestInc/go-gpt3"
)

func main() {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("Missing OPENAI_API_KEY")
	}

	client := gpt3.NewClient(apiKey)
	scanner := bufio.NewScanner(os.Stdin)
	messages := []gpt3.ChatCompletionRequestMessage{
		{
			Role:    "system",
			Content: "You are ChatGPT, a large language model trained by OpenAI. Answer as concisely as possible.",
		},
	}
	var respBuf strings.Builder
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}
		text := scanner.Text()
		messages = append(
			messages, gpt3.ChatCompletionRequestMessage{
				Role:    "user",
				Content: text,
			},
		)
		err := client.ChatCompletionStream(
			context.Background(),
			gpt3.ChatCompletionRequest{
				Model:       gpt3.GPT3Dot5Turbo,
				Messages:    messages,
				MaxTokens:   3000,
				Temperature: 0,
			}, func(resp *gpt3.ChatCompletionStreamResponse) {
				content := resp.Choices[0].Delta.Content
				respBuf.WriteString(content)
				fmt.Print(content)
			},
		)
		if err != nil {
			fmt.Println(err)
			continue
		}
		messages = append(
			messages, gpt3.ChatCompletionRequestMessage{
				Role:    "assistant",
				Content: respBuf.String(),
			},
		)
		respBuf.Reset()
		fmt.Println()
	}
}
