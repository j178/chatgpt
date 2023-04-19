# Cli for ChatGPT

A TUI for ChatGPT API, powered by GPT-3.5-turbo and GPT-4 models.

![chatgpt-1 0](https://user-images.githubusercontent.com/10510431/229564407-e4c0b6bf-adfb-40f0-a63c-840dafbc1291.gif)

## Usage

Get or create your OpenAI API Key from here: https://platform.openai.com/account/api-keys

```shell
export OPENAI_API_KEY=xxx

# Chat mode
chatgpt

# Create a new chat and provide a prompt
chatgpt -n -p translator

# One-time chat mode, easily integrate with other tools
cat config.yaml | chatgpt -p 'convert this yaml to json'
echo "Hello, world" | chatgpt -p translator | say
```

### General Key Bindings

| Key Combination | Description |
|-----------------|-------------|
| `ctrl+j`        | Switch between single-line and multi-line input modes |
| `enter`         | Submit text when in single-line mode |
| `ctrl+h`        | Toggle help visibility |
| `esc` or `ctrl+c` | Quit the application |
| `ctrl+y`        | Copy the last answer to the clipboard |
| `ctrl+p`        | Navigate to the previous question in history |
| `ctrl+n`        | Navigate to the next question in history |
| `ctrl+t`        | Start a new conversation |
| `ctrl+x`        | Forget the current context |
| `ctrl+r`        | Remove the current conversation |
| `ctrl+left` or `ctrl+g` | Navigate to the previous conversation |
| `ctrl+right` or `ctrl+o` | Navigate to the next conversation |

### Viewport Key Bindings

| Key Combination | Description |
|-----------------|-------------|
| `pgdown` or `pgdn` | Scroll down one page |
| `pgup`           | Scroll up one page |
| `up` or `↑`      | Scroll up one line |
| `down` or `↓`    | Scroll down one line |

### Text Area Key Bindings

| Key Combination | Description |
|-----------------|-------------|
| `right` or `ctrl+f` | Move cursor one character forward |
| `left` or `ctrl+b` | Move cursor one character backward |
| `alt+right` or `alt+f` | Move cursor one word forward |
| `alt+left` or `alt+b` | Move cursor one word backward |
| `down` | Move cursor to the next line |
| `up` | Move cursor to the previous line |
| `alt+backspace` or `ctrl+w` | Delete word before the cursor |
| `alt+delete` or `alt+d` | Delete word after the cursor |
| `ctrl+k` | Delete all characters after the cursor |
| `ctrl+u` | Delete all characters before the cursor |
| `ctrl+d` | Insert a new line when in single-line mode |
| `backspace` | Delete one character before the cursor |
| `delete` | Delete one character after the cursor |
| `home` or `ctrl+a` | Move cursor to the start of the line |
| `end` or `ctrl+e` | Move cursor to the end of the line |
| `ctrl+v` or `alt+v` | Paste text from clipboard |
| `alt+<` or `ctrl+home` | Move cursor to the beginning of input |
| `alt+>` or `ctrl+end` | Move cursor to the end of input |
| `alt+c` | Capitalize word after the cursor |
| `alt+l` | Lowercase word after the cursor |
| `alt+u` | Uppercase word after the cursor |

### Multi-line Input Mode Specific Key Bindings

| Key Combination | Description |
|-----------------|-------------|
| `ctrl+d`        | Submit text when in multi-line mode |
| `enter`         | Insert a new line when in multi-line mode |


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

### Install via [Nix](https://search.nixos.org/packages) on macOS/Linux

```
environment.systemPackages = [
  pkgs.chatgpt-cli
];
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

> **Note**
> The prompt can be a predefined prompt, or come up with one on the fly.
> e.g. `chatgpt -p translator` or `chatgpt -p "You are a cat. You can only meow. That's it."`

### Azure OpenAI service support

If you are using Azure OpenAI service, you should configure like this:

```json
{
  "api_type": "AZURE",
  "api_key": "xxxx",
  "endpoint": "https://YOUR_RESOURCE_NAME.openai.azure.com",
  "engine": "YOUR_DEPLOYMENT_NAME",
  "api_version": "2023-03-15-preview"
}
```

## Troubleshooting

1. `Error: unexpected EOF, please try again`

    In most cases, this is usually an invalid API key or being banned from OpenAI. To check for any error messages, please execute `echo hello | chatgpt`.

    If you cannot access to the default `https://api.openai.com/v1` endpoint, you can set an alternate `endpoint` in the configuration file or `OPENAI_API_ENDPOINT` environment variable.
    Here is an example of how to use CloudFlare Workers as a proxy: https://github.com/noobnooc/noobnooc/discussions/9

## License

MIT

## Original Author

Yasuhiro Matsumoto (a.k.a. mattn)
