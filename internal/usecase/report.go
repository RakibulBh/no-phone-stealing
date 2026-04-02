package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/rakibulbh/safe-london/internal/domain"
)

type ReportUseCase struct {
	histRepo    domain.HistoricalDataRepo
	analyzer    domain.LLMAnalyzer
	reportRepo  domain.ReportRepository
	broadcaster domain.AlertBroadcaster
}

func NewReportUseCase(
	histRepo domain.HistoricalDataRepo,
	analyzer domain.LLMAnalyzer,
	reportRepo domain.ReportRepository,
	broadcaster domain.AlertBroadcaster,
) *ReportUseCase {
	return &ReportUseCase{
		histRepo:    histRepo,
		analyzer:    analyzer,
		reportRepo:  reportRepo,
		broadcaster: broadcaster,
	}
}

// ProcessAndBroadcast orchestrates the full report pipeline (designed to run in a goroutine).
func (uc *ReportUseCase) ProcessAndBroadcast(ctx context.Context, report domain.Report) error {
	if err := report.Validate(); err != nil {
		return fmt.Errorf("validate report: %w", err)
	}

	crimes, err := uc.histRepo.GetRecentCrimes(ctx, report.Location.Lat, report.Location.Lng)
	if err != nil {
		return fmt.Errorf("get historical crimes: %w", err)
	}

	historyText := domain.FormatCrimesAsText(crimes)

	analysis, err := uc.analyzer.Analyze(ctx, report.ImageData, historyText)
	if err != nil {
		return fmt.Errorf("llm analysis: %w", err)
	}

	if !analysis.IsActionable() {
		slog.Info("low threat report discarded", "id", report.ID, "level", analysis.ThreatLevel)
		return nil
	}

	alert := domain.EnrichedAlert{
		Report:    report,
		Analysis:  *analysis,
		CreatedAt: time.Now(),
	}

	// Persist best-effort; broadcasting takes priority for safety
	if err := uc.reportRepo.Save(ctx, alert); err != nil {
		slog.Error("failed to save report, still broadcasting", "id", report.ID, "err", err)
	}

	uc.broadcaster.Broadcast(alert)
	return nil
}
