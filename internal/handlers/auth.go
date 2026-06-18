package handlers

import (
	"errors"
	"strings"
	"time"

	"clinithink/internal/response"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
)

type registerRequest struct {
	Name        string `json:"name"`
	Email       string `json:"email"`
	Password    string `json:"password"`
	Institution string `json:"institution"`
	CohortYear  *int   `json:"cohort_year"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handler) Register(c *fiber.Ctx) error {
	var req registerRequest
	if err := c.BodyParser(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "BAD_REQUEST", "Format request tidak valid")
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))

	if req.Name == "" || req.Email == "" || req.Password == "" {
		return response.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "name, email, dan password wajib diisi")
	}
	if len(req.Password) < 8 {
		return response.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "Password minimal 8 karakter")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan pada server")
	}

	var id, name, email string
	err = h.db.QueryRow(c.Context(), `
		INSERT INTO students (name, email, password_hash, institution, cohort_year)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5)
		RETURNING id, name, email`,
		req.Name, req.Email, string(hash), req.Institution, req.CohortYear,
	).Scan(&id, &name, &email)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return response.Error(c, fiber.StatusConflict, "EMAIL_TAKEN", "Email sudah terdaftar")
		}
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan pada server")
	}

	return response.OK(c, fiber.Map{
		"id":    id,
		"name":  name,
		"email": email,
	})
}

func (h *Handler) Login(c *fiber.Ctx) error {
	var req loginRequest
	if err := c.BodyParser(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "BAD_REQUEST", "Format request tidak valid")
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" || req.Password == "" {
		return response.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "email dan password wajib diisi")
	}

	var id, passwordHash string
	err := h.db.QueryRow(c.Context(),
		`SELECT id, password_hash FROM students WHERE email = $1`, req.Email,
	).Scan(&id, &passwordHash)
	if err != nil {
		return response.Error(c, fiber.StatusUnauthorized, "INVALID_CREDENTIALS", "Email atau password salah")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)); err != nil {
		return response.Error(c, fiber.StatusUnauthorized, "INVALID_CREDENTIALS", "Email atau password salah")
	}

	expiry, err := time.ParseDuration(h.cfg.JWTExpiry)
	if err != nil {
		expiry = 24 * time.Hour
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  id,
		"role": "student",
		"exp":  time.Now().Add(expiry).Unix(),
		"iat":  time.Now().Unix(),
	})

	tokenStr, err := token.SignedString([]byte(h.cfg.JWTSecret))
	if err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan pada server")
	}

	return response.OK(c, fiber.Map{
		"token":      tokenStr,
		"expires_in": h.cfg.JWTExpiry,
	})
}
