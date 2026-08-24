package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/zub-bee/first_server/internal/database"
	"github.com/zub-bee/first_server/internal/server"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Printf(".env not loaded: %v", err)
	}

	dbURL := os.Getenv("DB_URL")
	port := os.Getenv("PORT")

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal(err)
	}

	app := server.New(database.New(db))
	mux := http.NewServeMux()
	app.RegisterRoutes(mux)

	httpServer := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	log.Printf("Server listening on http://localhost:%s", port)
	if err := httpServer.ListenAndServe(); err != nil {
		return
	}
}
