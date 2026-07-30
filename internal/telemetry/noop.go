package telemetry

type NoopAdapter struct{}

func (n *NoopAdapter) StartTrial(ctx TrialContext)   {}
func (n *NoopAdapter) FinishTrial(result TrialResult) {}
func (n *NoopAdapter) Flush()                         {}
