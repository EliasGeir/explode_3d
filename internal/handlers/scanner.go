package handlers

import (
	"net/http"

	"3dmodels/internal/scanner"
	"3dmodels/templates"
)

type ScanHandler struct {
	scanner *scanner.Scanner
}

func NewScanHandler(s *scanner.Scanner) *ScanHandler {
	return &ScanHandler{scanner: s}
}

func (h *ScanHandler) StartScan(w http.ResponseWriter, r *http.Request) {
	h.scanner.StartScan()
	status := h.scanner.Status()
	templates.ScannerStatus(status).Render(r.Context(), w)
}

func (h *ScanHandler) Status(w http.ResponseWriter, r *http.Request) {
	status := h.scanner.Status()
	templates.ScannerStatus(status).Render(r.Context(), w)
}
