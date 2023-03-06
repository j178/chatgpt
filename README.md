# Cli for ChatGPT

A simple cli wrapper for ChatGPT API, powered by GPT-3.5-turbo model.

![chat-gpt](https://user-images.githubusercontent.com/10510431/222810716-31e51038-b2f1-4ebf-bc11-c827da3ed0c9.gif)


## Usage

Get or create your OpenAI API Key from here: https://platform.openai.com/account/api-keys

```shell
$ export OPENAI_API_KEY=xxx
$ chatgpt
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

## Custom OpenAI API Endpoint

If you cannot access to the default `api.openai.com` endpoint, you can use the custom endpoint.

```shell
chatgpt -e https://xxx.workers.dev/v1
```

Here is an example of using CloudFlare Workers as a proxy: https://github.com/noobnooc/noobnooc/discussions/9

## License

MIT

## Original Author

Yasuhiro Matsumoto (a.k.a. mattn)
