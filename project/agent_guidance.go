package project

// AgentGuidance identifies the durable framework guidance rendered into native agent files.
type AgentGuidance string

const (
	// AgentGuidanceBaseline renders GoForj's canonical baseline into selected native instruction files.
	AgentGuidanceBaseline AgentGuidance = "baseline"
	// AgentGuidanceNone keeps GoForj-managed guidance absent without removing user-authored instructions.
	AgentGuidanceNone AgentGuidance = "none"
)

// Valid reports whether the guidance value has defined render semantics.
func (guidance AgentGuidance) Valid() bool {
	return guidance == AgentGuidanceBaseline || guidance == AgentGuidanceNone
}
