// Daemon config persistence (~/.console/config.json). Port of the config
// helpers in apps/cli/daemon-manager.ts.
package daemon

import (
	"encoding/json"
	"os"
)

// Config is the saved daemon configuration.
type Config struct {
	Port     string `json:"port"`
	Host     string `json:"host"`
	LogLevel string `json:"logLevel"`
}

// DefaultConfig returns config seeded from PORT/HOST/LOG_LEVEL env vars,
// falling back to 3000/0.0.0.0/info.
func DefaultConfig() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	host := os.Getenv("HOST")
	if host == "" {
		host = "0.0.0.0"
	}
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}
	return Config{Port: port, Host: host, LogLevel: logLevel}
}

// SaveConfig merges overrides onto the current default and persists to
// ConsoleDir/config.json.
func SaveConfig(overrides Config) error {
	if err := EnsureConsoleDir(); err != nil {
		return err
	}
	merged := DefaultConfig()
	if overrides.Port != "" {
		merged.Port = overrides.Port
	}
	if overrides.Host != "" {
		merged.Host = overrides.Host
	}
	if overrides.LogLevel != "" {
		merged.LogLevel = overrides.LogLevel
	}
	data, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configFilePath(ConsoleDir()), data, 0o644)
}

// LoadConfig reads dir/config.json, falling back to defaults on any error
// (missing file, corrupt JSON).
func LoadConfig(dir string) Config {
	data, err := os.ReadFile(configFilePath(dir))
	if err != nil {
		return DefaultConfig()
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return DefaultConfig()
	}
	return cfg
}

// ResolvePortHost resolves the effective port/host: explicit override > env
// (PORT/HOST) > saved config > default. Pure, no I/O, for easy testing.
func ResolvePortHost(overridePort, overrideHost string, saved Config) (port, host string) {
	port = overridePort
	if port == "" {
		port = os.Getenv("PORT")
	}
	if port == "" {
		port = saved.Port
	}
	if port == "" {
		port = "3000"
	}
	host = overrideHost
	if host == "" {
		host = os.Getenv("HOST")
	}
	if host == "" {
		host = saved.Host
	}
	if host == "" {
		host = "0.0.0.0"
	}
	return port, host
}
