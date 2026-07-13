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

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/zub-bee/first_server/internal/database"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	database       *database.Queries
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
	var user database.User
	err := decoder.Decode(&user)
	if err != nil {
		log.Printf("Error decoding request body email: %s", err)
		w.WriteHeader(500)
		return
	}
	useresp, err := cfg.database.CreateUser(r.Context(), user.Email)

	if err != nil {
		log.Printf("Error creating user: %s", err)
		w.WriteHeader(500)
	}

	err = respondWithJSON(w, 201, useresp)
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

	// update db
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

	// create chirp
	mux.HandleFunc("POST /api/chirps", apiCfg.createChirp)

	// get all chirps
	mux.HandleFunc("GET /api/chirps", apiCfg.getAllChirps)

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
