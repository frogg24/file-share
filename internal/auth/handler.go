package auth

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
)

type Handler struct {
	service *Service
}

type registerRequest struct {
	Login       string `json:"login"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

type registerResponse struct {
	ID          string `json:"id"`
	Login       string `json:"login"`
	DisplayName string `json:"display_name"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var request registerRequest

	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{
			Error: "некорректный JSON запроса",
		})
		return
	}

	user, err := h.service.Register(r.Context(), RegisterInput{
		Login:       request.Login,
		Password:    request.Password,
		DisplayName: request.DisplayName,
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrLoginTaken):
			writeJSON(w, http.StatusConflict, errorResponse{
				Error: "логин уже занят",
			})

		case errors.Is(err, ErrInvalidInput):
			writeJSON(w, http.StatusBadRequest, errorResponse{
				Error: "некорректные данные регистрации",
			})

		default:
			log.Printf("Registration failed: %v", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{
				Error: "внутренняя ошибка сервера",
			})
		}
		return
	}

	response := registerResponse{
		ID:          user.ID,
		Login:       user.Login,
		DisplayName: user.DisplayName,
	}

	writeJSON(w, http.StatusCreated, response)
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	err := json.NewEncoder(w).Encode(data)
	if err != nil {
		log.Printf("Failed to write JSON response: %v", err)
	}
}
