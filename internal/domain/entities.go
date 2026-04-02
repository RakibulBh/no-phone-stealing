package domain

import (
	"fmt"
	"strings"
	"time"
)

const MaxImageSize = 5 * 1024 * 1024 // 5MB

// Greater London bounding box
const (
	LondonMinLat = 51.28
	LondonMaxLat = 51.70
	LondonMinLng = -0.51
	LondonMaxLng = 0.33
)

type Location struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

func (l Location) Validate() error {
	if l.Lat < LondonMinLat || l.Lat > LondonMaxLat {
		return fmt.Errorf("latitude %f outside Greater London bounds [%f, %f]", l.Lat, LondonMinLat, LondonMaxLat)
	}
	if l.Lng < LondonMinLng || l.Lng > LondonMaxLng {
		return fmt.Errorf("longitude %f outside Greater London bounds [%f, %f]", l.Lng, LondonMinLng, LondonMaxLng)
	}
	return nil
}

type Report struct {
	ID        string   `json:"id"`
	Location  Location `json:"location"`
	TheftType string   `json:"theft_type"`
	ImageData []byte   `json:"-"`
}

func (r Report) Validate() error {
	if err := r.Location.Validate(); err != nil {
		return fmt.Errorf("invalid location: %w", err)
	}
	if r.TheftType == "" {
		return fmt.Errorf("theft_type is required")
	}
	if len(r.ImageData) == 0 {
		return fmt.Errorf("image is required")
	}
	if len(r.ImageData) > MaxImageSize {
		return fmt.Errorf("image exceeds maximum size of %d bytes", MaxImageSize)
	}
	return nil
}

type HistoricalCrime struct {
	ID       int     `json:"id"`
	Date     string  `json:"date"`
	Street   string  `json:"street"`
	Category string  `json:"category"`
	Lat      float64 `json:"lat"`
	Lng      float64 `json:"lng"`
}

func (c HistoricalCrime) FormatAsText() string {
	return fmt.Sprintf("On %s at %s, %s occurred.", c.Date, c.Street, c.Category)
}

// FormatCrimesAsText converts a slice of crimes into the plain-text format the LLM expects.
func FormatCrimesAsText(crimes []HistoricalCrime) string {
	if len(crimes) == 0 {
		return "No recent crimes recorded in this area."
	}
	lines := make([]string, len(crimes))
	for i, c := range crimes {
		lines[i] = c.FormatAsText()
	}
	return strings.Join(lines, "\n")
}

type LLMAnalysisResult struct {
	IsThreat      bool   `json:"is_threat"`
	ThreatLevel   int    `json:"threat_level"`
	Description   string `json:"description"`
	TrendAnalysis string `json:"trend_analysis"`
}

// IsActionable returns true when the analysis warrants alerting users.
func (r LLMAnalysisResult) IsActionable() bool {
	return r.IsThreat || r.ThreatLevel >= 3
}

type EnrichedAlert struct {
	Report    Report            `json:"report"`
	Analysis  LLMAnalysisResult `json:"analysis"`
	CreatedAt time.Time         `json:"created_at"`
}
