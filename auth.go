package cargo

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// generateJWT creates a new JWT token from auth identity using app config
func (a *App) generateJWT(identity *AuthIdentity) (string, error) {
	expire := time.Now().Add(a.config.Auth.JWT.ExpiryDuration)

	claims := &JWTClaims{
		ID:     identity.ID,
		Claims: identity.Claims,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expire),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    a.config.Auth.JWT.Issuer,
			Audience:  []string{a.config.Auth.JWT.Audience},
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(a.config.Auth.JWT.Secret))
}

// validateJWT validates a JWT token and returns the claims using app config
func (a *App) validateJWT(tokenString string) (*JWTClaims, error) {
	claims := &JWTClaims{}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return []byte(a.config.Auth.JWT.Secret), nil
	})

	if err != nil || !token.Valid {
		return nil, err
	}

	return claims, nil
}
