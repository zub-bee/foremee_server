package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/zub-bee/first_server/internal/auth"
	"github.com/zub-bee/first_server/internal/database"
)

type Server struct {
	fileserverHits atomic.Int32
	database       *database.Queries
}

type userPayload struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func New(database *database.Queries) *Server {
	return &Server{database: database}
}

func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("/app/", s.withMetrics(s.loggingMiddleware(http.StripPrefix("/app/", http.FileServer(http.Dir("."))))))
	mux.HandleFunc("GET /api/healthz", s.handleHealthCheck)
	mux.HandleFunc("GET /admin/metrics", s.handleMetrics)
	mux.HandleFunc("/admin/reset", s.handleReset)
	mux.HandleFunc("POST /api/validate_chirp", handleValidateChirp)
	mux.HandleFunc("POST /api/users", s.handleCreateUser)
	mux.HandleFunc("POST /api/login", s.handleLoginUser)
	mux.HandleFunc("POST /api/chirps", s.handleCreateChirp)
	mux.HandleFunc("GET /api/chirps", s.handleListChirps)
	mux.HandleFunc("GET /api/chirps/{chirpID}", s.handleGetChirpByID)
}

func (s *Server) withMetrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte(fmt.Sprintln("OK")))
	if err != nil {
		return
	}
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte(fmt.Sprintf(`
<html>
  <body>
    <h1>Welcome, Chirpy Admin</h1>
    <p>Chirpy has been visited %d times!</p>
  </body>
</html>`, s.fileserverHits.Load())))
	if err != nil {
		return
	}
}

