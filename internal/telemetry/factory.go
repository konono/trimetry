package telemetry

import "github.com/konono/trimetry/internal/config"

func NewFromConfig(cfg *config.Config) Adapter {
	if !cfg.Telemetry.Enabled {
		return &NoopAdapter{}
	}
	switch cfg.Telemetry.Provider {
	case "mlflow":
		return NewMLflowAdapter(
			cfg.Telemetry.TrackingURI,
			cfg.Telemetry.Token,
			cfg.Telemetry.Workspace,
			cfg.Telemetry.TLSSkipVerify,
		)
	default:
		return NewLangfuseAdapter(LangfuseOptions{
			BaseURL:        cfg.Telemetry.BaseURL,
			PublicKey:      cfg.Telemetry.PublicKey,
			SecretKey:      cfg.Telemetry.SecretKey,
			TLSSkipVerify:  cfg.Telemetry.TLSSkipVerify,
			BatchChunkSize: cfg.Telemetry.BatchChunkSize,
			MaxRetries:     cfg.Telemetry.MaxRetries,
			MaxBatchQueue:  cfg.Telemetry.MaxBatchQueue,
		})
	}
}
