// Command sie is a modeless terminal text editor.
//
// This is the composition root: it is the only package that knows the
// concrete adapters and wires them to the core's ports. The command line is
// built with cobra; settings are loaded by the config adapter (viper).
package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/avero-it/sie/internal/adapters/clipboard"
	"github.com/avero-it/sie/internal/adapters/config"
	fsadapter "github.com/avero-it/sie/internal/adapters/fs"
	"github.com/avero-it/sie/internal/adapters/keymap"
	"github.com/avero-it/sie/internal/adapters/recent"
	"github.com/avero-it/sie/internal/adapters/syntax"
	"github.com/avero-it/sie/internal/adapters/tui"
	"github.com/avero-it/sie/internal/adapters/validate"
	"github.com/avero-it/sie/internal/buildinfo"
	"github.com/avero-it/sie/internal/core/app"
	"github.com/avero-it/sie/internal/core/domain/buffer"
	"github.com/avero-it/sie/internal/core/ports"
	"github.com/spf13/cobra"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	root := newRootCmd()
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(stderr, "sie:", err)
		return 1
	}
	return 0
}

func newRootCmd() *cobra.Command {
	var configPath string
	root := &cobra.Command{
		Use:   "sie [file]",
		Short: "A modeless terminal text editor",
		Long: "sie is a modeless terminal text editor.\n\n" +
			"Settings are read from /etc/sie/config.yaml and ~/.sie/config.yaml\n" +
			"(the user file wins key by key) and SIE_* environment variables.",
		Args:          cobra.MaximumNArgs(1),
		Version:       buildinfo.Get().Short(),
		SilenceErrors: true, // run prints errors once, prefixed with "sie:"
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEditor(configPath, args)
		},
	}
	root.PersistentFlags().StringVar(&configPath, "config", "", "read settings only from this file")
	root.AddCommand(newVersionCmd(), newConfigCmd(&configPath))
	return root
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show version, commit, build time and Go version",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprint(cmd.OutOrStdout(), buildinfo.Get().String())
		},
	}
}

func newConfigCmd(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Show the effective settings and the files they come from",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, used, err := config.Load(*configPath)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(used) == 0 {
				fmt.Fprintf(out, "# no config file found; searched: %s\n", strings.Join(config.SearchPaths(), ", "))
			}
			for _, f := range used {
				fmt.Fprintf(out, "# from %s\n", f)
			}
			fmt.Fprintf(out, "tab_width: %d\nshow_invisibles: %t\nshow_empty_cells: %t\nclick_ripple: %t\nline_ending: %s\njump_lines: %d\n",
				s.TabWidth, s.ShowInvisibles, s.ShowEmptyCells, s.ClickRipple, s.LineEnding, s.JumpLines)
			fmt.Fprintf(out, "syntax_highlighting: %t\nsyntax_max_size: %s\nvalidate_on_save: %t\n",
				s.SyntaxHighlighting, s.SyntaxMaxSize, s.ValidateOnSave)
			return nil
		},
	}
}

// runEditor wires the adapters to the core and runs the terminal UI.
func runEditor(configPath string, args []string) error {
	settings, _, err := config.Load(configPath)
	if err != nil {
		return err
	}
	term, err := tui.New()
	if err != nil {
		return err
	}
	workDir, _ := os.Getwd()       // "" falls back to "." in the app
	homeDir, _ := os.UserHomeDir() // "" disables "~"
	var recentFiles *recent.Store
	if path, err := recent.DefaultPath(); err == nil {
		recentFiles = recent.New(path) // ~/.sie/recent.json
	}
	highlighter := syntax.New(term.Wake)
	defer highlighter.Close()
	eol := buffer.LF
	if settings.LineEnding == "crlf" {
		eol = buffer.CRLF
	}
	editor, err := app.New(app.Config{
		Files:          fsadapter.Store{},
		Clipboard:      clipboard.New(term.SetClipboard),
		Keymap:         keymap.Default(),
		TabWidth:       settings.TabWidth,
		DefaultEOL:     eol,
		Version:        buildinfo.Get().Display(),
		ShowInvisibles: settings.ShowInvisibles,
		ShowEmptyCells: settings.ShowEmptyCells,
		ClickRipple:    settings.ClickRipple,
		JumpLines:      settings.JumpLines,

		Recent:         recentFilesPort(recentFiles),
		Validator:      validate.Validator{},
		ValidateOnSave: settings.ValidateOnSave,
		WorkDir:        workDir,
		HomeDir:        homeDir,

		Highlighter:        highlighter,
		SyntaxHighlighting: settings.SyntaxHighlighting,
		SyntaxMaxSize:      settings.SyntaxMaxBytes,
	})
	if err != nil {
		return err
	}
	if len(args) == 1 {
		_ = editor.Dispatch("file.open", args[0]) // errors show in the prompt line
	}
	return term.Run(editor)
}

// recentFilesPort avoids a typed-nil interface when there is no home dir.
func recentFilesPort(s *recent.Store) ports.RecentFiles {
	if s == nil {
		return nil
	}
	return s
}
