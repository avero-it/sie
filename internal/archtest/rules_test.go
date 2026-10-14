package archtest

import "testing"

func TestViolation(t *testing.T) {
	tests := []struct {
		pkg, imp string
		bad      bool
	}{
		{domain + "/buffer", "bytes", false},
		{domain + "/buffer", uniseg, false},
		{domain + "/editor", domain + "/buffer", false},
		{domain + "/buffer", "os", true},
		{domain + "/buffer", "os/exec", true},
		{domain + "/buffer", app, true},
		{domain + "/buffer", ports, true},
		{domain + "/buffer", tcell + "/v2", true},
		{domain + "/buffer", "golang.org/x/text", true},
		{app + "/commands", ports, false},
		{app + "/commands", domain + "/buffer", false},
		{app + "/commands", adapters + "fs", true},
		{ports, app, true},
		{adapters + "tui", tcell + "/v2", false},
		{adapters + "tui", ports, false},
		{adapters + "tui/paint", adapters + "tui", false},
		{adapters + "tui", adapters + "fs", true},
		{adapters + "fs", tcell + "/v2", true},
		{module + "/cmd/sie", adapters + "tui", false},
		{module + "/cmd/sie", "C", true},
		{adapters + "config", "github.com/spf13/viper", false},
		{module + "/cmd/sie", "github.com/spf13/viper", true},
		{app, "github.com/spf13/viper", true},
		{module + "/cmd/sie", "github.com/spf13/cobra", false},
		{adapters + "tui", "github.com/spf13/cobra", true},
		{adapters + "syntax", "github.com/alecthomas/chroma/v2/lexers", false},
		{app, "github.com/alecthomas/chroma/v2", true},
		{module + "/cmd/sie", "github.com/alecthomas/chroma/v2", true},
		{adapters + "validate", "github.com/goccy/go-yaml", false},
		{app, "github.com/goccy/go-yaml", true},
		{adapters + "config", "github.com/pelletier/go-toml/v2", true},
	}
	for _, tt := range tests {
		if got := violation(tt.pkg, tt.imp) != ""; got != tt.bad {
			t.Errorf("violation(%q, %q) = %v, want %v", tt.pkg, tt.imp, got, tt.bad)
		}
	}
}
