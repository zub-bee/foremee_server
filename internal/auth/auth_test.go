package auth

import "testing"

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