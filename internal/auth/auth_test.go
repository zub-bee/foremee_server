package auth

import (
	"log"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestHashPassword(t *testing.T) {
	password := "mysecretpassword"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword returned an error: %v", err)
	}

	if hash == "" {
		t.Fatal("HashPassword returned an empty hash")
	}
}

func TestCheckPasswordHash(t *testing.T) {
	password := "mysecretpassword"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword returned an error: %v", err)
	}

	match, err := CheckPasswordHash(password, hash)
	if err != nil {
		t.Fatalf("CheckPasswordHash returned an error: %v", err)
	}

	if !match {
		t.Fatal("CheckPasswordHash returned false for a valid password and hash")
	}

	wrongPassword := "wrongpassword"
	match, err = CheckPasswordHash(wrongPassword, hash)
	if err != nil {
		t.Fatalf("CheckPasswordHash returned an error: %v", err)
	}

	if match {
		t.Fatal("CheckPasswordHash returned true for an invalid password and hash")
	}
}

// create JWTs
func TestJWT(t *testing.T) {

	userId := uuid.MustParse("3d3aa131-be0c-42ec-af77-2eebfba1ae49")
	secret := "d2983dbb73e45b4b9f11ff215ecdb28e5a45729aaf27033e08ba06b42812f858"
	exp := time.Duration(1784213536)
	

	t.Run("create JWTs", func(t *testing.T) {
		
		jwt, err := MakeJWT(userId, secret, exp)

		if err != nil {
			t.Fatalf("TestMakeJWT returned an error %s", err)
		}

		if jwt == "" {
			t.Fatalf("TestMakeJWT returned no jwt")
		}

	})

	t.Run("validate JWTs", func(t *testing.T) {

		jwt, err := MakeJWT(userId, secret, exp)

		id, err := ValidateJWT(jwt, secret)

		if err != nil {
			log.Fatalf("validate JWTs: returns an error: %s", err)
		}

		if id != userId {
			log.Fatalf("validate JWTs: returns wrong id, want %s, got %s", userId, id)
		}

	})

	t.Run("expired tokens are rejected", func(t *testing.T) {

		jwt, _ := MakeJWT(userId, secret, -time.Second)

		_, err := ValidateJWT(jwt, secret)

		if err == nil {
			t.Fatal("expected error for expired token, got nil, want expired token error")
		}

	})

	t.Run("wrong secret is rejected", func(t *testing.T) {

		jwt, _ := MakeJWT(userId, secret, time.Hour)
		_, err := ValidateJWT(jwt, "wrong-secret")
		if err == nil {
			t.Fatal("expected error for wrong secret, got nil")
		}

	})
	
}
	

// validate JWTs
// expired tokens are rejected 
// JWTs signed with the wrong secret are rejected
