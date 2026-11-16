package upload

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/frogg24/file-share/internal/platform/httpauth"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

type uploadResponse struct {
	ID           string    `json:"id"`
	OwnerID      string    `json:"owner_id"`
	OriginalName string    `json:"original_name"`
	Size         int64     `json:"size"`
	CreatedAt    time.Time `json:"created_at"`
}

// Upload accepts exactly one file in a multipart/form-data field named "file".
// RequireAuth must wrap this handler to establish the verified owner ID.
func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := httpauth.UserID(r.Context())
	if !ok {
		w.Header().Set("WWW-Authenticate", "Bearer")
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "требуется авторизация"})
		return
	}
	// The HTTP body includes multipart boundaries and headers as well as bytes.
	const multipartOverhead int64 = 64 * 1024
	r.Body = http.MaxBytesReader(w, r.Body, MaxFileSize+multipartOverhead)
	reader, err := r.MultipartReader()
	if err != nil {
		writeReadError(w, err)
		return
	}
	part, err := reader.NextPart()
	if err != nil {
		writeReadError(w, err)
		return
	}
	defer part.Close()
	if part.FormName() != "file" || part.FileName() == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ожидается файл в поле file"})
		return
	}
	originalName := part.FileName()
	// One extra byte lets us detect an oversized file without reading it all.
	content, err := io.ReadAll(io.LimitReader(part, MaxFileSize+1))
	if err != nil {
		writeReadError(w, err)
		return
	}
	if int64(len(content)) > MaxFileSize {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": ErrFileTooLarge.Error()})
		return
	}
	// This endpoint accepts only one file, with no additional form fields.
	_, err = reader.NextPart()
	if err != io.EOF {
		if err != nil {
			writeReadError(w, err)
		} else {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "отправьте ровно один файл в поле file"})
		}
		return
	}
	file, err := h.service.Upload(r.Context(), UploadInput{
		OwnerID:      ownerID,
		OriginalName: originalName,
		Content:      content,
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrFileTooLarge):
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": err.Error()})
		case errors.Is(err, ErrInvalidInput):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		default:
			log.Printf("File upload failed: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "внутренняя ошибка сервера"})
		}
		return
	}
	writeJSON(w, http.StatusCreated, uploadResponse{
		ID:           file.ID,
		OwnerID:      file.OwnerID,
		OriginalName: file.OriginalName,
		Size:         file.Size,
		CreatedAt:    file.CreatedAt,
	})
}
func writeReadError(w http.ResponseWriter, err error) {
	var limitErr *http.MaxBytesError
	if errors.As(err, &limitErr) {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": ErrFileTooLarge.Error()})
		return
	}
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": "не удалось прочитать форму с файлом"})
}
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Failed to write upload response: %v", err)
	}
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := httpauth.UserID(r.Context())
	if !ok {
		w.Header().Set("WWW-Authenticate", "Bearer")
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "требуется авторизация",
		})
		return
	}

	err := h.service.Delete(r.Context(), r.PathValue("id"), ownerID)
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

		case errors.Is(err, ErrEventPublicationFailed):
			log.Printf("File deletion notification failed: %v", err)
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"error": "удаление отмечено, но отправить уведомление не удалось; повторите запрос",
			})

		default:
			log.Printf("File deletion failed: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "внутренняя ошибка сервера",
			})
		}
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}
