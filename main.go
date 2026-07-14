package main

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
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/zub-bee/first_server/internal/auth"
	"github.com/zub-bee/first_server/internal/database"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	database       *database.Queries
}

type userPayload struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func middlewareLog(next http.Handler) http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})

}

func (cfg *apiConfig) serveHits(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, err := w.Write([]byte(fmt.Sprintf(`
<html>
  <body>
    <h1>Welcome, Chirpy Admin</h1>
    <p>Chirpy has been visited %d times!</p>
  </body>
</html>`, cfg.fileserverHits.Load())))
	if err != nil {
		return
	}
}

func (cfg *apiConfig) reset(w http.ResponseWriter, r *http.Request) {
	if os.Getenv("PLATFORM") != "dev" {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	err := cfg.database.DeleteAllUsers(r.Context())
	if err != nil {
		log.Printf("Error deleting all users: %s", err)
		return
	}
	cfg.fileserverHits.Store(0)
	_, err = w.Write([]byte(fmt.Sprintln("Hits and users in database have been reset")))
	if err != nil {
		log.Printf("Error writing response: %s", err)
		return
	}
}

func validate(w http.ResponseWriter, r *http.Request) {
	type chirp struct {
		Body string `json:"body"`
	}

	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	chirpval := chirp{}
	err := decoder.Decode(&chirpval)
	if err != nil {
		log.Printf("Error decoding chirp body: %s", err)
		w.WriteHeader(500)
		return
	}

	if len(chirpval.Body) > 140 {
		err := respondWithJSON(w, 400, map[string]string{"Error": "Chirp is too long!"})
		if err != nil {
			log.Printf("Error writing response: %s", err)
			return
		}
		return
	}

	chirpval.Body = replaceInvalidStrings(chirpval.Body, "kerfuffle", "sharbert", "fornax")

	err = respondWithJSON(w, 200, map[string]string{"cleaned_body": chirpval.Body})
	if err != nil {
		log.Printf("Error writing response: %s", err)
		return
	}

}

func (cfg *apiConfig) createUser(w http.ResponseWriter, r *http.Request) {

	decoder := json.NewDecoder(r.Body)
	defer r.Body.Close()
	var user userPayload
	err := decoder.Decode(&user)
	if err != nil {
		log.Printf("Error decoding request body email: %s", err)
		w.WriteHeader(500)
		return
	}

	var userDB database.CreateUserParams
	userDB.Email = user.Email

	userDB.HashedPassword, err = auth.HashPassword(user.Password)
	if err != nil {
		fmt.Printf("Error hashing password: %s", err)
		w.WriteHeader(500)
		return
	}

	useresp, err := cfg.database.CreateUser(r.Context(), userDB)

	if err != nil {
		log.Printf("Error creating user: %s", err)
		w.WriteHeader(500)
	}

	newUser := struct {
		ID             uuid.UUID `json:"id"`
		CreatedAt      time.Time `json:"created_at"`
		UpdatedAt      time.Time `json:"updated_at"`
		Email          string    `json:"email"`
	}{
		ID:        useresp.ID,
		CreatedAt: useresp.CreatedAt,
		UpdatedAt: useresp.UpdatedAt,
		Email:     useresp.Email,
	}

	err = respondWithJSON(w, 201, newUser)
	if err != nil {
		log.Printf("Error writing response: %s", err)
		return
	}
}

func (cfg *apiConfig) loginUser(w http.ResponseWriter, r *http.Request) {

	decoder := json.NewDecoder(r.Body)
	defer r.Body.Close()
	var userLogin userPayload
	err := decoder.Decode(&userLogin)
	if err != nil {
		log.Printf("Error decoding request body email: %s", err)
		w.WriteHeader(500)
		return
	}

	if err != nil {
		fmt.Printf("Error hashing password: %s", err)
		w.WriteHeader(500)
		return
	}

	user, err := cfg.database.GetUserByEmail(r.Context(), userLogin.Email)

	if err != nil {
		if err.Error() == "ERR_NO_ROWS" {
			err = respondWithJSON(w, 401, map[string]string{"error": "Incorrect email or password"})
			if err != nil {
				log.Printf("Error writing response: %s", err)
				return
			}
			log.Printf("User not found: %s", userLogin.Email)
			return
		}

		log.Printf("Error getting user by email: %s", err)
		w.WriteHeader(500)
		return

	}

	if ok, err := auth.CheckPasswordHash(userLogin.Password, user.HashedPassword); err != nil || !ok {
		err = respondWithJSON(w, 401, map[string]string{"error": "Incorrect email or password"})
		if err != nil {
			log.Printf("Error writing response: %s", err)
			return
		}
		log.Printf("Invalid password for user: %s", userLogin.Email)
		w.WriteHeader(401)
		return
	}

	userRes := struct {
		ID        uuid.UUID `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Email     string    `json:"email"`
	}{
		ID:        user.ID,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
		Email:     user.Email,
	}

	err = respondWithJSON(w, 200, userRes)
	if err != nil {
		log.Printf("Error writing response: %s", err)
		return
	}

}

func (cfg *apiConfig) createChirp(w http.ResponseWriter, r *http.Request) {

	chirp := database.CreateChirpParams{}
	decoder := json.NewDecoder(r.Body)
	defer r.Body.Close()
	err := decoder.Decode(&chirp)
	if err != nil {
		log.Printf("Error decoding request body: %s", err)
		w.WriteHeader(500)
		return
	}

	// update db
	newChirp, err := cfg.database.CreateChirp(r.Context(), chirp)
	if err != nil {
		log.Printf("Error creating chirp: %s", err)
		w.WriteHeader(500)
		return
	}

	err = respondWithJSON(w, 201, newChirp)
	if err != nil {
		log.Printf("Error writing response: %s", err)
		return
	}
}

func (cfg *apiConfig) getAllChirps(w http.ResponseWriter, r *http.Request) {

	log.Printf("Getting all chirps")
	// get from db
	chirps, err := cfg.database.GetAllChirps(r.Context())
	if err != nil {
		log.Printf("Error creating chirp: %s", err)
		w.WriteHeader(500)
		return
	}

	err = respondWithJSON(w, 200, chirps)
	if err != nil {
		log.Printf("Error writing response: %s", err)
		return
	}

}

func (cfg *apiConfig) getChirpById(w http.ResponseWriter, r *http.Request) {

	id := r.PathValue("chirpID")
	log.Printf("ID: %s", id)

	if id == "" {

		log.Printf("Invalid ID")
		w.WriteHeader(400)
		return

	}

	parsedUUID, err := uuid.Parse(id)
	log.Printf("Parsed UUID: %s", parsedUUID)
	if err != nil {
		log.Printf("Invalid UUID string: %v", err)
		return
	}

	log.Printf("Getting chirp with id: %s", id)
	chirp, err := cfg.database.GetOneChirp(r.Context(), parsedUUID)
	if fmt.Sprintf("%s", err) == "sql: no rows in result set" {
		log.Printf("Chirp not found: %s", err)
		w.WriteHeader(404)
		return
	} else if err != nil {
		log.Printf("Error getting chirp: %s", err)
		w.WriteHeader(500)
		return
	}

	err = respondWithJSON(w, 200, chirp)
	log.Printf("Chirp found")
	if err != nil {
		log.Printf("Error writing response: %s", err)
		return
	}
}

func replaceInvalidStrings(s string, invalidStrs ...string) string {
	stringArr := strings.Fields(s)
	for i, word := range stringArr {

		word = strings.ToLower(word)
		for _, invalidStr := range invalidStrs {
			if word == invalidStr {
				stringArr[i] = "****"
			}
		}
	}

	s = strings.Join(stringArr, " ")
	return s
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

func main() {
	// database setup
	godotenv.Load()
	dbURL := os.Getenv("DB_URL")
	port := os.Getenv("PORT")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal(err)
	}

	dbQueries := database.New(db)
	apiCfg := &apiConfig{database: dbQueries}
	mux := http.NewServeMux()

	// file server
	mux.Handle("/app/", apiCfg.middlewareMetricsInc(middlewareLog(http.StripPrefix("/app/", http.FileServer(http.Dir("."))))))

	// new handler function
	mux.HandleFunc("GET /api/healthz", func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
		writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, err := writer.Write([]byte(fmt.Sprintln("OK")))
		if err != nil {
			return
		}
	})

	// hits
	mux.HandleFunc("GET /admin/metrics", apiCfg.serveHits)
	mux.HandleFunc("/admin/reset", apiCfg.reset)

	// validate chirp
	mux.HandleFunc("POST /api/validate_chirp", validate)

	// create user
	mux.HandleFunc("POST /api/users", apiCfg.createUser)

	// login
	mux.HandleFunc("POST /api/login", apiCfg.loginUser)

	// create chirp
	mux.HandleFunc("POST /api/chirps", apiCfg.createChirp)

	// get all chirps
	mux.HandleFunc("GET /api/chirps", apiCfg.getAllChirps)

	// get chirps by id
	mux.HandleFunc("GET /api/chirps/{chirpID}", apiCfg.getChirpById)

	server := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	err = server.ListenAndServe()
	if err != nil {
		return
	}
	fmt.Printf(`Server has started on http:\\localhost:%s`, port)
}
