package chatgpt

import (
	"context"
	"fmt"
	"io"

	"github.com/j178/llms/llms"
	"github.com/j178/llms/llms/googleai"
	"github.com/j178/llms/llms/openai"
	"github.com/j178/llms/schema"
)

type ChatGPT struct {
	conf *GlobalConfig
	llms map[string]llms.Model
}

func New(conf *GlobalConfig) (*ChatGPT, error) {
	providers := make(map[string]llms.Model)
	for _, p := range conf.Providers {
		var (
			err error
			llm llms.Model
		)
		switch p.Type {
		case ProviderOpenAI:
			llm, err = newOpenAI(p.KVs)
		case ProviderGemini:
			llm, err = newGoogleAI(p.KVs)
		}
		if err != nil {
			return nil, err
		}
		providers[p.Name] = llm
	}

	return &ChatGPT{conf: conf, llms: providers}, nil
}

func newOpenAI(kvs map[string]any) (*openai.LLM, error) {
	var opts []openai.Option
	for k, v := range kvs {
		switch k {
		// TODO fix potential type conversion issue
		case "api_key":
			opts = append(opts, openai.WithToken(v.(string)))
		case "base_url":
			opts = append(opts, openai.WithBaseURL(v.(string)))
		case "organization":
			opts = append(opts, openai.WithOrganization(v.(string)))
		case "api_type":
			opts = append(opts, openai.WithAPIType(openai.APIType(v.(string))))
		case "api_version":
			opts = append(opts, openai.WithAPIVersion(v.(string)))
		case "model":
			opts = append(opts, openai.WithModel(v.(string)))
		}
	}
	return openai.New(opts...)
}

// TODO 搞清楚 GoogleAI, Vertex, Palm 的区别
func newGoogleAI(kvs map[string]any) (*googleai.GoogleAI, error) {
	var opts []googleai.Option
	for k, v := range kvs {
		switch k {
		case "api_key":
			opts = append(opts, googleai.WithAPIKey(v.(string)))
			// case "base_url":
			// 	opts = append(opts, googleai.WithBaseURL(v.(string)))
			// case "api_type":
			// 	opts = append(opts, googleai.WithAPIType(googleai.APIType(v.(string))))
			// case "api_version":
			// 	opts = append(opts, googleai.WithAPIVersion(v.(string)))
			// case "model":
			// 	opts = append(opts, googleai.WithModel(v.(string)))
		}
	}
	return googleai.New(context.Background(), opts...)
}

func message(role schema.ChatMessageType, msg string) llms.MessageContent {
	return llms.MessageContent{
		Role:  role,
		Parts: []llms.ContentPart{llms.TextPart(msg)},
	}
}

func (c *ChatGPT) Ask(ctx context.Context, conf ConversationConfig, question string, out io.Writer) error {
	llm := c.llms[conf.Provider]
	if llm == nil {
		return fmt.Errorf("unknown provider: %s", conf.Provider)
	}

	messages := []llms.MessageContent{
		message(schema.ChatMessageTypeSystem, c.conf.LookupPrompt(conf.Prompt)),
		message(schema.ChatMessageTypeHuman, question),
	}
	opts := []llms.CallOption{
		llms.WithModel(conf.Model),
		llms.WithMaxTokens(conf.MaxTokens),
		llms.WithTemperature(conf.Temperature),
		llms.WithN(1),
	}
	if conf.Stream {
		opts = append(
			opts, llms.WithStreamingFunc(
				func(ctx context.Context, chunk []byte) error {
					_, err := out.Write(chunk)
					return err
				},
			),
		)
	}

	content, err := llm.GenerateContent(ctx, messages, opts...)
	if err != nil {
		return err
	}
	if !conf.Stream {
		_, err = out.Write([]byte(content.Choices[0].Content))
	}
	return nil
}

func (c *ChatGPT) Send(
	ctx context.Context,
	conf ConversationConfig,
	messages []llms.MessageContent,
	stream func(chunk []byte, done bool),
) (string, error) {
	llm := c.llms[conf.Provider]
	if llm == nil {
		return "", fmt.Errorf("unknown provider: %s", conf.Provider)
	}

	opts := []llms.CallOption{
		llms.WithModel(conf.Model),
		llms.WithMaxTokens(conf.MaxTokens),
		llms.WithTemperature(conf.Temperature),
		llms.WithN(1),
	}
	if conf.Stream {
		opts = append(
			opts, llms.WithStreamingFunc(
				func(ctx context.Context, chunk []byte) error {
					stream(chunk, false)
					return nil
				},
			),
		)
	}
	resp, err := llm.GenerateContent(ctx, messages, opts...)
	if err != nil {
		return "", err
	}
	if conf.Stream {
		stream(nil, true)
		return "", nil
	} else {
		return resp.Choices[0].Content, nil
	}
}
