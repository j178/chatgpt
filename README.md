# Cli for ChatGPT

A simple cli wrapper for ChatGPT API, powered by GPT-3.5-turbo model.

![chat-gpt](https://user-images.githubusercontent.com/10510431/222810716-31e51038-b2f1-4ebf-bc11-c827da3ed0c9.gif)


## Usage

Get or create your OpenAI API Key from here: https://platform.openai.com/account/api-keys

```shell
export OPENAI_API_KEY=xxx
chatgpt
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

## Customization

You can customize the model and parameters by creating a configuration file in `~/.config/chatgpt.json`.

Here is the default configuration:

```json
{
  "api_key": "sk-xxxxxx",
  "endpoint": "https://api.openai.com/v1",
  "prompt": "You are ChatGPT, a large language model trained by OpenAI. Answer as concisely as possible.",
  "model": "gpt-3.5-turbo",
  "context_length": 6,
  "stream": true,
  "temperature": 0,
  "max_tokens": 1024
}
```

If you cannot access to the default `https://api.openai.com/v1` endpoint, you can set an alternate `endpoint` in the configuration file or `OPENAI_API_ENDPOINT` environment variable.
Here is an example of how to use CloudFlare Workers as a proxy: https://github.com/noobnooc/noobnooc/discussions/9

## License

MIT

## Original Author

Yasuhiro Matsumoto (a.k.a. mattn)
