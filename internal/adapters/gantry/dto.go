package gantry

import (
	"encoding/json"

	"capcom/internal/domain"
)

type gantryAgent struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Display     string         `json:"displayName"`
	Description string         `json:"description"`
	Status      string         `json:"status"`
	AppID       string         `json:"appId"`
	Harness     string         `json:"agentHarness"`
	ConfigID    string         `json:"currentConfigVersionId"`
	ParentID    string         `json:"parentAgentId"`
	CreatedAt   string         `json:"createdAt"`
	UpdatedAt   string         `json:"updatedAt"`
	Raw         map[string]any `json:"-"`
}

type gantryAgentDelegates struct {
	AgentID   string                   `json:"agentId"`
	Revision  int                      `json:"revision"`
	Delegates []string                 `json:"delegates"`
	Resolved  []gantryResolvedDelegate `json:"resolved"`
}

type gantryResolvedDelegate struct {
	Ref         string         `json:"ref"`
	AgentID     string         `json:"agentId"`
	ToolName    string         `json:"toolName"`
	DisplayName string         `json:"displayName"`
	Persona     string         `json:"persona"`
	Raw         map[string]any `json:"-"`
}

func (g *gantryResolvedDelegate) UnmarshalJSON(data []byte) error {
	type alias gantryResolvedDelegate
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*g = gantryResolvedDelegate(decoded)
	g.Raw = raw
	return nil
}

func (g *gantryAgent) UnmarshalJSON(data []byte) error {
	type alias gantryAgent
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*g = gantryAgent(decoded)
	g.Raw = raw
	return nil
}

func (g gantryAgent) DisplayName() string {
	if g.Display != "" {
		return g.Display
	}
	if g.Name != "" {
		return g.Name
	}
	return g.ID
}

func (g gantryAgent) StatusDomain() domain.AgentStatus {
	switch g.Status {
	case "active", "enabled", "running":
		return domain.AgentStatusEnabled
	case "disabled":
		return domain.AgentStatusDisabled
	default:
		return domain.AgentStatusUnknown
	}
}

type gantryAccess struct {
	AgentID    string            `json:"agentId"`
	Selections []gantrySelection `json:"selections"`
	Raw        map[string]any    `json:"-"`
}

func (g *gantryAccess) UnmarshalJSON(data []byte) error {
	type alias gantryAccess
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*g = gantryAccess(decoded)
	g.Raw = raw
	return nil
}

type gantrySelection struct {
	ID      string `json:"id"`
	Version string `json:"version,omitempty"`
}

type gantryDoctor struct {
	Status string              `json:"status"`
	Checks []gantryDoctorCheck `json:"checks"`
}

type gantryDoctorCheck struct {
	ID      string         `json:"id"`
	Status  string         `json:"status"`
	Message string         `json:"message"`
	Raw     map[string]any `json:"-"`
}

func (g *gantryDoctorCheck) UnmarshalJSON(data []byte) error {
	type alias gantryDoctorCheck
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*g = gantryDoctorCheck(decoded)
	g.Raw = raw
	return nil
}

type gantryInventory struct {
	Inventory struct {
		Tools      []map[string]any `json:"tools"`
		Skills     []map[string]any `json:"skills"`
		MCPServers []map[string]any `json:"mcpServers"`
	} `json:"inventory"`
}

type gantryCapabilityList struct {
	Capabilities []gantryCapability `json:"capabilities"`
}

type gantryCapability struct {
	ID          string         `json:"id"`
	Version     any            `json:"version"`
	DisplayName string         `json:"displayName"`
	Category    string         `json:"category"`
	Risk        string         `json:"risk"`
	Can         string         `json:"can"`
	Cannot      string         `json:"cannot"`
	Source      string         `json:"source"`
	Raw         map[string]any `json:"-"`
}

func (g *gantryCapability) UnmarshalJSON(data []byte) error {
	type alias gantryCapability
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*g = gantryCapability(decoded)
	g.Raw = raw
	return nil
}

type gantrySkillBinding struct {
	ID              string `json:"id"`
	AgentID         string `json:"agentId"`
	SkillID         string `json:"skillId"`
	Status          string `json:"status"`
	ConfigVersionID string `json:"configVersionId"`
}

type gantrySkill struct {
	ID                string           `json:"id"`
	Name              string           `json:"name"`
	Description       string           `json:"description"`
	Source            string           `json:"source"`
	Status            string           `json:"status"`
	PromptRefs        []string         `json:"promptRefs"`
	ToolIDs           []string         `json:"toolIds"`
	WorkflowRefs      []string         `json:"workflowRefs"`
	RequiredEnvVars   []string         `json:"requiredEnvVars"`
	ActionPermissions []map[string]any `json:"actionPermissions"`
}

type gantryRun struct {
	RunID     string `json:"run_id"`
	JobID     string `json:"job_id"`
	StartedAt string `json:"started_at"`
	EndedAt   string `json:"ended_at"`
	Status    string `json:"status"`
}

type gantryRunEvent struct {
	ID        string         `json:"id"`
	RunID     string         `json:"runId"`
	Type      string         `json:"type"`
	Payload   map[string]any `json:"payload"`
	CreatedAt string         `json:"createdAt"`
	Metadata  struct {
		RuntimeEventType string `json:"runtimeEventType"`
	} `json:"metadata"`
}

type gantryJob struct {
	JobID  string `json:"jobId"`
	Target *struct {
		AgentID string `json:"agentId"`
	} `json:"target"`
}
