package config

import (
	"os"
	"path/filepath"
)

// defaultConfigJSON is written to the global config path on first run, when no
// configuration exists at any layer, so eonyx starts with a working setup the
// user can edit. The default model is the LOCAL Ollama-compatible one, so no
// data leaves the machine until the user explicitly switches to a cloud
// provider; the two DeepSeek models are preconfigured but opt-in.
//
// Every top-level key the schema supports is present so the file doubles as a
// discoverable reference. Fields whose example values would cause startup side
// effects (mcp servers dialed, hooks executed, a database connected) are left
// empty; see config.go for their structure. JSON allows no comments
// and config.go rejects unknown fields, so this template carries no annotations.
const defaultConfigJSON = `{
  "providers": {
    "deepseek": {
      "type": "openai",
      "base_url": "https://api.deepseek.com",
      "api_key": "$DEEPSEEK_API_KEY"
    },
    "local": {
      "type": "openai",
      "base_url": "http://localhost:11434/v1",
      "api_key": ""
    }
  },
  "models": [
    {"id": "deepseek-v4-flash", "provider": "deepseek", "context_window": 1000000, "input_price": 0.14, "output_price": 0.28},
    {"id": "deepseek-v4-pro", "provider": "deepseek", "context_window": 1000000, "input_price": 0.435, "output_price": 0.87},
    {"id": "qwen2.5-coder", "provider": "local", "context_window": 32768, "input_price": 0, "output_price": 0}
  ],
  "default_model": "qwen2.5-coder",
  "max_steps": 50,
  "permissions": {
    "allowed_tools": []
  },
  "skills": [],
  "mcp": [],
  "hooks": {},
  "report": {
    "database_url": ""
  },
  "system_prompt": "",
  "log_file": "",
  "retry": {}
}
`

// EnsureDefaultConfig writes a default global config file when eonyx finds no
// configuration at all — no global config, no global data, and no project config
// reachable from startDir. It returns the written path and created=true in that
// case; otherwise it makes no changes and returns created=false.
func EnsureDefaultConfig(startDir string) (path string, created bool, err error) {
	if fileExists(GlobalConfigPath()) || fileExists(GlobalDataPath()) {
		return "", false, nil
	}
	if _, _, found := FindProjectConfig(startDir); found {
		return "", false, nil
	}

	path = GlobalConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", false, err
	}
	if err := os.WriteFile(path, []byte(defaultConfigJSON), 0o644); err != nil {
		return "", false, err
	}
	return path, true, nil
}

func fileExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}
