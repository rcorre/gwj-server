package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"

	_ "github.com/lib/pq"
)

type db struct {
	*sql.DB
}

func (d *db) Init() error {
	if _, err := d.Exec(
		"CREATE TABLE IF NOT EXISTS users(" +
			"id serial PRIMARY KEY," +
			"name varchar NOT NULL UNIQUE," +
			"auth varchar NOT NULL" +
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

func (d *db) AddUser(name, auth string) error {
	stmt, err := d.Prepare("INSERT INTO users(name, auth) VALUES($1, $2)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(name, auth)
	return err
}

func (d *db) AuthUser(user, auth string) (bool, error) {
	var expected string
	err := d.QueryRow("SELECT auth from users where name = $1", user).Scan(&expected)
	return expected == auth, err
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

func (v1 *v1API) getUsers(r *http.Request) (interface{}, error) {
	return v1.db.GetUsers()
}

func (v1 *v1API) putUser(r *http.Request) (interface{}, error) {
	name := r.URL.Path[len("/v1/users/"):]
	log.Println("putUser:", name)
	if name == "" {
		return "", fmt.Errorf("Missing name")
	}
	buf := make([]byte, 32)
	_, err := rand.Read(buf)
	if err != nil {
		return "", err
	}
	auth := base64.StdEncoding.EncodeToString(buf)
	err = v1.db.AddUser(name, auth)
	return auth, nil
}

func (v1 *v1API) getAuth(r *http.Request) (interface{}, error) {
	parts := strings.Split(r.Header.Get("Authorization"), ":")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid auth")
	}
	return v1.db.AuthUser(parts[0], parts[1])
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

func handle(w http.ResponseWriter, r *http.Request, f func(*http.Request) (interface{}, error)) {
	if result, err := f(r); err != nil {
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
}

func newMux(db db) *http.ServeMux {
	mux := http.NewServeMux()
	v1 := &v1API{db: db}

	mux.HandleFunc("/v1/auth", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handle(w, r, v1.getAuth)
		} else {
			http.Error(w, "", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/v1/users", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handle(w, r, v1.getUsers)
		} else {
			http.Error(w, "", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/v1/users/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			handle(w, r, v1.putUser)
		} else {
			http.Error(w, "", http.StatusMethodNotAllowed)
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
