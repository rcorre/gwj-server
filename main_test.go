package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"net/http"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/suite"
)

type fakeClock struct {
	now int64
}

func (f *fakeClock) Now() time.Time { return time.Unix(f.now, 0) }

type Suite struct {
	suite.Suite
	server http.Handler
	clock  fakeClock
}

func (s *Suite) SetupTest() {
	s.server = newServer(&s.clock)
}

func TestExampleTestSuite(t *testing.T) {
	suite.Run(t, new(Suite))
}

func (s *Suite) req(method string, path string, body interface{}, out interface{}, auth string) int {
	b, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(method, path, bytes.NewReader(b))
	r.Header.Set("Authorization", auth)
	s.server.ServeHTTP(w, r)

	respBytes := w.Body.Bytes()
	if w.Code < 300 {
		if err := json.Unmarshal(respBytes, out); err != nil {
			panic(fmt.Errorf("failed to unmarshal %s: %v", respBytes, err))
		}
	}
	return w.Code
}

func (s *Suite) get(path string, out interface{}) int {
	return s.req(http.MethodGet, path, nil, out, "")
}

func (s *Suite) post(path string, body interface{}, out interface{}) int {
	return s.req(http.MethodPost, path, body, out, "")
}

func (s *Suite) put(path string, body interface{}, out interface{}, auth string) int {
	return s.req(http.MethodPut, path, body, out, auth)
}

func (s *Suite) TestPlayers() {
	startTime := int64(1600000000)
	s.clock.now = startTime

	emptyPlots := map[int64]plot{
		0: {ID: 0},
		1: {ID: 1},
		2: {ID: 2},
		3: {ID: 3},
		4: {ID: 4},
		5: {ID: 5},
	}

	// no records yet
	players := map[int64]player{}
	s.Equal(s.get("/players", &players), 200)
	s.Empty(players)

	var newPlayer player
	s.Equal(s.post("/players", map[string]string{"name": "foo", "auth": "abcde"}, &newPlayer), 200)
	s.Equal(newPlayer, player{ID: 1, Name: "foo"})

	s.Equal(s.get("/players", &players), 200)
	s.Equal(players, map[int64]player{
		1: {
			ID:    1,
			Name:  "foo",
			Plots: emptyPlots,
		},
	})

	s.Equal(s.post("/players", map[string]string{"name": "bar", "auth": "123de"}, &newPlayer), 200)
	s.Equal(newPlayer, player{ID: 2, Name: "bar"})

	s.Equal(s.get("/players", &players), 200)
	s.Equal(players, map[int64]player{
		1: {
			ID:    1,
			Name:  "foo",
			Plots: emptyPlots,
		},
		2: {
			ID:    2,
			Name:  "bar",
			Plots: emptyPlots,
		},
	})

	var pl plot

	s.Equal(s.get("/players/1/plots/0", &pl), 200)
	s.Equal(pl, plot{ID: 0})
	s.Equal(s.get("/players/1/plots/5", &pl), 200)
	s.Equal(pl, plot{ID: 5})

	s.Equal(s.put("/players/1/plots/5", &pl, nil, "badauth"), 401)

	pl.Item = ITEM_CARROT_SEED
	s.Equal(s.put("/players/1/plots/5", &pl, &pl, "abcde"), 200)
	s.Equal(pl, plot{
		ID:         5,
		Item:       ITEM_CARROT_SEED,
		Transition: startTime + 10,
	})

	// don't transition yet
	s.clock.now = startTime + 1
	s.Equal(s.get("/players/1/plots/5", &pl), 200)
	s.Equal(pl, plot{
		ID:         5,
		Item:       ITEM_CARROT_SEED,
		Transition: startTime + 10,
	})

	// transition
	s.clock.now = startTime + 11
	s.Equal(s.get("/players/1/plots/5", &pl), 200)
	s.Equal(pl, plot{
		ID:         5,
		Item:       ITEM_CARROT,
		Transition: 0,
	})
}
