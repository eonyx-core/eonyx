package config

import (
	"os"
	"path/filepath"
	"runtime"
)

const (
	appName = "eonyx"
	// configFileName is the canonical config file name used at every layer.
	configFileName = "eonyx.json"
	// projectDirName is the per-project data directory (history, logs, overrides).
	projectDirName = ".eonyx"
)

// GlobalConfigPath returns the path to the user-global config file. The
// EONYX_GLOBAL_CONFIG environment variable overrides the computed location.
func GlobalConfigPath() string {
	if p := os.Getenv("EONYX_GLOBAL_CONFIG"); p != "" {
		return p
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(localAppData(), appName, configFileName)
	}
	return filepath.Join(xdgConfigHome(), appName, configFileName)
}

// GlobalDataPath returns the path to the user-global data config file. The
// EONYX_GLOBAL_DATA environment variable overrides the computed location.
func GlobalDataPath() string {
	if p := os.Getenv("EONYX_GLOBAL_DATA"); p != "" {
		return p
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(localAppData(), appName, configFileName)
	}
	return filepath.Join(xdgDataHome(), appName, configFileName)
}

// GlobalSessionsDir returns the directory where session data is stored,
// alongside the global data config, so sessions are shared across all projects.
func GlobalSessionsDir() string {
	return filepath.Dir(GlobalDataPath())
}

// CacheDir returns the directory for cache data. The EONYX_CACHE_DIR
// environment variable overrides the computed location.
func CacheDir() string {
	if p := os.Getenv("EONYX_CACHE_DIR"); p != "" {
		return p
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(localAppData(), appName, "cache")
	}
	return filepath.Join(xdgCacheHome(), appName)
}

// FindProjectConfig walks up from startDir looking for a project config file
// (eonyx.json, then .eonyx.json at each level). It returns the config
// file path and the directory that contains it (the project root). found is
// false if no project config exists between startDir and the filesystem root.
func FindProjectConfig(startDir string) (path, root string, found bool) {
	dir := startDir
	for {
		for _, name := range []string{configFileName, "." + configFileName} {
			candidate := filepath.Join(dir, name)
			if fi, err := os.Stat(candidate); err == nil && !fi.IsDir() {
				return candidate, dir, true
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", "", false
		}
		dir = parent
	}
}

// ProjectDataDir returns the per-project data directory for the given project
// root (the directory holding the project config).
func ProjectDataDir(root string) string {
	return filepath.Join(root, projectDirName)
}

func localAppData() string {
	if d := os.Getenv("LOCALAPPDATA"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "AppData", "Local")
}

func xdgConfigHome() string {
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config")
}

func xdgDataHome() string {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share")
}

func xdgCacheHome() string {
	if d := os.Getenv("XDG_CACHE_HOME"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache")
}
