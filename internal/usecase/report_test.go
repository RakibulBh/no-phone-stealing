package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/rakibulbh/safe-london/internal/domain"
)

// --- Manual mocks (no external libraries) ---

type mockHistoricalRepo struct {
	crimes []domain.HistoricalCrime
	err    error
}

func (m *mockHistoricalRepo) GetRecentCrimes(_ context.Context, _, _ float64) ([]domain.HistoricalCrime, error) {
	return m.crimes, m.err
}

type mockLLMAnalyzer struct {
	result *domain.LLMAnalysisResult
	err    error
	// Capture what was sent to verify prompt construction
	receivedHistory string
}

func (m *mockLLMAnalyzer) Analyze(_ context.Context, _ []byte, history string) (*domain.LLMAnalysisResult, error) {
	m.receivedHistory = history
	return m.result, m.err
}

type mockReportRepo struct {
	saved *domain.EnrichedAlert
	err   error
}

func (m *mockReportRepo) Save(_ context.Context, alert domain.EnrichedAlert) error {
	m.saved = &alert
	return m.err
}

type mockBroadcaster struct {
	broadcasted *domain.EnrichedAlert
}

func (m *mockBroadcaster) Broadcast(alert domain.EnrichedAlert) {
	m.broadcasted = &alert
}

// --- Tests ---

func TestProcessAndBroadcast_FullFlow(t *testing.T) {
	histRepo := &mockHistoricalRepo{
		crimes: []domain.HistoricalCrime{
			{Date: "2024-01", Street: "Oxford Street", Category: "robbery"},
		},
	}
	analyzer := &mockLLMAnalyzer{
		result: &domain.LLMAnalysisResult{
			IsThreat:      true,
			ThreatLevel:   4,
			Description:   "Active theft",
			TrendAnalysis: "Suspects flee north",
		},
	}
	reportRepo := &mockReportRepo{}
	broadcaster := &mockBroadcaster{}

	uc := NewReportUseCase(histRepo, analyzer, reportRepo, broadcaster)

	report := domain.Report{
		ID:        "r-1",
		Location:  domain.Location{Lat: 51.5074, Lng: -0.1278},
		TheftType: "phone_snatch",
		ImageData: []byte("fake-image"),
	}

	err := uc.ProcessAndBroadcast(context.Background(), report)
	if err != nil {
		t.Fatalf("ProcessAndBroadcast failed: %v", err)
	}

	// Verify historical context was formatted as text for LLM
	expectedHistory := "On 2024-01 at Oxford Street, robbery occurred."
	if analyzer.receivedHistory != expectedHistory {
		t.Errorf("expected history %q, got %q", expectedHistory, analyzer.receivedHistory)
	}

	// Verify report was saved
	if reportRepo.saved == nil {
		t.Fatal("expected report to be saved")
	}
	if reportRepo.saved.Report.ID != "r-1" {
		t.Errorf("saved report ID mismatch: %s", reportRepo.saved.Report.ID)
	}
	if reportRepo.saved.Analysis.ThreatLevel != 4 {
		t.Errorf("saved threat level mismatch: %d", reportRepo.saved.Analysis.ThreatLevel)
	}

	// Verify broadcast was called
	if broadcaster.broadcasted == nil {
		t.Fatal("expected alert to be broadcasted")
	}
	if broadcaster.broadcasted.Analysis.TrendAnalysis != "Suspects flee north" {
		t.Errorf("broadcast trend mismatch: %s", broadcaster.broadcasted.Analysis.TrendAnalysis)
	}
}

