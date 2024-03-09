package chatgpt

import (
	"context"
	"fmt"
	"io"

	"github.com/j178/llms/llms"
	"github.com/j178/llms/llms/anthropic"
	"github.com/j178/llms/llms/cohere"
	"github.com/j178/llms/llms/ernie"
	"github.com/j178/llms/llms/googleai"
	"github.com/j178/llms/llms/huggingface"
	"github.com/j178/llms/llms/ollama"
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
		case ProviderAzureOpenAI:
			llm, err = newAzureOpenAI(p.KVs)
		case ProviderGemini:
			llm, err = newGemini(p.KVs)
		case ProviderClaude:
			llm, err = newClaude(p.KVs)
		case ProviderOllama:
			llm, err = newOllama(p.KVs)
		case ProviderCohere:
			llm, err = newCohere(p.KVs)
		case ProviderHuggingFace:
			llm, err = newHuggingFace(p.KVs)
		case ProviderErnie:
			llm, err = newErnie(p.KVs)
		}
		if err != nil {
			return nil, err
		}
		providers[p.Name] = llm
	}

	return &ChatGPT{conf: conf, llms: providers}, nil
}

func collectOpts[T any](kvs map[string]any, optFuncs map[string]func(string) T) ([]T, error) {
	var opts []T
	for k, v := range kvs {
		v, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("invalid value type for key %s", k)
		}
		if f, ok := optFuncs[k]; ok {
			opts = append(opts, f(v))
		}
	}
	return opts, nil
}

func newOpenAI(kvs map[string]any) (*openai.LLM, error) {
	optFuncs := map[string]func(string) openai.Option{
		"api_key":       openai.WithToken,
		"base_url":      openai.WithBaseURL,
		"organization":  openai.WithOrganization,
		"default_model": openai.WithModel,
	}
	opts, err := collectOpts(kvs, optFuncs)
	if err != nil {
		return nil, err
	}
	return openai.New(opts...)
}

func newAzureOpenAI(kvs map[string]any) (*openai.LLM, error) {
	optFuncs := map[string]func(string) openai.Option{
		"api_key":  openai.WithToken,
		"base_url": openai.WithBaseURL,
		"api_type": func(s string) openai.Option {
			return openai.WithAPIType(openai.APIType(s))
		},
		"api_version": openai.WithAPIVersion,
		"deployment":  openai.WithDeploymentName,
	}
	opts, err := collectOpts(kvs, optFuncs)
	if err != nil {
		return nil, err
	}
	return openai.New(opts...)
}

func newClaude(kvs map[string]any) (*anthropic.LLM, error) {
	optFuncs := map[string]func(string) anthropic.Option{
		"api_key":       anthropic.WithToken,
		"default_model": anthropic.WithModel,
	}
	opts, err := collectOpts(kvs, optFuncs)
	if err != nil {
		return nil, err
	}
	return anthropic.New(opts...)
}

func newOllama(kvs map[string]any) (*ollama.LLM, error) {
	optFuncs := map[string]func(string) ollama.Option{
		"base_url":      ollama.WithServerURL,
		"default_model": ollama.WithModel,
	}
	opts, err := collectOpts(kvs, optFuncs)
	if err != nil {
		return nil, err
	}
	return ollama.New(opts...)
}

func newGemini(kvs map[string]any) (*googleai.GoogleAI, error) {
	optFuncs := map[string]func(string) googleai.Option{
		"api_key":       googleai.WithAPIKey,
		"default_model": googleai.WithDefaultModel,
	}
	opts, err := collectOpts(kvs, optFuncs)
	if err != nil {
		return nil, err
	}
	return googleai.New(context.Background(), opts...)
}

func newCohere(kvs map[string]any) (*cohere.LLM, error) {
	optFuncs := map[string]func(string) cohere.Option{
		"api_key":       cohere.WithToken,
		"base_url":      cohere.WithBaseURL,
		"default_model": cohere.WithModel,
	}
	opts, err := collectOpts(kvs, optFuncs)
	if err != nil {
		return nil, err
	}
	return cohere.New(opts...)
}

func newHuggingFace(kvs map[string]any) (*huggingface.LLM, error) {
	optFuncs := map[string]func(string) huggingface.Option{
		"api_key":       huggingface.WithToken,
		"base_url":      huggingface.WithURL,
		"default_model": huggingface.WithModel,
	}
	opts, err := collectOpts(kvs, optFuncs)
	if err != nil {
		return nil, err
	}
	return huggingface.New(opts...)
}

func newErnie(kvs map[string]any) (*ernie.LLM, error) {
	optFuncs := map[string]func(string) ernie.Option{
		"access_token": ernie.WithAccessToken,
		"default_model": func(s string) ernie.Option {
			return ernie.WithModelName(ernie.ModelName(s))
		},
	}
	opts, err := collectOpts(kvs, optFuncs)
	if err != nil {
		return nil, err
	}
	apiKey, err := getStr(kvs, "api_key")
	if err != nil {
		return nil, err
	}
	secretKey, err := getStr(kvs, "secret_key")
	if err != nil {
		return nil, err
	}
	opts = append(opts, ernie.WithAKSK(apiKey, secretKey))

	return ernie.New(opts...)
}

func (c *ChatGPT) Ask(ctx context.Context, conf ConversationConfig, question string, out io.Writer) error {
	llm := c.llms[conf.Provider]
	if llm == nil {
		return fmt.Errorf("unknown provider: %s", conf.Provider)
	}

	messages := []llms.MessageContent{
		llms.TextParts(schema.ChatMessageTypeSystem, c.conf.LookupPrompt(conf.Prompt)),
		llms.TextParts(schema.ChatMessageTypeHuman, question),
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
		stream([]byte(resp.Choices[0].Content), true)
		return resp.Choices[0].Content, nil
	}
}
