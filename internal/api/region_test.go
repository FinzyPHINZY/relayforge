package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStandbyRejectsWritesAndAllowsReads(t *testing.T) {
	s := &Server{RegionRole: "standby"}
	handler := s.regionWrites(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	for _, item := range []struct {
		method string
		want   int
	}{{http.MethodGet, 200}, {http.MethodPost, 503}, {http.MethodPatch, 503}} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(item.method, "/v1/events", nil))
		if recorder.Code != item.want {
			t.Fatalf("method=%s status=%d", item.method, recorder.Code)
		}
	}
}
