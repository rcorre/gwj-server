package main

import (
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"os"
	"testing"

	"net/http"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
)

func toJSON(data interface{}) []byte {
	b, err := json.Marshal(data)
	if err != nil {
		panic(err)
	}
	return b
}

func setup(t *testing.T) http.Handler {
	pg, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		panic(err)
	}

	/*
		if _, err := pg.Exec("DROP TABLE IF EXISTS records"); err != nil {
			panic(err)
		}
		t.Cleanup(func() { pg.Exec("DROP TABLE IF EXISTS records") })
	*/

	d := db{pg}
	if err := d.Init(); err != nil {
		panic(err)
	}
	return newMux(d)
}

func TestV1Users(t *testing.T) {
	v1 := setup(t)
	get := func() []string {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/v1/users", nil)
		v1.ServeHTTP(w, r)
		assert.Equal(t, w.Code, http.StatusOK)
		var res []string
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Errorf("Failed to unmarshal %s, %v", w.Body.Bytes(), err)
		}
		return res
	}
	put := func(name string) (int, string) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("PUT", "/v1/users/"+name, nil)
		v1.ServeHTTP(w, r)
		assert.NotEmpty(t, w.Body.String())
		if w.Code != 200 {
			return w.Code, w.Body.String()
		}
		var res string
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Errorf("Failed to unmarshal %s, %v", w.Body.Bytes(), err)
		}
		return w.Code, res
	}
	check := func(name, auth string) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/v1/auth", nil)
		r.Header.Set("Authorization", name+":"+auth)
		v1.ServeHTTP(w, r)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "true", w.Body.String())
	}

	// no records yet
	assert.Equal(t, []string{}, get())

	code, fooAuth := put("foo")
	assert.EqualValues(t, code, 200)
	assert.EqualValues(t, []string{"foo"}, get())

	code, barAuth := put("bar")
	assert.EqualValues(t, code, 200)
	assert.EqualValues(t, []string{"foo", "bar"}, get())

	check("foo", fooAuth)
	check("bar", barAuth)

	code, _ = put("foo")
	assert.EqualValues(t, code, 500)
}
