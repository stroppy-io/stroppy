package token

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/oklog/ulid/v2"
)

type Claims struct {
	jwt.RegisteredClaims
	UserID     string                 `json:"user_id"`
	Additional map[string]interface{} `json:"additional"`
}

type Actor struct {
	cfg *Config
	key []byte
}

func NewTokenActor(config *Config) *Actor {
	return &Actor{cfg: config, key: []byte(config.HmacSecretKey)}
}

func (s *Actor) ValidateToken(tokenStr string) (*Claims, error) {
	withClaims, err := jwt.ParseWithClaims(
		tokenStr,
		&Claims{},
		func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return s.key, nil
		},
		jwt.WithLeeway(s.cfg.LeewaySeconds),
		jwt.WithIssuer(s.cfg.Issuer),
		jwt.WithAudience(s.cfg.Issuer),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, err
	}
	claims, ok := withClaims.Claims.(*Claims)
	if !withClaims.Valid || !ok {
		return nil, jwt.ErrTokenUnverifiable
	}
	return claims, nil
}

func (s *Actor) createToken(claims jwt.Claims, opts ...jwt.TokenOption) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims, opts...)
	signedString, err := token.SignedString(s.key)
	if err != nil {
		return "", err
	}

	return signedString, nil
}

func (s *Actor) claims(userID string, additional map[string]interface{}, expire time.Duration) *Claims {
	return &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			Issuer:    s.cfg.Issuer,
			Audience:  jwt.ClaimStrings{s.cfg.Issuer},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(expire)),
		},
		UserID:     userID,
		Additional: additional,
	}
}

func (s *Actor) NewAccessToken(userID string, additional map[string]interface{}) (string, error) {
	if additional == nil {
		additional = make(map[string]interface{})
	}
	accessClaims := s.claims(userID, additional, s.cfg.AccessExpire)
	accessToken, err := s.createToken(accessClaims)
	if err != nil {
		return "", err
	}

	return accessToken, nil
}

func NewRandomToken(length int) (string, error) {
	const charset = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	code := make([]byte, length)
	for i := range code {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		code[i] = charset[num.Int64()]
	}
	return ulid.Make().String() + string(code), nil
}
