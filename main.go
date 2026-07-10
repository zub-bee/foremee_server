package main

import (
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
)

const port = "8080"

type apiConfig struct {
	fileserverHits atomic.Int32
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
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	cfg.fileserverHits.Store(0)
	_, err := w.Write([]byte(fmt.Sprintln("Hits have been reset")))
	if err != nil {
		return
	}
}

func main() {
	apiCfg := &apiConfig{}
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
	mux.HandleFunc("POST /admin/reset", apiCfg.reset)

	server := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	err := server.ListenAndServe()
	if err != nil {
		return
	}
	fmt.Printf(`Server has started on http:\\localhost:%s`, port)
}
