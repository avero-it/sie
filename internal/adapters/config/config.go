// Package config loads sie's settings with viper from YAML files and
// environment variables. It is read once by the composition root; the core
// receives plain values and never sees viper.
package config

import (
	"bytes"
	"errors"
	"fmt"
	iofs "io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// Settings are the user-configurable options.
type Settings struct {
	TabWidth       int    `mapstructure:"tab_width"`
	ShowInvisibles bool   `mapstructure:"show_invisibles"`  // mark spaces and tabs
	ShowEmptyCells bool   `mapstructure:"show_empty_cells"` // mark cells holding no character
	LineEnding     string `mapstructure:"line_ending"`      // for new files: "lf" or "crlf"
	JumpLines      int    `mapstructure:"jump_lines"`       // lines moved by Alt+Up/Down
}

// Defaults returns the built-in settings.
func Defaults() Settings {
	return Settings{TabWidth: 4, ShowInvisibles: false, ShowEmptyCells: true, LineEnding: "lf", JumpLines: 5}
}

// SearchPaths returns the config files, lowest precedence first: the system
// file /etc/sie/config.yaml, then the user file ~/.sie/config.yaml.
func SearchPaths() []string {
	paths := []string{filepath.Join(string(filepath.Separator), "etc", "sie", "config.yaml")}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".sie", "config.yaml"))
	}
	return paths
}

// Load reads the settings. With an explicit path only that file is read and
// it must exist. Otherwise /etc/sie/config.yaml and then ~/.sie/config.yaml
// are merged (the user file wins key by key); missing files are skipped.
// Environment variables SIE_<KEY> (e.g. SIE_TAB_WIDTH) override files.
// It returns the files actually read.
func Load(explicit string) (Settings, []string, error) {
	if explicit != "" {
		return load([]string{explicit}, true)
	}
	return load(SearchPaths(), false)
}

func load(paths []string, mustExist bool) (Settings, []string, error) {
	v := viper.New()
	v.SetConfigType("yaml")
	d := Defaults()
	v.SetDefault("tab_width", d.TabWidth)
	v.SetDefault("show_invisibles", d.ShowInvisibles)
	v.SetDefault("show_empty_cells", d.ShowEmptyCells)
	v.SetDefault("line_ending", d.LineEnding)
	v.SetDefault("jump_lines", d.JumpLines)
	v.SetEnvPrefix("SIE")
	v.AutomaticEnv()

	var used []string
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if errors.Is(err, iofs.ErrNotExist) && !mustExist {
			continue
		}
		if err != nil {
			return Settings{}, used, fmt.Errorf("config: %w", err)
		}
		if err := v.MergeConfig(bytes.NewReader(data)); err != nil {
			return Settings{}, used, fmt.Errorf("config %s: %w", path, err)
		}
		used = append(used, path)
	}
	var s Settings
	if err := v.UnmarshalExact(&s); err != nil {
		return Settings{}, used, fmt.Errorf("config: %w", err)
	}
	if err := s.validate(); err != nil {
		return Settings{}, used, fmt.Errorf("config: %w", err)
	}
	return s, used, nil
}

func (s *Settings) validate() error {
	if s.TabWidth < 1 || s.TabWidth > 16 {
		return fmt.Errorf("tab_width must be between 1 and 16, got %d", s.TabWidth)
	}
	if s.JumpLines < 1 || s.JumpLines > 100 {
		return fmt.Errorf("jump_lines must be between 1 and 100, got %d", s.JumpLines)
	}
	s.LineEnding = strings.ToLower(s.LineEnding)
	if s.LineEnding != "lf" && s.LineEnding != "crlf" {
		return fmt.Errorf(`line_ending must be "lf" or "crlf", got %q`, s.LineEnding)
	}
	return nil
}
