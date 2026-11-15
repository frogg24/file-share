package download

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"mime"
	"net/http"
	"strconv"

	"github.com/frogg24/file-share/internal/events"
	"github.com/frogg24/file-share/internal/platform/httpauth"
	"github.com/google/uuid"
)

type EventPublisher interface {
	Publish(ctx context.Context, eventType string, data any) error
}
type Handler struct {
	repository *Repository
	publisher  EventPublisher
}

func NewHandler(repository *Repository, publisher EventPublisher) *Handler {
	return &Handler{repository: repository, publisher: publisher}
}
func (h *Handler) Download(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if len(id) != 36 {
		writeError(w, http.StatusBadRequest, "некорректный ID файла")
		return
	}
	if _, err := uuid.Parse(id); err != nil {
		writeError(w, http.StatusBadRequest, "некорректный ID файла")
		return
	}
	file, err := h.repository.FindByID(r.Context(), id)
	if errors.Is(err, ErrFileNotFound) {
		writeError(w, http.StatusNotFound, "файл не найден")
		return
	}
	if err != nil {
		log.Printf("Find file %s failed: %v", id, err)
		writeError(w, http.StatusInternalServerError, "внутренняя ошибка сервера")
		return
	}

	if r.Method != http.MethodHead {
		record := DownloadRecord{
			ID:     uuid.NewString(),
			FileID: file.ID,
		}
		if userID, ok := httpauth.UserID(r.Context()); ok {
			record.UserID = &userID
		}
		if err := h.repository.RecordDownload(r.Context(), &record); err != nil {
			log.Printf("Record download for file %s failed: %v", file.ID, err)
			writeError(w, http.StatusInternalServerError, "внутренняя ошибка сервера")
			return
		}
		event := events.FileDownloaded{
			DownloadID:   record.ID,
			FileID:       record.FileID,
			UserID:       record.UserID,
			DownloadedAt: record.DownloadedAt,
		}
		if err := h.publisher.Publish(r.Context(), events.FileDownloadedType, event); err != nil {
			log.Printf("Download %s recorded, but event publication failed: %v", record.ID, err)
		} else {
			log.Printf("Sent file.downloaded: download_id=%s, file_id=%s", record.ID, file.ID)
		}
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", mime.FormatMediaType(
		"attachment", map[string]string{"filename": file.OriginalName},
	))
	w.Header().Set("Content-Length", strconv.FormatInt(file.Size, 10))
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	if _, err := w.Write(file.Content); err != nil {
		log.Printf("Write file %s failed: %v", file.ID, err)
	}
}
func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]string{"error": message}); err != nil {
		log.Printf("Write download error response failed: %v", err)
	}
}
