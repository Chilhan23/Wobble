package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var (
	ErrInvalidToken = errors.New("invalid or malformed token")
	ErrExpiredToken = errors.New("token has expired")
)

type TicketTokenPayload struct {
	PublicID uuid.UUID
	TenantID int64
	UserID   string
	TicketID int64
}

type TicketClaims struct {
	TenantID int64  `json:"tid"`
	UserID   string `json:"uid"`
	TicketID int64  `json:"id"`
	jwt.RegisteredClaims
}

func (c *TicketClaims) PublicID() (uuid.UUID, error) {
	return uuid.Parse(c.Subject)
}

type TokenService interface {
	Issue(payload TicketTokenPayload) (tokenString string, expiresAt time.Time, err error)
	Verify(tokenString string) (*TicketClaims, error)
}

type tokenService struct {
	secret []byte
	ttl    time.Duration
}

func NewTokenService(secret string, ttl time.Duration) TokenService {
	return &tokenService{
		secret: []byte(secret),
		ttl:    ttl,
	}
}

func (s *tokenService) Issue(payload TicketTokenPayload) (string, time.Time, error) {
	expiresAt := time.Now().Add(s.ttl)

	claims := TicketClaims{
		TenantID: payload.TenantID,
		UserID:   payload.UserID,
		TicketID: payload.TicketID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   payload.PublicID.String(),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(s.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to sign ticket token: %w", err)
	}

	return tokenString, expiresAt, nil
}

func (s *tokenService) Verify(tokenString string) (*TicketClaims, error) {
	var claims TicketClaims

	token, err := jwt.ParseWithClaims(tokenString, &claims, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.secret, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	if !token.Valid {
		return nil, ErrInvalidToken
	}

	return &claims, nil
}
