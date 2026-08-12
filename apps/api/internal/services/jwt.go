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
	UserID              string   `json:"sub"`
	Email               string   `json:"email"`
	Name                string   `json:"name"`
	Role                string   `json:"role"`
	Purpose             string   `json:"purpose"`
	PersonID            string   `json:"personId,omitempty"`
	SessionID           string   `json:"sessionId,omitempty"`
	HouseholdID         string   `json:"householdId,omitempty"`
	MFAAt               int64    `json:"mfaAt,omitempty"`
	BranchIDs           []string `json:"branchIds,omitempty"`
	MinistryIDs         []string `json:"ministryIds,omitempty"`
	AssignedResourceIDs []string `json:"assignedResourceIds,omitempty"`
	AccessVersion       *int64   `json:"accessVersion,omitempty"`
	jwt.RegisteredClaims
}

// GenerateMember issues a short-lived member access token. The opaque refresh
// token and device lifecycle are persisted separately so a single device can
// be revoked without terminating every member session.
func (s *JWTService) GenerateMember(accountID, personID, sessionID, householdID, email, name string) (string, error) {
	now := time.Now().UTC()
	claims := Claims{
		UserID: accountID, PersonID: personID, SessionID: sessionID,
		HouseholdID: householdID, Email: email, Name: name, Role: "member", Purpose: "session",
		RegisteredClaims: jwt.RegisteredClaims{IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute))},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
}

func NewJWTService(secret string) *JWTService {
	return &JWTService{secret: []byte(secret)}
}

// Generate issues a 24h HS256 token for the user.
func (s *JWTService) Generate(userID, email, name, role string) (string, error) {
	return s.generateStaff(userID, email, name, role, 0, []string{"*"}, []string{"*"}, nil)
}

func (s *JWTService) GenerateScoped(userID, email, name, role string, branchIDs, ministryIDs, assignedResourceIDs []string, accessVersion int64) (string, error) {
	return s.generateStaffVersioned(userID, email, name, role, 0, branchIDs, ministryIDs, assignedResourceIDs, &accessVersion)
}

func (s *JWTService) GenerateMFAAuthenticated(userID, email, name, role string, confirmedAt time.Time) (string, error) {
	return s.generateStaff(userID, email, name, role, confirmedAt.UTC().Unix(), []string{"*"}, []string{"*"}, nil)
}

func (s *JWTService) GenerateMFAAuthenticatedScoped(userID, email, name, role string, confirmedAt time.Time, branchIDs, ministryIDs, assignedResourceIDs []string, accessVersion int64) (string, error) {
	return s.generateStaffVersioned(userID, email, name, role, confirmedAt.UTC().Unix(), branchIDs, ministryIDs, assignedResourceIDs, &accessVersion)
}

func (s *JWTService) generateStaff(userID, email, name, role string, mfaAt int64, branchIDs, ministryIDs, assignedResourceIDs []string) (string, error) {
	return s.generateStaffVersioned(userID, email, name, role, mfaAt, branchIDs, ministryIDs, assignedResourceIDs, nil)
}

func (s *JWTService) generateStaffVersioned(userID, email, name, role string, mfaAt int64, branchIDs, ministryIDs, assignedResourceIDs []string, accessVersion *int64) (string, error) {
	now := time.Now().UTC()
	claims := Claims{
		UserID:              userID,
		Email:               email,
		Name:                name,
		Role:                role,
		Purpose:             "session",
		MFAAt:               mfaAt,
		BranchIDs:           append([]string(nil), branchIDs...),
		MinistryIDs:         append([]string(nil), ministryIDs...),
		AssignedResourceIDs: append([]string(nil), assignedResourceIDs...),
		AccessVersion:       accessVersion,
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
