package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"net/http"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/suite"
)

type Suite struct {
	suite.Suite
	server http.Handler
}

func (s *Suite) SetupTest() {
	s.server = newServer()
}

func TestExampleTestSuite(t *testing.T) {
	suite.Run(t, new(Suite))
}

func (s *Suite) req(method string, path string, body interface{}, out interface{}) int {
	b, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(method, path, bytes.NewReader(b))
	s.server.ServeHTTP(w, r)

	respBytes := w.Body.Bytes()
	if err := json.Unmarshal(respBytes, out); err != nil {
		panic(fmt.Errorf("failed to unmarshal %s: %v", respBytes, err))
	}
	return w.Code
}

func (s *Suite) get(path string, out interface{}) int {
	return s.req(http.MethodGet, path, nil, out)
}

func (s *Suite) post(path string, body interface{}, out interface{}) int {
	return s.req(http.MethodPost, path, body, out)
}

func (s *Suite) TestPlayers() {
	// no records yet
	var players []player
	s.Equal(s.get("/players", &players), 200)
	s.Empty(players)

	var newPlayer player
	s.Equal(s.post("/players", map[string]string{"name": "foo", "auth": "abcde"}, &newPlayer), 200)
	s.Equal(newPlayer, player{ID: 1, Name: "foo"})

	s.Equal(s.post("/players", map[string]string{"name": "bar", "auth": "123de"}, &newPlayer), 200)
	s.Equal(newPlayer, player{ID: 2, Name: "bar"})
}
