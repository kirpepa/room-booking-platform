package jwt

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	jwtv5 "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func TestGenerateAndValidateToken(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	publicKey := &privateKey.PublicKey

	userID := uuid.New()
	role := "admin"
	ttl := 1 * time.Hour

	token, err := GenerateToken(privateKey, userID, role, ttl)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	claims, err := ValidateToken(publicKey, token)
	if err != nil {
		t.Fatalf("validate token: %v", err)
	}
	if claims.UserID != userID {
		t.Errorf("expected user_id %s, got %s", userID, claims.UserID)
	}
	if claims.Role != role {
		t.Errorf("expected role %s, got %s", role, claims.Role)
	}
}

func TestValidateToken_Invalid(t *testing.T) {
	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	otherKey, _ := rsa.GenerateKey(rand.Reader, 2048)

	token, _ := GenerateToken(privateKey, uuid.New(), "user", time.Hour)

	// Validate with wrong key should fail
	_, err := ValidateToken(&otherKey.PublicKey, token)
	if err == nil {
		t.Error("expected error for wrong key")
	}

	// Validate garbage should fail
	_, err = ValidateToken(&privateKey.PublicKey, "garbage")
	if err == nil {
		t.Error("expected error for garbage token")
	}
}

func TestValidateToken_Expired(t *testing.T) {
	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	now := time.Now().UTC()
	claims := Claims{
		UserID: uuid.New(),
		Role:   "user",
		RegisteredClaims: jwtv5.RegisteredClaims{
			ExpiresAt: jwtv5.NewNumericDate(now.Add(-time.Hour)),
			IssuedAt:  jwtv5.NewNumericDate(now.Add(-2 * time.Hour)),
			Issuer:    "room-booking",
		},
	}
	token, _ := jwtv5.NewWithClaims(jwtv5.SigningMethodRS256, claims).SignedString(privateKey)

	_, err := ValidateToken(&privateKey.PublicKey, token)
	if err == nil {
		t.Error("expected error for expired token")
	}
}

func TestValidateToken_RejectsWrongAlgorithmAndIssuer(t *testing.T) {
	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	now := time.Now().UTC()
	claims := Claims{
		UserID: uuid.New(),
		Role:   "user",
		RegisteredClaims: jwtv5.RegisteredClaims{
			ExpiresAt: jwtv5.NewNumericDate(now.Add(time.Hour)),
			IssuedAt:  jwtv5.NewNumericDate(now),
			Issuer:    "another-issuer",
		},
	}
	wrongIssuer, _ := jwtv5.NewWithClaims(jwtv5.SigningMethodRS256, claims).SignedString(privateKey)
	if _, err := ValidateToken(&privateKey.PublicKey, wrongIssuer); err == nil {
		t.Fatal("expected issuer validation error")
	}

	claims.Issuer = "room-booking"
	ps256, _ := jwtv5.NewWithClaims(jwtv5.SigningMethodPS256, claims).SignedString(privateKey)
	if _, err := ValidateToken(&privateKey.PublicKey, ps256); err == nil {
		t.Fatal("expected strict RS256 algorithm validation error")
	}
}

func TestGenerateToken_RejectsInvalidClaims(t *testing.T) {
	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	if _, err := GenerateToken(privateKey, uuid.Nil, "user", time.Hour); err == nil {
		t.Fatal("expected nil user id to be rejected")
	}
	if _, err := GenerateToken(privateKey, uuid.New(), "owner", time.Hour); err == nil {
		t.Fatal("expected unknown role to be rejected")
	}
}

func TestGenerateToken_Roles(t *testing.T) {
	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)

	for _, role := range []string{"admin", "user"} {
		token, err := GenerateToken(privateKey, uuid.New(), role, time.Hour)
		if err != nil {
			t.Errorf("generate token for role %s: %v", role, err)
		}
		claims, err := ValidateToken(&privateKey.PublicKey, token)
		if err != nil {
			t.Errorf("validate token for role %s: %v", role, err)
		}
		if claims.Role != role {
			t.Errorf("expected role %s, got %s", role, claims.Role)
		}
	}
}
