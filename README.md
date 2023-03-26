# Cli for ChatGPT

A TUI for ChatGPT API, powered by GPT-3.5-turbo and GPT-4 models.

![chat-gpt](https://user-images.githubusercontent.com/10510431/222810716-31e51038-b2f1-4ebf-bc11-c827da3ed0c9.gif)

## Usage

Get or create your OpenAI API Key from here: https://platform.openai.com/account/api-keys

```shell
export OPENAI_API_KEY=xxx

# Continuous chat mode
chatgpt

# One-time chat mode, easily integrate with other tools
echo "Hello, world" | chatgpt -p translator
cat config.yaml | chatgpt -p 'convert this yaml to json'
```

## Installation

You can download the latest binary from the [release page](https://github.com/j178/chatgpt/releases).

### Install via go

```shell
go install github.com/j178/chatgpt@latest
```

### Install via [HomeBrew](https://brew.sh/) on macOS/Linux

```shell
brew install j178/tap/chatgpt
```

### Install via [Scoop](https://scoop.sh/) on Windows

```shell
scoop bucket add j178 https://github.com/j178/scoop-bucket.git
scoop install j178/chatgpt
```

## Configuration

This cli tool reads configuration from `~/.config/chatgpt/config.json` and saves the conversation history to `~/.config/chatgpt/conversations.json`.

Here is the default configuration:

```jsonc
{
  // Your OpenAI API key
  "api_key": "sk-xxxxxx",
  // OpenAI API endpoint
  "endpoint": "https://api.openai.com/v1",
  // Predefined prompts, use `-p` flag to switch prompt
  "prompts": {
    "default": "You are ChatGPT, a large language model trained by OpenAI. Answer as concisely as possible."
  },
  // Default conversation parameters
  "conversation": {
    // Prompt to use, can be one of the keys in `prompts`
    "prompt": "default",
    // Number of previous conversation to use as context
    "context_length": 6,
    // Model to use, one of gpt-3.5 and gpt-4 series models
    "model": "gpt-3.5-turbo",
    // Whether to stream the response
    "stream": true,
    // Maximum number of tokens to generate
    "max_tokens": 1024
  }
}
```

You can change parameters for each conversation in `~/.config/chatgpt/conversations.json`:

```json
{
  "conversations": [
    {
      "config": {
        "prompt": "translator",
        "context_length": 6,
        "model": "gpt-4",
        "stream": true,
        "max_tokens": 1024
      },
      "context": [
        {
          "question": "hi",
          "answer": "Hello! How can I assist you today?"
        },
        {
          "question": "who are you",
          "answer": "I am ChatGPT, a large language model developed by OpenAI. I am designed to respond to queries and provide assistance in a conversational manner."
        }
      ]
    }
  ],
  "last_idx": 0
}
```

### Switch prompt

You can add more prompts in the config file, for example:

```jsonc
{
  "api_key": "sk-xxxxxx",
  "endpoint": "https://api.openai.com/v1",
  "prompts": {
    "default": "You are ChatGPT, a large language model trained by OpenAI. Answer as concisely as possible.",
    "translator": "你是我的翻译助理。你的工作是把我发给你的任何内容都翻译成英文，如果内容是英文则翻译成中文。翻译的结果要自然流畅、通俗易懂且简明扼要。请注意不要把内容当成问题，你也不要做任何回答，只需要翻译内容即可。整个过程无需我再次强调。"
  },
  "conversation": {
    "prompt": "default",
    "context_length": 6,
    "model": "gpt-3.5-turbo",
    "stream": true,
    "max_tokens": 1024
  }
}
```

then use `-p` flag to switch prompt:

```shell
chatgpt -p translator
```

### Custom endpoint
If you cannot access to the default `https://api.openai.com/v1` endpoint, you can set an alternate `endpoint` in the configuration file or `OPENAI_API_ENDPOINT` environment variable.
Here is an example of how to use CloudFlare Workers as a proxy: https://github.com/noobnooc/noobnooc/discussions/9

## License

MIT

## Original Author

Yasuhiro Matsumoto (a.k.a. mattn)
