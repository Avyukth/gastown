package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuiltinPresets(t *testing.T) {
	presets := []AgentPreset{AgentClaude, AgentGemini, AgentCodex, AgentOpenCode}

	for _, preset := range presets {
		info := GetAgentPreset(preset)
		if info == nil {
			t.Errorf("GetAgentPreset(%s) returned nil", preset)
			continue
		}

		if info.Command == "" {
			t.Errorf("preset %s has empty Command", preset)
		}
	}
}

func TestGetAgentPresetByName(t *testing.T) {
	tests := []struct {
		name    string
		want    AgentPreset
		wantNil bool
	}{
		{"claude", AgentClaude, false},
		{"gemini", AgentGemini, false},
		{"codex", AgentCodex, false},
		{"opencode", AgentOpenCode, false},
		{"aider", "", true}, // Not built-in, can be added via config
		{"unknown", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetAgentPresetByName(tt.name)
			if tt.wantNil && got != nil {
				t.Errorf("GetAgentPresetByName(%s) = %v, want nil", tt.name, got)
			}
			if !tt.wantNil && got == nil {
				t.Errorf("GetAgentPresetByName(%s) = nil, want preset", tt.name)
			}
			if !tt.wantNil && got != nil && got.Name != tt.want {
				t.Errorf("GetAgentPresetByName(%s).Name = %v, want %v", tt.name, got.Name, tt.want)
			}
		})
	}
}

func TestRuntimeConfigFromPreset(t *testing.T) {
	tests := []struct {
		preset      AgentPreset
		wantCommand string
	}{
		{AgentClaude, "claude"},
		{AgentGemini, "gemini"},
		{AgentCodex, "codex"},
		{AgentOpenCode, "opencode"},
	}

	for _, tt := range tests {
		t.Run(string(tt.preset), func(t *testing.T) {
			rc := RuntimeConfigFromPreset(tt.preset)
			if rc.Command != tt.wantCommand {
				t.Errorf("RuntimeConfigFromPreset(%s).Command = %v, want %v",
					tt.preset, rc.Command, tt.wantCommand)
			}
		})
	}
}

