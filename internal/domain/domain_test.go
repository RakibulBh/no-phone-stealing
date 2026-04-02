package domain

import (
	"testing"
	"time"
)

func TestLocationValidation_WithinLondon(t *testing.T) {
	tests := []struct {
		name    string
		lat     float64
		lng     float64
		wantErr bool
	}{
		{"central london", 51.5074, -0.1278, false},
		{"east london", 51.5155, 0.0922, false},
		{"south london", 51.4545, -0.0983, false},
		{"too far north", 52.0, -0.1278, true},
		{"too far south", 51.2, -0.1278, true},
		{"too far east", 51.5074, 0.4, true},
		{"too far west", 51.5074, -0.6, true},
		{"zero values", 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc := Location{Lat: tt.lat, Lng: tt.lng}
			err := loc.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Location{%f, %f}.Validate() error = %v, wantErr %v", tt.lat, tt.lng, err, tt.wantErr)
			}
		})
	}
}

func TestReport_Validate(t *testing.T) {
	validLoc := Location{Lat: 51.5074, Lng: -0.1278}

	t.Run("valid report", func(t *testing.T) {
		r := Report{
			Location:  validLoc,
			TheftType: "phone_snatch",
			ImageData: []byte("fake-image-bytes"),
		}
		if err := r.Validate(); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("missing image", func(t *testing.T) {
		r := Report{Location: validLoc, TheftType: "phone_snatch"}
		if err := r.Validate(); err == nil {
			t.Error("expected error for missing image")
		}
	})

	t.Run("missing theft type", func(t *testing.T) {
		r := Report{Location: validLoc, ImageData: []byte("img")}
		if err := r.Validate(); err == nil {
			t.Error("expected error for missing theft type")
		}
	})

	t.Run("invalid location", func(t *testing.T) {
		r := Report{Location: Location{Lat: 0, Lng: 0}, TheftType: "phone_snatch", ImageData: []byte("img")}
		if err := r.Validate(); err == nil {
			t.Error("expected error for invalid location")
		}
	})

	t.Run("image too large", func(t *testing.T) {
		bigImage := make([]byte, 6*1024*1024)
		r := Report{Location: validLoc, TheftType: "phone_snatch", ImageData: bigImage}
		if err := r.Validate(); err == nil {
			t.Error("expected error for oversized image")
		}
	})
}

func TestHistoricalCrime_FormatAsText(t *testing.T) {
	crime := HistoricalCrime{
		Date:     "2024-01",
		Street:   "Oxford Street",
		Category: "robbery",
		Lat:      51.5155,
		Lng:      -0.1415,
	}

	got := crime.FormatAsText()
	want := "On 2024-01 at Oxford Street, robbery occurred."
	if got != want {
		t.Errorf("FormatAsText() = %q, want %q", got, want)
	}
}

func TestFormatCrimesAsText_MultipleCrimes(t *testing.T) {
	crimes := []HistoricalCrime{
		{Date: "2024-01", Street: "Oxford Street", Category: "robbery"},
		{Date: "2024-01", Street: "Baker Street", Category: "theft-from-the-person"},
	}

	got := FormatCrimesAsText(crimes)
	want := "On 2024-01 at Oxford Street, robbery occurred.\nOn 2024-01 at Baker Street, theft-from-the-person occurred."
	if got != want {
		t.Errorf("FormatCrimesAsText() = %q, want %q", got, want)
	}
}

func TestFormatCrimesAsText_Empty(t *testing.T) {
	got := FormatCrimesAsText(nil)
	want := "No recent crimes recorded in this area."
	if got != want {
		t.Errorf("FormatCrimesAsText(nil) = %q, want %q", got, want)
	}
}

func TestLLMAnalysisResult_IsActionable(t *testing.T) {
	t.Run("high threat is actionable", func(t *testing.T) {
		r := LLMAnalysisResult{IsThreat: true, ThreatLevel: 4}
		if !r.IsActionable() {
			t.Error("expected high threat to be actionable")
		}
	})

	t.Run("low threat not actionable", func(t *testing.T) {
		r := LLMAnalysisResult{IsThreat: false, ThreatLevel: 2}
		if r.IsActionable() {
			t.Error("expected low non-threat to not be actionable")
		}
	})

	t.Run("borderline threat level 3 with is_threat true", func(t *testing.T) {
		r := LLMAnalysisResult{IsThreat: true, ThreatLevel: 3}
		if !r.IsActionable() {
			t.Error("expected threat level 3 with is_threat=true to be actionable")
		}
	})

	t.Run("borderline threat level 3 with is_threat false", func(t *testing.T) {
		r := LLMAnalysisResult{IsThreat: false, ThreatLevel: 3}
		if !r.IsActionable() {
			t.Error("expected threat level >= 3 to be actionable regardless")
		}
	})
}

func TestEnrichedAlert_Fields(t *testing.T) {
	now := time.Now()
	alert := EnrichedAlert{
		Report: Report{
			ID:        "r-123",
			Location:  Location{Lat: 51.5074, Lng: -0.1278},
			TheftType: "moped_theft",
		},
		Analysis: LLMAnalysisResult{
			IsThreat:      true,
			ThreatLevel:   4,
			Description:   "Active moped theft in progress",
			TrendAnalysis: "5 similar incidents in past month, suspects flee north",
		},
		CreatedAt: now,
	}

	if alert.Report.ID != "r-123" {
		t.Errorf("expected report ID r-123, got %s", alert.Report.ID)
	}
	if alert.Analysis.ThreatLevel != 4 {
		t.Errorf("expected threat level 4, got %d", alert.Analysis.ThreatLevel)
	}
	if alert.CreatedAt != now {
		t.Error("timestamp mismatch")
	}
}
