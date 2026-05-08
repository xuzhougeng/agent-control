package llm

import (
	"fmt"
	"os"
	"time"
)

// Config is the subset of provider settings the registry needs to construct a
// concrete Provider. Higher-level config packages can embed or convert into it.
type Config struct {
	Provider   string
	APIKey     string
	BaseURL    string
	APIKeyEnv  string
	Timeout    time.Duration
	DefaultMod string
}

// New constructs a Provider from a Config. Environment fallbacks are applied
// (ANTHROPIC_API_KEY for anthropic, OPENAI_API_KEY for openai) when APIKey is
// empty.
func New(cfg Config) (Provider, error) {
	switch cfg.Provider {
	case "", "anthropic":
		key := cfg.APIKey
		if key == "" {
			key = os.Getenv(envOrDefault(cfg.APIKeyEnv, "ANTHROPIC_API_KEY"))
		}
		return NewAnthropic(AnthropicOptions{
			APIKey: key, BaseURL: cfg.BaseURL, Timeout: cfg.Timeout,
		})
	case "openai", "deepseek", "qwen", "openai-compat":
		key := cfg.APIKey
		if key == "" {
			key = os.Getenv(envOrDefault(cfg.APIKeyEnv, "OPENAI_API_KEY"))
		}
		base := cfg.BaseURL
		if base == "" {
			base = "https://api.openai.com/v1"
		}
		return NewOpenAI(OpenAIOptions{
			APIKey: key, BaseURL: base, Timeout: cfg.Timeout,
		})
	default:
		return nil, fmt.Errorf("unknown provider: %q", cfg.Provider)
	}
}

func envOrDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