func TestProcessAndBroadcast_LowThreatAborts(t *testing.T) {
	histRepo := &mockHistoricalRepo{crimes: nil}
	analyzer := &mockLLMAnalyzer{
		result: &domain.LLMAnalysisResult{
			IsThreat:    false,
			ThreatLevel: 2,
			Description: "Nothing suspicious",
		},
	}
	reportRepo := &mockReportRepo{}
	broadcaster := &mockBroadcaster{}

	uc := NewReportUseCase(histRepo, analyzer, reportRepo, broadcaster)

	report := domain.Report{
		ID:        "r-2",
		Location:  domain.Location{Lat: 51.5074, Lng: -0.1278},
		TheftType: "phone_snatch",
		ImageData: []byte("fake-image"),
	}

	err := uc.ProcessAndBroadcast(context.Background(), report)
	if err != nil {
		t.Fatalf("should not error on low threat: %v", err)
	}

	// Should NOT save or broadcast for low threats
	if reportRepo.saved != nil {
		t.Error("should not save low-threat reports")
	}
	if broadcaster.broadcasted != nil {
		t.Error("should not broadcast low-threat reports")
	}
}

func TestProcessAndBroadcast_InvalidReport(t *testing.T) {
	uc := NewReportUseCase(&mockHistoricalRepo{}, &mockLLMAnalyzer{}, &mockReportRepo{}, &mockBroadcaster{})

	report := domain.Report{
		Location:  domain.Location{Lat: 0, Lng: 0},
		TheftType: "",
	}

	err := uc.ProcessAndBroadcast(context.Background(), report)
	if err == nil {
		t.Error("expected validation error for invalid report")
	}
}

func TestProcessAndBroadcast_HistoryRepoError(t *testing.T) {
	histRepo := &mockHistoricalRepo{err: errors.New("db down")}
	analyzer := &mockLLMAnalyzer{}
	reportRepo := &mockReportRepo{}
	broadcaster := &mockBroadcaster{}

	uc := NewReportUseCase(histRepo, analyzer, reportRepo, broadcaster)

	report := domain.Report{
		ID:        "r-3",
		Location:  domain.Location{Lat: 51.5074, Lng: -0.1278},
		TheftType: "phone_snatch",
		ImageData: []byte("img"),
	}

	err := uc.ProcessAndBroadcast(context.Background(), report)
	if err == nil {
		t.Error("expected error when history repo fails")
	}
}

func TestProcessAndBroadcast_LLMError(t *testing.T) {
	histRepo := &mockHistoricalRepo{crimes: nil}
	analyzer := &mockLLMAnalyzer{err: errors.New("openai timeout")}
	reportRepo := &mockReportRepo{}
	broadcaster := &mockBroadcaster{}

	uc := NewReportUseCase(histRepo, analyzer, reportRepo, broadcaster)

	report := domain.Report{
		ID:        "r-4",
		Location:  domain.Location{Lat: 51.5074, Lng: -0.1278},
		TheftType: "phone_snatch",
		ImageData: []byte("img"),
	}

	err := uc.ProcessAndBroadcast(context.Background(), report)
	if err == nil {
		t.Error("expected error when LLM fails")
	}
}

func TestProcessAndBroadcast_SaveError_StillBroadcasts(t *testing.T) {
	histRepo := &mockHistoricalRepo{crimes: nil}
	analyzer := &mockLLMAnalyzer{
		result: &domain.LLMAnalysisResult{IsThreat: true, ThreatLevel: 5, Description: "urgent"},
	}
	reportRepo := &mockReportRepo{err: errors.New("disk full")}
	broadcaster := &mockBroadcaster{}

	uc := NewReportUseCase(histRepo, analyzer, reportRepo, broadcaster)

	report := domain.Report{
		ID:        "r-5",
		Location:  domain.Location{Lat: 51.5074, Lng: -0.1278},
		TheftType: "phone_snatch",
		ImageData: []byte("img"),
	}

	// Should still broadcast even if save fails (safety-critical)
	err := uc.ProcessAndBroadcast(context.Background(), report)
	if err != nil {
		t.Fatalf("save error should not block processing: %v", err)
	}
	if broadcaster.broadcasted == nil {
		t.Error("should still broadcast even when save fails")
	}
}
