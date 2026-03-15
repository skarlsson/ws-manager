package cmd

import "testing"

func TestParseWorkspaceRef(t *testing.T) {
	tests := []struct {
		input    string
		wantHost string
		wantName string
	}{
		{"myws", "", "myws"},
		{"host:name", "host", "name"},
		{"clawdbot1:workshell", "clawdbot1", "workshell"},
		{"h:complex-name-here", "h", "complex-name-here"},
		{":leading-colon", "", ":leading-colon"}, // colon at pos 0 → no host
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			host, name := parseWorkspaceRef(tt.input)
			if host != tt.wantHost {
				t.Errorf("parseWorkspaceRef(%q) host = %q, want %q", tt.input, host, tt.wantHost)
			}
			if name != tt.wantName {
				t.Errorf("parseWorkspaceRef(%q) name = %q, want %q", tt.input, name, tt.wantName)
			}
		})
	}
}

func TestStateKey(t *testing.T) {
	tests := []struct {
		host string
		name string
		want string
	}{
		{"", "myws", "myws"},
		{"host", "name", "host@name"},
		{"clawdbot1", "workshell", "clawdbot1@workshell"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := stateKey(tt.host, tt.name)
			if got != tt.want {
				t.Errorf("stateKey(%q, %q) = %q, want %q", tt.host, tt.name, got, tt.want)
			}
		})
	}
}
