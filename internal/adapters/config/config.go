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
	ClickRipple    bool   `mapstructure:"click_ripple"`     // wave animation on clicks
	IndentGuide    bool   `mapstructure:"indent_guide"`     // flash a guide when the line indent moves
	Wrap           bool   `mapstructure:"wrap"`             // wrap long lines
	ColumnRuler    bool   `mapstructure:"column_ruler"`     // column numbers on the first text row
	LineEnding     string `mapstructure:"line_ending"`      // for new files: "lf" or "crlf"
	JumpLines      int    `mapstructure:"jump_lines"`       // lines moved by Alt+Up/Down

	ValidateOnSave     bool   `mapstructure:"validate_on_save"`
	SyntaxHighlighting bool   `mapstructure:"syntax_highlighting"`
	SyntaxMaxSize      string `mapstructure:"syntax_max_size"` // e.g. "2MB", "512KB"
	SyntaxMaxBytes     int    `mapstructure:"-"`               // SyntaxMaxSize parsed

	LargeFileSize  string `mapstructure:"large_file_size"` // e.g. "32MB": larger files stay on disk
	LargeFileBytes int    `mapstructure:"-"`               // LargeFileSize parsed
}

// Defaults returns the built-in settings.
func Defaults() Settings {
	return Settings{TabWidth: 4, ShowInvisibles: false, ShowEmptyCells: true, ColumnRuler: true, ClickRipple: false, LineEnding: "lf", JumpLines: 5,
		SyntaxHighlighting: true, SyntaxMaxSize: "2MB", SyntaxMaxBytes: 2 << 20, ValidateOnSave: true,
		LargeFileSize: "32MB", LargeFileBytes: 32 << 20}
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
	v.SetDefault("click_ripple", d.ClickRipple)
	v.SetDefault("indent_guide", d.IndentGuide)
	v.SetDefault("wrap", d.Wrap)
	v.SetDefault("column_ruler", d.ColumnRuler)
	v.SetDefault("line_ending", d.LineEnding)
	v.SetDefault("jump_lines", d.JumpLines)
	v.SetDefault("syntax_highlighting", d.SyntaxHighlighting)
	v.SetDefault("validate_on_save", d.ValidateOnSave)
	v.SetDefault("syntax_max_size", d.SyntaxMaxSize)
	v.SetDefault("large_file_size", d.LargeFileSize)
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
	s.SyntaxMaxBytes = int(v.GetSizeInBytes("syntax_max_size"))
	s.LargeFileBytes = int(v.GetSizeInBytes("large_file_size"))
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
	if s.SyntaxMaxBytes <= 0 {
		return fmt.Errorf(`syntax_max_size must be a size like "2MB" or "512KB", got %q`, s.SyntaxMaxSize)
	}
	if s.LargeFileBytes <= 0 {
		return fmt.Errorf(`large_file_size must be a size like "32MB" or "512KB", got %q`, s.LargeFileSize)
	}
	s.LineEnding = strings.ToLower(s.LineEnding)
	if s.LineEnding != "lf" && s.LineEnding != "crlf" {
		return fmt.Errorf(`line_ending must be "lf" or "crlf", got %q`, s.LineEnding)
	}
	return nil
}
