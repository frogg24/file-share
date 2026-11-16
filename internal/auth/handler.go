package auth

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/frogg24/file-share/internal/platform/httpauth"
	"github.com/frogg24/file-share/internal/platform/session"
)

type Handler struct {
	service       *Service
	tokenIssuer   *TokenIssuer
	secureCookies bool
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

type loginRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

type loginResponse struct {
	ID          string `json:"id"`
	Login       string `json:"login"`
	DisplayName string `json:"display_name"`
}

type errorResponse struct {
	Error string `json:"error"`
}

type updateProfileRequest struct {
	DisplayName string `json:"display_name"`
}

func NewHandler(service *Service, tokenIssuer *TokenIssuer, secureCookies bool) *Handler {
	return &Handler{service: service, tokenIssuer: tokenIssuer, secureCookies: secureCookies}
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var request loginRequest

	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{
			Error: "некорректный JSON запроса",
		})
		return
	}

	user, err := h.service.Login(r.Context(), LoginInput{
		Login:    request.Login,
		Password: request.Password,
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidCredentials):
			writeJSON(w, http.StatusUnauthorized, errorResponse{
				Error: "неверный логин или пароль",
			})

		default:
			log.Printf("Login failed: %v", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{
				Error: "внутренняя ошибка сервера",
			})
		}
		return
	}

	accessToken, err := h.tokenIssuer.Issue(user.ID)
	if err != nil {
		log.Printf("Token issuance failed: %v", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{
			Error: "внутренняя ошибка сервера",
		})
		return
	}

	if err := session.SetCookie(w, accessToken, h.tokenIssuer.ttl, h.secureCookies); err != nil {
		log.Printf("Session cookie creation failed: %v", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "внутренняя ошибка сервера"})
		return
	}

	response := loginResponse{
		ID:          user.ID,
		Login:       user.Login,
		DisplayName: user.DisplayName,
	}

	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	session.ClearCookie(w, h.secureCookies)
	writeJSON(w, http.StatusOK, map[string]string{"message": "вы вышли"})
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

func (h *Handler) Profile(w http.ResponseWriter, r *http.Request) {
	id, ok := httpauth.UserID(r.Context())
	if !ok {
		w.Header().Set("WWW-Authenticate", "Bearer")
		writeJSON(w, http.StatusUnauthorized, errorResponse{
			Error: "требуется авторизация",
		})
		return
	}

	profile, err := h.service.Profile(r.Context(), id)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidInput):
			writeJSON(w, http.StatusBadRequest, errorResponse{
				Error: "некорректный ID пользователя",
			})
		case errors.Is(err, ErrUserNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{
				Error: "пользователь не найден",
			})
		default:
			log.Printf("Profile lookup failed: %v", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{
				Error: "внутренняя ошибка сервера",
			})
		}
		return
	}

	writeJSON(w, http.StatusOK, profile)
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	err := json.NewEncoder(w).Encode(data)
	if err != nil {
		log.Printf("Failed to write JSON response: %v", err)
	}
}

func (h *Handler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	id, ok := httpauth.UserID(r.Context())
	if !ok {
		w.Header().Set("WWW-Authenticate", "Bearer")
		writeJSON(w, http.StatusUnauthorized, errorResponse{
			Error: "требуется авторизация",
		})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 4096)

	var request updateProfileRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&request); err != nil {
		var limitErr *http.MaxBytesError
		if errors.As(err, &limitErr) {
			writeJSON(w, http.StatusRequestEntityTooLarge, errorResponse{
				Error: "слишком большой запрос",
			})
		} else {
			writeJSON(w, http.StatusBadRequest, errorResponse{
				Error: "некорректный JSON запроса",
			})
		}
		return
	}

	profile, err := h.service.UpdateDisplayName(r.Context(), id, request.DisplayName)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidInput):
			writeJSON(w, http.StatusBadRequest, errorResponse{
				Error: "некорректные данные профиля",
			})
		case errors.Is(err, ErrUserNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{
				Error: "пользователь не найден",
			})
		default:
			log.Printf("Profile update failed: %v", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{
				Error: "внутренняя ошибка сервера",
			})
		}
		return
	}

	writeJSON(w, http.StatusOK, profile)
}
