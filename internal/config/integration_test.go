// Test Rig-Level Custom Agent Support
//
// This test file verifies that custom agents defined in rig-level
// settings/config.json are correctly loaded and used when spawning
// polecats and crew members.

package config

import (
	"testing"
)

// TestRigLevelCustomAgentIntegration tests end-to-end rig-level custom agent functionality.
func TestRigLevelCustomAgentIntegration(t *testing.T) {
	t.Skip("Integration test: requires full Gas Town environment setup")

	// TODO: Set up temporary town and rig with custom agents
	// Then verify:
	// 1. gt config agent list shows custom agent
	// 2. gt sling <bead> <rig> spawns polecat with custom agent
	// 3. Polecat uses the custom agent (not default)

	t.Log("This integration test is skipped - use e2e tests in CI/CD")
}
