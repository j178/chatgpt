package main

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/avast/retry-go"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sashabaranov/go-openai"
)

type ChatGPT struct {
	globalConf GlobalConfig
	client     *openai.Client
	stream     *openai.ChatCompletionStream
	answering  bool
}

func newChatGPT(conf GlobalConfig) *ChatGPT {
	config := openai.DefaultConfig(conf.APIKey)
	config.OrgID = conf.OrgID
	if conf.Endpoint != "" {
		config.BaseURL = conf.Endpoint
	}
	if conf.APIType != openai.APITypeOpenAI {
		config.APIType = conf.APIType
		config.APIVersion = conf.APIVersion
		config.Engine = conf.Engine
	}
	client := openai.NewClientWithConfig(config)
	return &ChatGPT{globalConf: conf, client: client}
}

func (c *ChatGPT) ask(conf ConversationConfig, question string, out io.Writer) error {
	req := openai.ChatCompletionRequest{
		Model: conf.Model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: c.globalConf.LookupPrompt(conf.Prompt)},
			{Role: openai.ChatMessageRoleUser, Content: question},
		},
		MaxTokens:   conf.MaxTokens,
		Temperature: conf.Temperature,
		N:           1,
	}
	if conf.Stream {
		stream, err := c.client.CreateChatCompletionStream(context.Background(), req)
		if err != nil {
			return err
		}
		defer stream.Close()
		for {
			resp, err := stream.Recv()
			if err != nil {
				if errors.Is(err, io.EOF) {
					_, _ = fmt.Fprintln(out)
					break
				}
				return err
			}
			content := resp.Choices[0].Delta.Content
			_, _ = fmt.Fprint(out, content)
		}
	} else {
		resp, err := c.client.CreateChatCompletion(context.Background(), req)
		if err != nil {
			return err
		}
		content := resp.Choices[0].Message.Content
		_, _ = fmt.Fprintln(out, content)
	}
	return nil
}

func (c *ChatGPT) send(conf ConversationConfig, messages []openai.ChatCompletionMessage) tea.Cmd {
	c.answering = true
	return func() (msg tea.Msg) {
		err := retry.Do(
			func() error {
				req := openai.ChatCompletionRequest{
					Model:       conf.Model,
					Messages:    messages,
					MaxTokens:   conf.MaxTokens,
					Temperature: conf.Temperature,
					N:           1,
				}
				if conf.Stream {
					stream, err := c.client.CreateChatCompletionStream(context.Background(), req)
					c.stream = stream
					if err != nil {
						return errMsg(err)
					}
					resp, err := stream.Recv()
					if err != nil {
						return err
					}
					content := resp.Choices[0].Delta.Content
					msg = deltaAnswerMsg(content)
				} else {
					resp, err := c.client.CreateChatCompletion(context.Background(), req)
					if err != nil {
						return errMsg(err)
					}
					content := resp.Choices[0].Message.Content
					msg = answerMsg(content)
				}
				return nil
			},
			retry.Attempts(3),
			retry.LastErrorOnly(true),
		)
		if err != nil {
			return errMsg(err)
		}
		return
	}
}

func (c *ChatGPT) recv() tea.Cmd {
	return func() tea.Msg {
		resp, err := c.stream.Recv()
		if err != nil {
			return errMsg(err)
		}
		content := resp.Choices[0].Delta.Content
		return deltaAnswerMsg(content)
	}
}

func (c *ChatGPT) done() {
	if c.stream != nil {
		c.stream.Close()
	}
	c.stream = nil
	c.answering = false
}
