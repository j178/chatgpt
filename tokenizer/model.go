package tokenizer

import (
	"strings"
	"sync"

	"github.com/tiktoken-go/tokenizer"
)

// Reference: https://github.com/openai/tiktoken/blob/main/tiktoken/model.py

var modelPrefixToEncoding = map[string]string{
	// chat
	"gpt-4-":         "cl100k_base", // e.g., gpt-4-0314, etc., plus gpt-4-32k
	"gpt-3.5-turbo-": "cl100k_base", // e.g, gpt-3.5-turbo-0301, -0401, etc.
	"gpt-35-turbo-":  "cl100k_base", // Azure deployment name
	// fine-tuned
	"ft:gpt-4":         "cl100k_base",
	"ft:gpt-3.5-turbo": "cl100k_base",
	"ft:davinci-002":   "cl100k_base",
	"ft:babbage-002":   "cl100k_base",
}

var modelToEncoding = map[string]string{
	// chat
	"gpt-4":         "cl100k_base",
	"gpt-3.5-turbo": "cl100k_base",
	"gpt-35-turbo":  "cl100k_base", // Azure deployment name
	// base
	"davinci-002": "cl100k_base",
	"babbage-002": "cl100k_base",
	// embeddings
	"text-embedding-ada-002": "cl100k_base",
	// DEPRECATED MODELS
	// text (DEPRECATED)
	"text-davinci-003": "p50k_base",
	"text-davinci-002": "p50k_base",
	"text-davinci-001": "r50k_base",
	"text-curie-001":   "r50k_base",
	"text-babbage-001": "r50k_base",
	"text-ada-001":     "r50k_base",
	"davinci":          "r50k_base",
	"curie":            "r50k_base",
	"babbage":          "r50k_base",
	"ada":              "r50k_base",
	// code (DEPRECATED)
	"code-davinci-002": "p50k_base",
	"code-davinci-001": "p50k_base",
	"code-cushman-002": "p50k_base",
	"code-cushman-001": "p50k_base",
	"davinci-codex":    "p50k_base",
	"cushman-codex":    "p50k_base",
	// edit (DEPRECATED)
	"text-davinci-edit-001": "p50k_edit",
	"code-davinci-edit-001": "p50k_edit",
	// old embeddings (DEPRECATED)
	"text-similarity-davinci-001":  "r50k_base",
	"text-similarity-curie-001":    "r50k_base",
	"text-similarity-babbage-001":  "r50k_base",
	"text-similarity-ada-001":      "r50k_base",
	"text-search-davinci-doc-001":  "r50k_base",
	"text-search-curie-doc-001":    "r50k_base",
	"text-search-babbage-doc-001":  "r50k_base",
	"text-search-ada-doc-001":      "r50k_base",
	"code-search-babbage-code-001": "r50k_base",
	"code-search-ada-code-001":     "r50k_base",
	// open source
	"gpt2": "gpt2",
}

func EncodingNameForModel(model string) (string, error) {
	if encodingName, ok := modelToEncoding[model]; ok {
		return encodingName, nil
	}
	for prefix, encodingName := range modelPrefixToEncoding {
		if strings.HasPrefix(model, prefix) {
			return encodingName, nil
		}
	}
	return "", tokenizer.ErrEncodingNotSupported
}

func ForModel(model string) (tokenizer.Codec, error) {
	encodingName, err := EncodingNameForModel(model)
	if err != nil {
		return nil, err
	}
	return Get(encodingName)
}

func Get(encodingName string) (tokenizer.Codec, error) {
	l.Lock()
	defer l.Unlock()
	if enc, ok := encoders[encodingName]; ok {
		return enc, nil
	}
	enc, err := tokenizer.Get(tokenizer.Encoding(encodingName))
	if err != nil {
		return nil, err
	}
	encoders[encodingName] = enc
	return enc, nil
}

var (
	l        sync.Mutex
	encoders = map[string]tokenizer.Codec{}
)