func (s *Server) handleReset(w http.ResponseWriter, r *http.Request) {
	if os.Getenv("PLATFORM") != "dev" {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	err := s.database.DeleteAllUsers(r.Context())
	if err != nil {
		log.Printf("Error deleting all users: %s", err)
		return
	}
	s.fileserverHits.Store(0)
	_, err = w.Write([]byte(fmt.Sprintln("Hits and users in database have been reset")))
	if err != nil {
		log.Printf("Error writing response: %s", err)
		return
	}
}

func handleValidateChirp(w http.ResponseWriter, r *http.Request) {
	type chirp struct {
		Body string `json:"body"`
	}

	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	chirpValue := chirp{}
	err := decoder.Decode(&chirpValue)
	if err != nil {
		log.Printf("Error decoding chirp body: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if len(chirpValue.Body) > 140 {
		err := respondWithJSON(w, http.StatusBadRequest, map[string]string{"Error": "Chirp is too long!"})
		if err != nil {
			log.Printf("Error writing response: %s", err)
			return
		}
		return
	}

	chirpValue.Body = replaceInvalidStrings(chirpValue.Body, "kerfuffle", "sharbert", "fornax")

	err = respondWithJSON(w, http.StatusOK, map[string]string{"cleaned_body": chirpValue.Body})
	if err != nil {
		log.Printf("Error writing response: %s", err)
		return
	}
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	defer r.Body.Close()

	var user userPayload
	err := decoder.Decode(&user)
	if err != nil {
		log.Printf("Error decoding request body email: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	userDB := database.CreateUserParams{Email: user.Email}
	userDB.HashedPassword, err = auth.HashPassword(user.Password)
	if err != nil {
		log.Printf("Error hashing password: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	createdUser, err := s.database.CreateUser(r.Context(), userDB)
	if err != nil {
		log.Printf("Error creating user: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	response := struct {
		ID        uuid.UUID `json:"id"`
		CreatedAt time.Time  `json:"created_at"`
		UpdatedAt time.Time  `json:"updated_at"`
		Email     string     `json:"email"`
	}{
		ID:        createdUser.ID,
		CreatedAt: createdUser.CreatedAt,
		UpdatedAt: createdUser.UpdatedAt,
		Email:     createdUser.Email,
	}

	err = respondWithJSON(w, http.StatusCreated, response)
	if err != nil {
		log.Printf("Error writing response: %s", err)
		return
	}
}

func (s *Server) handleLoginUser(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	defer r.Body.Close()

	var userLogin userPayload
	err := decoder.Decode(&userLogin)
	if err != nil {
		log.Printf("Error decoding request body email: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	user, err := s.database.GetUserByEmail(r.Context(), userLogin.Email)
	if err != nil {
		if err == sql.ErrNoRows {
			err = respondWithJSON(w, http.StatusUnauthorized, map[string]string{"error": "Incorrect email or password"})
			if err != nil {
				log.Printf("Error writing response: %s", err)
				return
			}
			log.Printf("User not found: %s", userLogin.Email)
			return
		}

		log.Printf("Error getting user by email: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if ok, err := auth.CheckPasswordHash(userLogin.Password, user.HashedPassword); err != nil || !ok {
		err = respondWithJSON(w, http.StatusUnauthorized, map[string]string{"error": "Incorrect email or password"})
		if err != nil {
			log.Printf("Error writing response: %s", err)
			return
		}
		log.Printf("Invalid password for user: %s", userLogin.Email)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	response := struct {
		ID        uuid.UUID `json:"id"`
		CreatedAt time.Time  `json:"created_at"`
		UpdatedAt time.Time  `json:"updated_at"`
		Email     string     `json:"email"`
	}{
		ID:        user.ID,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
		Email:     user.Email,
	}

	err = respondWithJSON(w, http.StatusOK, response)
	if err != nil {
		log.Printf("Error writing response: %s", err)
		return
	}
}

func (s *Server) handleCreateChirp(w http.ResponseWriter, r *http.Request) {
	chirp := database.CreateChirpParams{}
	decoder := json.NewDecoder(r.Body)
	defer r.Body.Close()
	err := decoder.Decode(&chirp)
	if err != nil {
		log.Printf("Error decoding request body: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	createdChirp, err := s.database.CreateChirp(r.Context(), chirp)
	if err != nil {
		log.Printf("Error creating chirp: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	err = respondWithJSON(w, http.StatusCreated, createdChirp)
	if err != nil {
		log.Printf("Error writing response: %s", err)
		return
	}
}

func (s *Server) handleListChirps(w http.ResponseWriter, r *http.Request) {
	log.Printf("Getting all chirps")
	chirps, err := s.database.GetAllChirps(r.Context())
	if err != nil {
		log.Printf("Error creating chirp: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	err = respondWithJSON(w, http.StatusOK, chirps)
	if err != nil {
		log.Printf("Error writing response: %s", err)
		return
	}
}

func (s *Server) handleGetChirpByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("chirpID")
	log.Printf("ID: %s", id)

	if id == "" {
		log.Printf("Invalid ID")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	parsedUUID, err := uuid.Parse(id)
	log.Printf("Parsed UUID: %s", parsedUUID)
	if err != nil {
		log.Printf("Invalid UUID string: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	log.Printf("Getting chirp with id: %s", id)
	chirp, err := s.database.GetOneChirp(r.Context(), parsedUUID)
	if err == sql.ErrNoRows {
		log.Printf("Chirp not found: %s", err)
		w.WriteHeader(http.StatusNotFound)
		return
	} else if err != nil {
		log.Printf("Error getting chirp: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	err = respondWithJSON(w, http.StatusOK, chirp)
	log.Printf("Chirp found")
	if err != nil {
		log.Printf("Error writing response: %s", err)
		return
	}
}

func replaceInvalidStrings(input string, invalidStrs ...string) string {
	stringArr := strings.Fields(input)
	for i, word := range stringArr {
		word = strings.ToLower(word)
		for _, invalidStr := range invalidStrs {
			if word == invalidStr {
				stringArr[i] = "****"
			}
		}
	}

	return strings.Join(stringArr, " ")
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) error {
	response, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(code)
	_, err = w.Write(response)
	if err != nil {
		return err
	}
	return nil
}