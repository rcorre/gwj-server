package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	_ "github.com/lib/pq"
)

type db struct {
	*sql.DB
}

func (d *db) Init() error {
	if _, err := d.Exec(
		"CREATE TABLE IF NOT EXISTS users(" +
			"id serial PRIMARY KEY," +
			"name varchar NOT NULL UNIQUE" +
			")",
	); err != nil {
		return err
	}
	return nil
}

func (d *db) GetUsers() ([]string, error) {
	rows, err := d.Query("SELECT name from users")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	res := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return res, err
		}

		res = append(res, name)
	}
	return res, rows.Err()
}

func (d *db) AddUser(name string) error {
	stmt, err := d.Prepare("INSERT INTO users(name) VALUES($1)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(name)
	return err
}

type v1API struct {
	db db
}

func unmarshal(r io.Reader, out interface{}) error {
	b, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return fmt.Errorf("Failed to unmarshal %q: %v", b, err)
	}
	return nil
}

/*
	if !strings.HasPrefix(auth, "Bearer ") {
		return nil, fmt.Errorf("Missing bearer in %q", auth)
	}
*/

func (v1 *v1API) getUsers(w http.ResponseWriter, r *http.Request) ([]string, error) {
	return v1.db.GetUsers()
}

func (v1 *v1API) putUser(w http.ResponseWriter, r *http.Request) (interface{}, error) {
	err := v1.db.AddUser("foo")
	return nil, err
}

/*
func (v1 *v1API) login(name, auth string) ([]string, error) {
	user, err := v1.getItchUser(r.Header.Get("Authorization"))
	if err != nil {
		log.Printf("Failed to lookup user: %v", err)
		http.Error(w, "", http.StatusUnauthorized)
		return
	}

	if err := v1.db.PutUser(user); err != nil {
		log.Printf("Failed to store user: %v", err)
		http.Error(w, "Failed to store user", http.StatusInternalServerError)
		return
	}

	var entry record
	if err := unmarshal(r.Body, &entry); err != nil {
		log.Printf("Failed to parse POST body: %v", err)
		http.Error(w, "", http.StatusBadRequest)
		return
	}
	if entry.Level == 0 || entry.Time <= 0 {
		log.Println("Missing time or level")
		http.Error(w, "", http.StatusBadRequest)
		return
	}

	entry.ItchID = user.ID
	if err := v1.db.PutRecord(entry); err != nil {
		log.Printf("Failed to store record: %v", err)
		http.Error(w, "Failed to store record", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	log.Println("POST record ok", entry.UserName, entry.Level, entry.Time)
}
*/

func newMux(db db) *http.ServeMux {
	mux := http.NewServeMux()
	v1 := &v1API{db: db}

	mux.HandleFunc("/v1/users", func(w http.ResponseWriter, r *http.Request) {
		var result interface{}
		var err error
		if r.Method == http.MethodGet {
			result, err = v1.getUsers(w, r)
		} else if r.Method == http.MethodPut {
			result, err = v1.putUser(w, r)
		} else {
			http.Error(w, "", http.StatusMethodNotAllowed)
		}

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		} else if result != nil {
			if body, err := json.Marshal(result); err != nil {
				http.Error(w, "Failed to marshal response", http.StatusInternalServerError)
			} else {
				w.Write(body)
			}
		} else {
			w.WriteHeader(200)
		}
	})

	return mux
}

func main() {
	port := os.Getenv("PORT")

	if port == "" {
		port = "8080"
	}

	pg, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		panic(err)
	}
	log.Println("Connected to DB")

	d := &db{pg}
	if err := d.Init(); err != nil {
		panic(err)
	}
	log.Println("DB Initialized")

	server := &http.Server{
		Handler: newMux(db{pg}),
		Addr:    ":" + port,
	}
	log.Println("Listening on", port)
	log.Fatal(server.ListenAndServe())
}
