package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"github.com/julienschmidt/httprouter"

	_ "github.com/lib/pq"
)

type server struct {
	db *sql.DB
}

type player struct {
	ID   int64
	Name string
}

func (s *server) createTables() error {
	if _, err := s.db.Exec(
		"CREATE TABLE IF NOT EXISTS players(" +
			"id serial PRIMARY KEY," +
			"name varchar NOT NULL UNIQUE," +
			"auth varchar NOT NULL" +
			")",
	); err != nil {
		return err
	}
	return nil
}

func (s *server) getPlayers(_ []byte, _ httprouter.Params) (interface{}, error) {
	rows, err := s.db.Query("SELECT id, name from players")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	res := []player{}
	for rows.Next() {
		var p player
		if err := rows.Scan(&p.ID, &p.Name); err != nil {
			return res, err
		}

		res = append(res, p)
	}
	return res, rows.Err()
}

func (s *server) addPlayer(body []byte, _ httprouter.Params) (interface{}, error) {
	var req struct {
		Name string
		Auth string
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return player{}, err
	}

	var id int64
	query := "INSERT INTO players(name, auth) VALUES($1, $2) returning id"
	if err := s.db.QueryRow(query, req.Name, req.Auth).Scan(&id); err != nil {
		return player{}, err
	}

	return player{
		ID:   id,
		Name: req.Name,
	}, nil
}

func handle(handler func(body []byte, params httprouter.Params) (interface{}, error)) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		if b, err := ioutil.ReadAll(r.Body); err != nil {
			http.Error(w, fmt.Sprintf(`"%v"`, err), http.StatusBadRequest)
		} else if result, err := handler(b, p); err != nil {
			http.Error(w, fmt.Sprintf(`"%v"`, err), http.StatusInternalServerError)
		} else if result != nil {
			if body, err := json.Marshal(result); err != nil {
				http.Error(w, "Failed to marshal response", http.StatusInternalServerError)
			} else {
				w.Write(body)
			}
		} else {
			w.WriteHeader(200)
		}
	}
}

func newServer() http.Handler {
	dbURL, ok := os.LookupEnv("DATABASE_URL")
	if !ok {
		panic("DATABASE_URL not set")
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		panic(err)
	}
	log.Println("Connected to DB")

	s := &server{db: db}
	s.createTables()

	router := httprouter.New()
	router.GET("/players", handle(s.getPlayers))
	router.POST("/players", handle(s.addPlayer))

	return router
}

func main() {
	port := os.Getenv("PORT")

	if port == "" {
		port = "8080"
	}

	server := &http.Server{
		Handler: newServer(),
		Addr:    ":" + port,
	}
	log.Println("Listening on", port)
	log.Fatal(server.ListenAndServe())
}
