package service

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"testing"

	"github.com/google/uuid"
	"github.com/room-booking/services/auth-service/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

type stubUserRepository struct {
	createdEmail string
	createdHash  *string
	user         *repository.User
	getErr       error
}

func (s *stubUserRepository) Create(_ context.Context, id uuid.UUID, email string, passwordHash *string, role string) (*repository.User, error) {
	s.createdEmail = email
	s.createdHash = passwordHash
	return &repository.User{ID: id, Email: email, PasswordHash: passwordHash, Role: role}, nil
}

func (s *stubUserRepository) GetByEmail(_ context.Context, _ string) (*repository.User, error) {
	return s.user, s.getErr
}

func (s *stubUserRepository) EnsureUser(_ context.Context, _ uuid.UUID, _ string, _ string) error {
	return nil
}

func TestRegisterValidatesAndNormalizesCredentials(t *testing.T) {
	repo := &stubUserRepository{}
	svc := NewAuthService(repo, nil, false)

	user, err := svc.Register(context.Background(), "  Person@Example.COM ", "strong-password", "user")
	if err != nil {
		t.Fatal(err)
	}
	if user.Email != "person@example.com" || repo.createdEmail != "person@example.com" {
		t.Fatalf("email was not normalized: user=%q repo=%q", user.Email, repo.createdEmail)
	}
	if repo.createdHash == nil || *repo.createdHash == "strong-password" {
		t.Fatal("password was not hashed")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(*repo.createdHash), []byte("strong-password")); err != nil {
		t.Fatalf("stored bcrypt hash does not match password: %v", err)
	}
}

func TestRegisterRejectsUnsafeInput(t *testing.T) {
	svc := NewAuthService(&stubUserRepository{}, nil, false)
	tests := []struct {
		name     string
		email    string
		password string
		role     string
		want     error
	}{
		{name: "invalid role", email: "person@example.com", password: "strong-password", role: "owner", want: ErrInvalidRole},
		{name: "admin outside test mode", email: "admin@example.com", password: "strong-password", role: "admin", want: ErrTestModeDisabled},
		{name: "invalid email", email: "not-an-email", password: "strong-password", role: "user", want: ErrInvalidEmail},
		{name: "short password", email: "person@example.com", password: "short", role: "user", want: ErrInvalidPassword},
		{name: "bcrypt limit", email: "person@example.com", password: string(make([]byte, 73)), role: "user", want: ErrInvalidPassword},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := svc.Register(context.Background(), tt.email, tt.password, tt.role); err != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, err)
			}
		})
	}
}

func TestLoginReturnsSignedToken(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("strong-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	hashString := string(hash)
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	repo := &stubUserRepository{user: &repository.User{
		ID: uuid.New(), Email: "person@example.com", PasswordHash: &hashString, Role: "user",
	}}
	svc := NewAuthService(repo, privateKey, false)

	token, err := svc.Login(context.Background(), " PERSON@example.com ", "strong-password")
	if err != nil {
		t.Fatal(err)
	}
	if token == "" {
		t.Fatal("expected a signed token")
	}
}

func TestTestOnlyServiceMethodsRequireTestMode(t *testing.T) {
	svc := NewAuthService(&stubUserRepository{}, nil, false)
	if _, err := svc.DummyLogin(context.Background(), "admin"); err != ErrTestModeDisabled {
		t.Fatalf("expected disabled dummy login, got %v", err)
	}
	if err := svc.Seed(context.Background()); err != ErrTestModeDisabled {
		t.Fatalf("expected disabled seed, got %v", err)
	}
}
