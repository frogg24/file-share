package catalog

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/frogg24/file-share/internal/platform/httpauth"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	input := ListInput{
		Search:  query.Get("search"),
		SortBy:  query.Get("sort"),
		Order:   query.Get("order"),
		OwnerID: query.Get("owner_id"),
	}
	if value := query.Get("page"); value != "" {
		page, err := strconv.Atoi(value)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "page должен быть целым числом"})
			return
		}
		input.Page = page
	}
	if value := query.Get("page_size"); value != "" {
		pageSize, err := strconv.Atoi(value)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "page_size должен быть целым числом"})
			return
		}
		input.PageSize = pageSize
	}
	result, err := h.service.List(r.Context(), input)
	if err != nil {
		if errors.Is(err, ErrInvalidInput) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		} else {
			log.Printf("Catalog list failed: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "внутренняя ошибка сервера"})
		}
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Failed to write catalog response: %v", err)
	}
}

func (h *Handler) FileStats(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := httpauth.UserID(r.Context())
	if !ok {
		w.Header().Set("WWW-Authenticate", "Bearer")
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "требуется авторизация",
		})
		return
	}

	fileID := r.PathValue("id")

	stats, err := h.service.FileStats(r.Context(), fileID, ownerID)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidInput):
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "некорректный ID файла",
			})

		case errors.Is(err, ErrFileNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{
				"error": "файл не найден",
			})

		default:
			log.Printf("File stats failed: file_id=%s: %v", fileID, err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "внутренняя ошибка сервера",
			})
		}
		return
	}

	writeJSON(w, http.StatusOK, stats)
}
