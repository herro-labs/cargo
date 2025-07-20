package cargo

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// generateJWT creates a new JWT token from auth identity
func generateJWT(identity *AuthIdentity) (string, error) {
	expire := time.Now().Add(time.Hour * 24) // 24 hour expiration

	claims := &JWTClaims{
		ID:     identity.ID,
		Claims: identity.Claims,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expire),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

// validateJWT validates a JWT token and returns the claims
func validateJWT(tokenString string) (*JWTClaims, error) {
	claims := &JWTClaims{}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})

	if err != nil || !token.Valid {
		return nil, err
	}

	return claims, nil
}
