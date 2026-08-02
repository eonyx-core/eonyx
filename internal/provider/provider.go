// Package provider constructs a concrete llm.Provider from configuration. It is
// the single place that maps a configured model onto the right client
// implementation, keeping the rest of the code dependent only on llm.Provider.
package provider

import (
	"fmt"
	"net"
	"net/http"
	"time"

	"eonyx/internal/config"
	"eonyx/internal/llm"
	"eonyx/internal/llm/openai"
)

// defaultResponseHeaderTimeout bounds the wait for the first response header
// when the config does not override it.
const defaultResponseHeaderTimeout = 60 * time.Second

// newHTTPClient builds a client that bounds only the connection-setup phase
// (dial, TLS handshake, time-to-first-response-header). It deliberately sets no
// overall Client.Timeout, which would abort a legitimate long token stream, so
// a stalled handshake fails fast while streaming stays unbounded.
func newHTTPClient(responseHeaderTimeout time.Duration) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: responseHeaderTimeout,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
}

// For returns a provider for the given model id. An empty modelID selects the
// configured default model. The returned provider is bound to that model and
// its backend credentials.
func For(cfg *config.Config, modelID string, httpClient *http.Client) (llm.Provider, error) {
	if modelID == "" {
		modelID = cfg.DefaultModel
	}
	if modelID == "" {
		return nil, fmt.Errorf("no model specified and no default_model configured")
	}

	if httpClient == nil {
		httpClient = newHTTPClient(responseHeaderTimeout(cfg))
	}

	model, ok := cfg.Model(modelID)
	if !ok {
		return nil, fmt.Errorf("unknown model %q", modelID)
	}
	p, ok := cfg.Providers[model.Provider]
	if !ok {
		return nil, fmt.Errorf("model %q references unknown provider %q", model.ID, model.Provider)
	}

	switch p.Type {
	case "openai":
		return openai.New(p.BaseURL, p.APIKey, model.ID, httpClient, retryOptions(cfg)...), nil
	default:
		return nil, fmt.Errorf("provider %q has unsupported type %q", model.Provider, p.Type)
	}
}

// responseHeaderTimeout resolves the configured response-header timeout, falling
// back to the default when unset.
func responseHeaderTimeout(cfg *config.Config) time.Duration {
	if s := cfg.Retry.ResponseHeaderTimeoutSec; s > 0 {
		return time.Duration(s) * time.Second
	}
	return defaultResponseHeaderTimeout
}

// retryOptions translates the retry config into client options, leaving the
// client's built-in defaults in place for any field the config does not set.
func retryOptions(cfg *config.Config) []openai.Option {
	maxRetries := -1
	if cfg.Retry.MaxRetries != nil {
		maxRetries = *cfg.Retry.MaxRetries
	}
	base := time.Duration(cfg.Retry.BaseDelayMS) * time.Millisecond
	if maxRetries < 0 && base <= 0 {
		return nil
	}
	return []openai.Option{openai.WithRetry(maxRetries, base)}
}