func TestIsKnownPreset(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"claude", true},
		{"gemini", true},
		{"codex", true},
		{"opencode", true},
		{"aider", false}, // Not built-in, can be added via config
		{"unknown", false},
		{"chatgpt", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsKnownPreset(tt.name); got != tt.want {
				t.Errorf("IsKnownPreset(%s) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestLoadAgentRegistry(t *testing.T) {
	// Create temp directory for test config
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "agents.json")

	// Write custom agent config
	customRegistry := AgentRegistry{
		Version: CurrentAgentRegistryVersion,
		Agents: map[string]*AgentPresetInfo{
			"my-agent": {
				Name:    "my-agent",
				Command: "my-agent-bin",
				Args:    []string{"--auto"},
			},
		},
	}

	data, err := json.Marshal(customRegistry)
	if err != nil {
		t.Fatalf("failed to marshal test config: %v", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	// Reset global registry for test isolation
	ResetRegistryForTesting()

	// Load the custom registry
	if err := LoadAgentRegistry(configPath); err != nil {
		t.Fatalf("LoadAgentRegistry failed: %v", err)
	}

	// Check custom agent is available
	myAgent := GetAgentPresetByName("my-agent")
	if myAgent == nil {
		t.Fatal("custom agent 'my-agent' not found after loading registry")
	}
	if myAgent.Command != "my-agent-bin" {
		t.Errorf("my-agent.Command = %v, want my-agent-bin", myAgent.Command)
	}

	// Check built-ins still accessible
	claude := GetAgentPresetByName("claude")
	if claude == nil {
		t.Fatal("built-in 'claude' not found after loading registry")
	}

	// Reset for other tests
	ResetRegistryForTesting()
}

func TestAgentPresetYOLOFlags(t *testing.T) {
	// Verify YOLO flags are set correctly for each E2E tested agent
	tests := []struct {
		preset  AgentPreset
		wantArg string // At least this arg should be present
	}{
		{AgentClaude, "--dangerously-skip-permissions"},
		{AgentGemini, "yolo"}, // Part of "--approval-mode yolo"
		{AgentCodex, "--yolo"},
	}

	for _, tt := range tests {
		t.Run(string(tt.preset), func(t *testing.T) {
			info := GetAgentPreset(tt.preset)
			if info == nil {
				t.Fatalf("preset %s not found", tt.preset)
			}

			found := false
			for _, arg := range info.Args {
				if arg == tt.wantArg || (tt.preset == AgentGemini && arg == "yolo") {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("preset %s args %v missing expected %s", tt.preset, info.Args, tt.wantArg)
			}
		})
	}
}

func TestMergeWithPreset(t *testing.T) {
	// Test that user config overrides preset defaults
	userConfig := &RuntimeConfig{
		Command: "/custom/claude",
		Args:    []string{"--custom-arg"},
	}

	merged := userConfig.MergeWithPreset(AgentClaude)

	if merged.Command != "/custom/claude" {
		t.Errorf("merged command should be user value, got %s", merged.Command)
	}
	if len(merged.Args) != 1 || merged.Args[0] != "--custom-arg" {
		t.Errorf("merged args should be user value, got %v", merged.Args)
	}

	// Test nil config gets preset defaults
	var nilConfig *RuntimeConfig
	merged = nilConfig.MergeWithPreset(AgentClaude)

	if merged.Command != "claude" {
		t.Errorf("nil config merge should get preset command, got %s", merged.Command)
	}

	// Test empty config gets preset defaults
	emptyConfig := &RuntimeConfig{}
	merged = emptyConfig.MergeWithPreset(AgentGemini)

	if merged.Command != "gemini" {
		t.Errorf("empty config merge should get preset command, got %s", merged.Command)
	}
}

func TestBuildResumeCommand(t *testing.T) {
	tests := []struct {
		name      string
		agentName string
		sessionID string
		wantEmpty bool
		contains  []string // strings that should appear in result
	}{
		{
			name:      "claude with session",
			agentName: "claude",
			sessionID: "session-123",
			wantEmpty: false,
			contains:  []string{"claude", "--dangerously-skip-permissions", "--resume", "session-123"},
		},
		{
			name:      "gemini with session",
			agentName: "gemini",
			sessionID: "gemini-sess-456",
			wantEmpty: false,
			contains:  []string{"gemini", "--approval-mode", "yolo", "--resume", "gemini-sess-456"},
		},
		{
			name:      "codex subcommand style",
			agentName: "codex",
			sessionID: "codex-sess-789",
			wantEmpty: false,
			contains:  []string{"codex", "resume", "codex-sess-789", "--yolo"},
		},
		{
			name:      "opencode with session",
			agentName: "opencode",
			sessionID: "oc-sess-123",
			wantEmpty: false,
			contains:  []string{"opencode", "--session", "oc-sess-123"},
		},
		{
			name:      "empty session ID",
			agentName: "claude",
			sessionID: "",
			wantEmpty: true,
		},
		{
			name:      "unknown agent",
			agentName: "unknown-agent",
			sessionID: "session-123",
			wantEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildResumeCommand(tt.agentName, tt.sessionID)
			if tt.wantEmpty {
				if result != "" {
					t.Errorf("BuildResumeCommand(%s, %s) = %q, want empty", tt.agentName, tt.sessionID, result)
				}
				return
			}
			for _, s := range tt.contains {
				if !strings.Contains(result, s) {
					t.Errorf("BuildResumeCommand(%s, %s) = %q, missing %q", tt.agentName, tt.sessionID, result, s)
				}
			}
		})
	}
}

func TestSupportsSessionResume(t *testing.T) {
	tests := []struct {
		agentName string
		want      bool
	}{
		{"claude", true},
		{"gemini", true},
		{"codex", true},
		{"opencode", true},
		{"unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.agentName, func(t *testing.T) {
			if got := SupportsSessionResume(tt.agentName); got != tt.want {
				t.Errorf("SupportsSessionResume(%s) = %v, want %v", tt.agentName, got, tt.want)
			}
		})
	}
}

func TestGetSessionIDEnvVar(t *testing.T) {
	tests := []struct {
		agentName string
		want      string
	}{
		{"claude", "CLAUDE_SESSION_ID"},
		{"gemini", "GEMINI_SESSION_ID"},
		{"codex", ""},    // Codex uses JSONL output instead
		{"opencode", ""}, // OpenCode manages sessions internally
		{"unknown", ""},
	}

	for _, tt := range tests {
		t.Run(tt.agentName, func(t *testing.T) {
			if got := GetSessionIDEnvVar(tt.agentName); got != tt.want {
				t.Errorf("GetSessionIDEnvVar(%s) = %q, want %q", tt.agentName, got, tt.want)
			}
		})
	}
}

func TestBuiltInAgentNames(t *testing.T) {
	names := BuiltInAgentNames()

	if len(names) != 4 {
		t.Errorf("expected 4 built-in agents, got %d", len(names))
	}

	expected := map[string]bool{"claude": true, "gemini": true, "codex": true, "opencode": true}
	for _, name := range names {
		if !expected[name] {
			t.Errorf("unexpected agent in list: %s", name)
		}
		delete(expected, name)
	}

	if len(expected) > 0 {
		t.Errorf("missing agents: %v", expected)
	}
}

func TestOpenCodeNonInteractiveConfig(t *testing.T) {
	info := GetAgentPreset(AgentOpenCode)
	if info == nil {
		t.Fatal("AgentOpenCode preset not found")
	}

	if info.NonInteractive == nil {
		t.Fatal("OpenCode should have NonInteractive config")
	}

	if info.NonInteractive.Subcommand != "run" {
		t.Errorf("expected subcommand 'run', got %q", info.NonInteractive.Subcommand)
	}

	if info.NonInteractive.OutputFlag != "--format json" {
		t.Errorf("expected output flag '--format json', got %q", info.NonInteractive.OutputFlag)
	}

	if info.ResumeFlag != "--session" {
		t.Errorf("expected resume flag '--session', got %q", info.ResumeFlag)
	}

	if info.ResumeStyle != "flag" {
		t.Errorf("expected resume style 'flag', got %q", info.ResumeStyle)
	}

	if info.AutonomousModeEnv == nil {
		t.Fatal("OpenCode should have AutonomousModeEnv")
	}

	permission, ok := info.AutonomousModeEnv["OPENCODE_PERMISSION"]
	if !ok {
		t.Error("OpenCode should have OPENCODE_PERMISSION in AutonomousModeEnv")
	}
	if permission != `{"*":"allow"}` {
		t.Errorf("expected OPENCODE_PERMISSION to be '{\"*\":\"allow\"}', got %q", permission)
	}
}

func TestGetAutonomousModeEnv(t *testing.T) {
	tests := []struct {
		agentName string
		wantNil   bool
		wantKey   string
	}{
		{"claude", true, ""},
		{"gemini", true, ""},
		{"codex", true, ""},
		{"opencode", false, "OPENCODE_PERMISSION"},
		{"unknown", true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.agentName, func(t *testing.T) {
			got := GetAutonomousModeEnv(tt.agentName)
			if tt.wantNil && got != nil {
				t.Errorf("GetAutonomousModeEnv(%s) = %v, want nil", tt.agentName, got)
			}
			if !tt.wantNil {
				if got == nil {
					t.Errorf("GetAutonomousModeEnv(%s) = nil, want map", tt.agentName)
				} else if _, ok := got[tt.wantKey]; !ok {
					t.Errorf("GetAutonomousModeEnv(%s) missing key %q", tt.agentName, tt.wantKey)
				}
			}
		})
	}
}
