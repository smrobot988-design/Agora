package internal

import (
	"github.com/smrobot988-design/Agora/pkg/core/trace"
	"github.com/smrobot988-design/Agora/pkg/orchestrator"
)

// NewOrchestratorTracer creates an OrchestratorTracer that exports traces to the traces/ directory.
func NewOrchestratorTracer(patternName string) *orchestrator.OrchestratorTracer {
	exporter := &trace.JSONExporter{Dir: "traces"}
	return orchestrator.NewOrchestratorTracer(patternName, exporter)
}
