package http

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/rakibulbh/safe-london/internal/domain"
)

// --- Manual mocks ---

type stubUseCase struct {
	called bool
	report domain.Report
}

func (s *stubUseCase) ProcessAndBroadcast(_ context.Context, r domain.Report) error {
	s.called = true
	s.report = r
	return nil
}

type stubHistRepo struct {
	crimes []domain.HistoricalCrime
}

func (s *stubHistRepo) GetRecentCrimes(_ context.Context, _, _ float64) ([]domain.HistoricalCrime, error) {
	return s.crimes, nil
}

// --- Tests ---

func TestPostReport_ValidPayload_Returns202(t *testing.T) {
	e := echo.New()
	uc := &stubUseCase{}
	h := NewHandler(uc, &stubHistRepo{})

	body, contentType := buildMultipartReport(t, 51.5074, -0.1278, "phone_snatch", []byte("fake-image-data"))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/reports", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.PostReport(c); err != nil {
		t.Fatalf("PostReport returned error: %v", err)
	}

	if rec.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", rec.Code)
	}

	var resp envelope
	json.NewDecoder(rec.Body).Decode(&resp)
	if !resp.Success {
		t.Error("expected success=true")
	}
}

func TestPostReport_MissingImage_Returns400(t *testing.T) {
	e := echo.New()
	h := NewHandler(&stubUseCase{}, &stubHistRepo{})

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.WriteField("metadata", `{"lat":51.5074,"lng":-0.1278,"theft_type":"phone_snatch"}`)
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/reports", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	h.PostReport(c)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestPostReport_InvalidLocation_Returns400(t *testing.T) {
	e := echo.New()
	h := NewHandler(&stubUseCase{}, &stubHistRepo{})

	body, contentType := buildMultipartReport(t, 0, 0, "phone_snatch", []byte("img"))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/reports", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	h.PostReport(c)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestPostReport_MissingMetadata_Returns400(t *testing.T) {
	e := echo.New()
	h := NewHandler(&stubUseCase{}, &stubHistRepo{})

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("image", "photo.jpg")
	part.Write([]byte("img"))
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/reports", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	h.PostReport(c)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestGetTrends_ReturnsHistoricalCrimes(t *testing.T) {
	e := echo.New()
	histRepo := &stubHistRepo{
		crimes: []domain.HistoricalCrime{
			{Date: "2024-01", Street: "Oxford Street", Category: "robbery", Lat: 51.5155, Lng: -0.1415},
		},
	}
	h := NewHandler(&stubUseCase{}, histRepo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/trends?lat=51.5074&lng=-0.1278", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.GetTrends(c); err != nil {
		t.Fatalf("GetTrends error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var resp envelope
	json.NewDecoder(rec.Body).Decode(&resp)
	if !resp.Success {
		t.Error("expected success=true")
	}
}

func TestGetTrends_MissingParams_Returns400(t *testing.T) {
	e := echo.New()
	h := NewHandler(&stubUseCase{}, &stubHistRepo{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/trends", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	h.GetTrends(c)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

// --- Helpers ---

func buildMultipartReport(t *testing.T, lat, lng float64, theftType string, image []byte) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	meta, _ := json.Marshal(map[string]interface{}{"lat": lat, "lng": lng, "theft_type": theftType})
	writer.WriteField("metadata", string(meta))

	part, err := writer.CreateFormFile("image", "photo.jpg")
	if err != nil {
		t.Fatal(err)
	}
	part.Write(image)
	writer.Close()

	return body, writer.FormDataContentType()
}
