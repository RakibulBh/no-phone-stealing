package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/rakibulbh/safe-london/internal/domain"
)

type envelope struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// ReportProcessor abstracts the use case so handlers don't depend on concrete types.
type ReportProcessor interface {
	ProcessAndBroadcast(ctx context.Context, report domain.Report) error
}

type Handler struct {
	processor ReportProcessor
	histRepo  domain.HistoricalDataRepo
}

func NewHandler(processor ReportProcessor, histRepo domain.HistoricalDataRepo) *Handler {
	return &Handler{processor: processor, histRepo: histRepo}
}

func (h *Handler) PostReport(c echo.Context) error {
	metaField := c.FormValue("metadata")
	if metaField == "" {
		return c.JSON(http.StatusBadRequest, envelope{Error: "metadata field is required"})
	}

	var meta struct {
		Lat       float64 `json:"lat"`
		Lng       float64 `json:"lng"`
		TheftType string  `json:"theft_type"`
	}
	if err := json.Unmarshal([]byte(metaField), &meta); err != nil {
		return c.JSON(http.StatusBadRequest, envelope{Error: "invalid metadata JSON"})
	}

	file, err := c.FormFile("image")
	if err != nil {
		return c.JSON(http.StatusBadRequest, envelope{Error: "image file is required"})
	}

	src, err := file.Open()
	if err != nil {
		return c.JSON(http.StatusBadRequest, envelope{Error: "cannot read image"})
	}
	defer src.Close()

	imageData, err := io.ReadAll(src)
	if err != nil {
		return c.JSON(http.StatusBadRequest, envelope{Error: "cannot read image data"})
	}

	report := domain.Report{
		ID:        uuid.NewString(),
		Location:  domain.Location{Lat: meta.Lat, Lng: meta.Lng},
		TheftType: meta.TheftType,
		ImageData: imageData,
	}

	if err := report.Validate(); err != nil {
		return c.JSON(http.StatusBadRequest, envelope{Error: fmt.Sprintf("validation failed: %s", err)})
	}

	// Fire and forget — async processing
	go func() {
		if err := h.processor.ProcessAndBroadcast(context.Background(), report); err != nil {
			slog.Error("async processing failed", "id", report.ID, "err", err)
		}
	}()

	return c.JSON(http.StatusAccepted, envelope{
		Success: true,
		Data:    map[string]string{"id": report.ID, "status": "processing"},
	})
}

func (h *Handler) GetTrends(c echo.Context) error {
	latStr := c.QueryParam("lat")
	lngStr := c.QueryParam("lng")

	if latStr == "" || lngStr == "" {
		return c.JSON(http.StatusBadRequest, envelope{Error: "lat and lng query params are required"})
	}

	lat, err := strconv.ParseFloat(latStr, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, envelope{Error: "invalid lat"})
	}
	lng, err := strconv.ParseFloat(lngStr, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, envelope{Error: "invalid lng"})
	}

	crimes, err := h.histRepo.GetRecentCrimes(c.Request().Context(), lat, lng)
	if err != nil {
		slog.Error("get trends failed", "err", err)
		return c.JSON(http.StatusInternalServerError, envelope{Error: "internal error"})
	}

	return c.JSON(http.StatusOK, envelope{
		Success: true,
		Data:    crimes,
	})
}
