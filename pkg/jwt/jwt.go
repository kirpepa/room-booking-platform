package jwt

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"time"

	jwtv5 "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type Claims struct {
	UserID uuid.UUID `json:"user_id"`
	Role   string    `json:"role"`
	jwtv5.RegisteredClaims
}

func LoadPrivateKey(path string) (*rsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read private key: %w", err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("failed to decode PEM block")
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		pk, err2 := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err2 != nil {
			return nil, fmt.Errorf("parse private key as PKCS1 or PKCS8: %v; %w", err, err2)
		}
		rsaKey, ok := pk.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("not an RSA private key")
		}
		key = rsaKey
	}
	if key.N.BitLen() < 2048 {
		return nil, errors.New("RSA private key must be at least 2048 bits")
	}
	return key, nil
}

func LoadPublicKey(path string) (*rsa.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read public key: %w", err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("failed to decode PEM block")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("not an RSA public key")
	}
	if rsaPub.N.BitLen() < 2048 {
		return nil, errors.New("RSA public key must be at least 2048 bits")
	}
	return rsaPub, nil
}

func GenerateToken(privateKey *rsa.PrivateKey, userID uuid.UUID, role string, ttl time.Duration) (string, error) {
	if privateKey == nil {
		return "", errors.New("private key is required")
	}
	if userID == uuid.Nil {
		return "", errors.New("user id is required")
	}
	if role != "admin" && role != "user" {
		return "", errors.New("invalid role")
	}
	if ttl <= 0 {
		return "", errors.New("token ttl must be positive")
	}
	now := time.Now().UTC()
	claims := Claims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwtv5.RegisteredClaims{
			ExpiresAt: jwtv5.NewNumericDate(now.Add(ttl)),
			IssuedAt:  jwtv5.NewNumericDate(now),
			Issuer:    "room-booking",
		},
	}
	token := jwtv5.NewWithClaims(jwtv5.SigningMethodRS256, claims)
	return token.SignedString(privateKey)
}

func ValidateToken(publicKey *rsa.PublicKey, tokenStr string) (*Claims, error) {
	if publicKey == nil {
		return nil, errors.New("public key is required")
	}
	token, err := jwtv5.ParseWithClaims(tokenStr, &Claims{}, func(_ *jwtv5.Token) (interface{}, error) {
		return publicKey, nil
	}, jwtv5.WithValidMethods([]string{jwtv5.SigningMethodRS256.Alg()}), jwtv5.WithIssuer("room-booking"))
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	if claims.UserID == uuid.Nil || (claims.Role != "admin" && claims.Role != "user") {
		return nil, errors.New("invalid authorization claims")
	}
	return claims, nil
}
