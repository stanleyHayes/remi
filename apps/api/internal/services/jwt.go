package services

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type JWTService struct {
	secret []byte
}

type Claims struct {
	UserID  string `json:"sub"`
	Email   string `json:"email"`
	Name    string `json:"name"`
	Role    string `json:"role"`
	Purpose string `json:"purpose"`
	jwt.RegisteredClaims
}

func NewJWTService(secret string) *JWTService {
	return &JWTService{secret: []byte(secret)}
}

// Generate issues a 24h HS256 token for the user.
func (s *JWTService) Generate(userID, email, name, role string) (string, error) {
	now := time.Now().UTC()
	claims := Claims{
		UserID:  userID,
		Email:   email,
		Name:    name,
		Role:    role,
		Purpose: "session",
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(24 * time.Hour)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
}

func (s *JWTService) GenerateMFAChallenge(userID string) (string, error) {
	now := time.Now().UTC()
	claims := Claims{UserID: userID, Purpose: "mfa-challenge", RegisteredClaims: jwt.RegisteredClaims{IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(5 * time.Minute))}}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
}

func (s *JWTService) Validate(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return s.secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	if claims.Purpose != "" && claims.Purpose != "session" {
		return nil, errors.New("invalid token purpose")
	}
	return claims, nil
}

func (s *JWTService) ValidateMFAChallenge(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return s.secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid || claims.Purpose != "mfa-challenge" {
		return nil, errors.New("invalid challenge")
	}
	return claims, nil
}
