package config

import "testing"

func intPtr(v int) *int { return &v }

func TestValidateRetry(t *testing.T) {
	base := func() *Config {
		return &Config{
			Providers:    map[string]Provider{"p": {Type: "openai", BaseURL: "http://x"}},
			Models:       []Model{{ID: "m", Provider: "p"}},
			DefaultModel: "m",
		}
	}

	valid := base()
	valid.Retry = RetryConfig{MaxRetries: intPtr(0), BaseDelayMS: 100, ResponseHeaderTimeoutSec: 30}
	if err := valid.validate(); err != nil {
		t.Fatalf("valid retry config rejected: %v", err)
	}

	cases := map[string]RetryConfig{
		"negative max_retries": {MaxRetries: intPtr(-1)},
		"negative base_delay":  {BaseDelayMS: -5},
		"negative timeout":     {ResponseHeaderTimeoutSec: -1},
	}
	for name, rc := range cases {
		t.Run(name, func(t *testing.T) {
			c := base()
			c.Retry = rc
			if err := c.validate(); err == nil {
				t.Errorf("expected validation error for %s", name)
			}
		})
	}
}
