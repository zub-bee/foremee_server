package auth

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/alexedwards/argon2id"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func HashPassword(password string) (string, error) {
	return argon2id.CreateHash(password, argon2id.DefaultParams)
}

func CheckPasswordHash(password, hash string) (bool, error) {
	return argon2id.ComparePasswordAndHash(password, hash)
}

func MakeJWT(userID uuid.UUID, tokenSecret string, expiresIn time.Duration) (string, error) {
	claim := jwt.RegisteredClaims{
		Issuer:    "chirpy-access",
		Subject:   userID.String(),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiresIn)),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claim)
	tokenString, err := token.SignedString([]byte(tokenSecret))

	return tokenString, err
}

func ValidateJWT(tokenString, tokenSecret string) (uuid.UUID, error) {

	var claims jwt.RegisteredClaims
	token, err := jwt.ParseWithClaims(tokenString, &claims, func(t *jwt.Token) (any, error) {

		if t.Method.Alg() != "HS256" {
			return nil, fmt.Errorf("unexpected algorithm")
		}
		return []byte(tokenSecret), nil
	})

	if err != nil {
		log.Println("invalid jwt")
		return uuid.Nil, err
	}

	userId, err := token.Claims.GetSubject()
	id, err := uuid.Parse(userId)
	if err != nil {
		return uuid.Nil, err
	}

	return id, nil
}

func GetBearerToken(header http.Header) (string, error) {

	bearerToken := header.Get("Authorization")

	if bearerToken != "" {
		jwt, found := strings.CutPrefix(bearerToken, "Bearer ")
		if found == true {
			return jwt, nil
		}
		return "", fmt.Errorf("bearerToken wasn't retrieved")
	}

	return "", fmt.Errorf("bearerToken wasn't retrieved")
}
