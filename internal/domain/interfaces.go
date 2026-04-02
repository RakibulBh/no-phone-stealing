package domain

import "context"

// HistoricalDataRepo abstracts access to cached Police UK crime data.
type HistoricalDataRepo interface {
	GetRecentCrimes(ctx context.Context, lat, lng float64) ([]HistoricalCrime, error)
}

// LLMAnalyzer abstracts the vision-capable LLM used for threat analysis.
type LLMAnalyzer interface {
	Analyze(ctx context.Context, imageData []byte, historicalContext string) (*LLMAnalysisResult, error)
}

// ReportRepository persists verified reports and their analysis.
type ReportRepository interface {
	Save(ctx context.Context, alert EnrichedAlert) error
}

// AlertBroadcaster pushes enriched alerts to connected clients.
type AlertBroadcaster interface {
	Broadcast(alert EnrichedAlert)
}
