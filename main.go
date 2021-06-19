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

const PLOTS_PER_PLAYER = 6

type server struct {
	db *sql.DB
}

type plot struct {
	ID         int64
	Content    int64
	Transition int64
}

type player struct {
	ID    int64
	Name  string
	Plots map[int64]plot
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
	if _, err := s.db.Exec(
		"CREATE TABLE IF NOT EXISTS plots(" +
			"id integer," +
			"player_id integer REFERENCES players(id)," +
			"content integer NOT NULL DEFAULT(0)," +
			"transition integer NOT NULL DEFAULT(0)," +
			"PRIMARY KEY(id, player_id)" +
			")",
	); err != nil {
		return err
	}
	return nil
}

func (s *server) getPlayers(_ []byte, _ httprouter.Params) (interface{}, error) {
	res := map[int64]player{}
	if err := func() error {
		rows, err := s.db.Query("SELECT id, name FROM players")
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var p player
			if err := rows.Scan(&p.ID, &p.Name); err != nil {
				return err
			}

			p.Plots = map[int64]plot{}
			res[p.ID] = p
		}
		return rows.Err()
	}(); err != nil {
		return nil, err
	}

	rows, err := s.db.Query(
		" SELECT players.id, plots.id, content, transition" +
			" FROM players JOIN plots" +
			" on players.id = plots.player_id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var playerID int64
		var p plot
		if err := rows.Scan(&playerID, &p.ID, &p.Content, &p.Transition); err != nil {
			return nil, err
		}

		res[playerID].Plots[p.ID] = p
	}
	return res, rows.Err()
}

func (s *server) addPlayer(body []byte, _ httprouter.Params) (interface{}, error) {
	var req struct {
		Name string
		Auth string
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}

	var id int64

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	query := "INSERT INTO players(name, auth) VALUES($1, $2) returning id"
	if err := tx.QueryRow(query, req.Name, req.Auth).Scan(&id); err != nil {
		return nil, err
	}

	add_plot, err := tx.Prepare("INSERT INTO plots(id, player_id) VALUES($1, $2)")
	if err != nil {
		return nil, err
	}
	for i := 0; i < PLOTS_PER_PLAYER; i++ {
		if _, err := add_plot.Exec(i, id); err != nil {
			return nil, err
		}
	}

	tx.Commit()
	log.Printf("Added player %d: %s", id, req.Name)

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
