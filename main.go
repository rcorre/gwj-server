package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/julienschmidt/httprouter"

	_ "github.com/lib/pq"
)

const PLOTS_PER_PLAYER = 6

const (
	ITEM_NONE = iota
	ITEM_CARROT_SEED
)

// for mocking time in tests
type clock interface {
	Now() time.Time
}

type realClock struct{}

func (_ *realClock) Now() time.Time { return time.Now() }

type server struct {
	db    *sql.DB
	clock clock
}

type plot struct {
	ID         int64
	Item       int64
	Transition int64
}

type player struct {
	ID    int64
	Name  string
	Plots map[int64]plot
}

func (s *server) setTransitionTime(p *plot) error {
	now := s.clock.Now()
	switch p.Item {
	case ITEM_CARROT_SEED:
		p.Transition = now.Add(time.Second * 10).Unix()
	case ITEM_NONE:
		p.Transition = 0
	default:
		return fmt.Errorf("Unknown item %v", p.Item)
	}
	return nil
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
			"item integer NOT NULL DEFAULT(0)," +
			"transition integer NOT NULL DEFAULT(0)," +
			"PRIMARY KEY(id, player_id)" +
			")",
	); err != nil {
		return err
	}
	return nil
}

func getPlayerPlotID(p httprouter.Params) (int64, int64, error) {
	playerID, err := strconv.ParseInt(p.ByName("player_id"), 10, 64)
	if err != nil {
		return 0, 0, err
	}

	plotID, err := strconv.ParseInt(p.ByName("plot_id"), 10, 64)
	return playerID, plotID, err
}

func (s *server) getPlot(_ []byte, p httprouter.Params) (interface{}, error) {
	playerID, plotID, err := getPlayerPlotID(p)
	if err != nil {
		return nil, err
	}

	res := plot{ID: plotID}

	if err := s.db.QueryRow(
		" SELECT item, transition"+
			" FROM plots"+
			" WHERE player_id = $1 "+
			" AND id = $2 ",
		playerID,
		plotID,
	).Scan(&res.Item, &res.Transition); err != nil {
		return nil, err
	}
	return res, nil
}

func (s *server) putPlot(req []byte, p httprouter.Params) (interface{}, error) {
	playerID, plotID, err := getPlayerPlotID(p)
	if err != nil {
		return nil, err
	}

	var newPlot plot
	if err := json.Unmarshal(req, &newPlot); err != nil {
		return nil, err
	}

	if err := s.setTransitionTime(&newPlot); err != nil {
		return nil, err
	}

	if err := s.db.QueryRow(
		"UPDATE plots"+
			" SET item = $1, transition = $2"+
			" WHERE player_id = $3 "+
			" AND id = $4 ",
		newPlot.Item,
		newPlot.Transition,
		playerID,
		plotID,
	).Err(); err != nil {
		return nil, err
	}
	return newPlot, nil
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
		" SELECT players.id, plots.id, item, transition" +
			" FROM players JOIN plots" +
			" on players.id = plots.player_id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var playerID int64
		var p plot
		if err := rows.Scan(&playerID, &p.ID, &p.Item, &p.Transition); err != nil {
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

func newServer(c clock) http.Handler {
	dbURL, ok := os.LookupEnv("DATABASE_URL")
	if !ok {
		panic("DATABASE_URL not set")
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		panic(err)
	}
	log.Println("Connected to DB")

	s := &server{db: db, clock: c}
	s.createTables()

	router := httprouter.New()
	router.GET("/players", handle(s.getPlayers))
	router.GET("/players/:player_id/plots/:plot_id", handle(s.getPlot))
	router.PUT("/players/:player_id/plots/:plot_id", handle(s.putPlot))
	router.POST("/players", handle(s.addPlayer))

	return router
}

func main() {
	port := os.Getenv("PORT")

	if port == "" {
		port = "8080"
	}

	server := &http.Server{
		Handler: newServer(&realClock{}),
		Addr:    ":" + port,
	}
	log.Println("Listening on", port)
	log.Fatal(server.ListenAndServe())
}
