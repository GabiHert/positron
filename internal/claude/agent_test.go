package claude

import (
	"slices"
	"testing"
)

func TestRunArgsStartNoMCPServersForAnAgentWithoutAny(t *testing.T) {
	args := runArgs(AgentRun{SystemPrompt: "p"})
	if !slices.Contains(args, "--strict-mcp-config") {
		t.Fatalf("an agent without MCP servers should not start the person's servers: %v", args)
	}
	args = runArgs(AgentRun{SystemPrompt: "p", MCP: []string{"linear"}})
	if slices.Contains(args, "--strict-mcp-config") {
		t.Fatalf("an agent with MCP servers needs the person's config: %v", args)
	}
	i := slices.Index(args, "--allowedTools")
	if i < 0 || !slices.Contains([]string{args[i+1]}, "Read Write Edit MultiEdit Glob Grep mcp__linear") {
		t.Fatalf("allowed tools: %v", args)
	}
}
