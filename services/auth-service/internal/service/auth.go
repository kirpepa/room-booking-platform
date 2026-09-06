package service

import (
	"context"
	"crypto/rsa"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	pkgjwt "github.com/room-booking/pkg/jwt"
	"github.com/room-booking/services/auth-service/internal/repository"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidRole        = errors.New("invalid role")
	ErrEmailExists        = errors.New("email already exists")
	ErrInvalidEmail       = errors.New("invalid email")
	ErrInvalidPassword    = errors.New("password must contain 8 to 72 bytes")
	ErrTestModeDisabled   = errors.New("test task mode is disabled")
)

var (
	DummyAdminID = uuid.MustParse("00000000-0000-0000-0000-000000000001")
	DummyUserID  = uuid.MustParse("00000000-0000-0000-0000-000000000002")
)

const tokenTTL = 24 * time.Hour

type AuthService struct {
	repo         userRepository
	privateKey   *rsa.PrivateKey
	testTaskMode bool
}

type userRepository interface {
	Create(ctx context.Context, id uuid.UUID, email string, passwordHash *string, role string) (*repository.User, error)
	GetByEmail(ctx context.Context, email string) (*repository.User, error)
	EnsureUser(ctx context.Context, id uuid.UUID, email string, role string) error
}

func NewAuthService(repo userRepository, privateKey *rsa.PrivateKey, testTaskMode bool) *AuthService {
	return &AuthService{repo: repo, privateKey: privateKey, testTaskMode: testTaskMode}
}

type UserResponse struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"createdAt,omitempty"`
}

func (s *AuthService) Register(ctx context.Context, email, password, role string) (*UserResponse, error) {
	if role != "admin" && role != "user" {
		return nil, ErrInvalidRole
	}
	if role == "admin" && !s.testTaskMode {
		return nil, ErrTestModeDisabled
	}
	email = normalizeEmail(email)
	if !validEmail(email) {
		return nil, ErrInvalidEmail
	}
	if len(password) < 8 || len(password) > 72 {
		return nil, ErrInvalidPassword
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	hashStr := string(hash)

	id := uuid.New()
	u, err := s.repo.Create(ctx, id, email, &hashStr, role)
	if err != nil {
		if errors.Is(err, repository.ErrEmailExists) {
			return nil, ErrEmailExists
		}
		return nil, err
	}

	return &UserResponse{
		ID:        u.ID,
		Email:     u.Email,
		Role:      u.Role,
		CreatedAt: u.CreatedAt,
	}, nil
}

func (s *AuthService) Login(ctx context.Context, email, password string) (string, error) {
	email = normalizeEmail(email)
	u, err := s.repo.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return "", ErrInvalidCredentials
		}
		return "", err
	}
	if u.PasswordHash == nil {
		return "", ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(*u.PasswordHash), []byte(password)); err != nil {
		return "", ErrInvalidCredentials
	}
	return pkgjwt.GenerateToken(s.privateKey, u.ID, u.Role, tokenTTL)
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func validEmail(email string) bool {
	parsed, err := mail.ParseAddress(email)
	return err == nil && parsed.Address == email && strings.Contains(email, "@")
}

func (s *AuthService) DummyLogin(ctx context.Context, role string) (string, error) {
	if !s.testTaskMode {
		return "", ErrTestModeDisabled
	}
	if role != "admin" && role != "user" {
		return "", ErrInvalidRole
	}

	var userID uuid.UUID
	var email string
	if role == "admin" {
		userID = DummyAdminID
		email = "admin@test.com"
	} else {
		userID = DummyUserID
		email = "user@test.com"
	}

	if err := s.repo.EnsureUser(ctx, userID, email, role); err != nil {
		return "", fmt.Errorf("ensure dummy user: %w", err)
	}

	return pkgjwt.GenerateToken(s.privateKey, userID, role, tokenTTL)
}

func (s *AuthService) Seed(ctx context.Context) error {
	if !s.testTaskMode {
		return ErrTestModeDisabled
	}
	if err := s.repo.EnsureUser(ctx, DummyAdminID, "admin@test.com", "admin"); err != nil {
		return fmt.Errorf("seed admin: %w", err)
	}
	if err := s.repo.EnsureUser(ctx, DummyUserID, "user@test.com", "user"); err != nil {
		return fmt.Errorf("seed user: %w", err)
	}
	return nil
}
