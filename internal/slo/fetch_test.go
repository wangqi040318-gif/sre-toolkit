package slo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wangqi040318-gif/sre-toolkit/internal/triage"
)

func TestFetchMockPrometheus(t *testing.T) {
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("query")
		queries = append(queries, q)
		value := "0.0001"
		switch {
		case strings.Contains(q, "[1h]"):
			value = "0.02"
		case strings.Contains(q, "[6h]"):
			value = "0.008"
		case strings.Contains(q, "[3d]"):
			value = "0.0015"
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1700000000,"` + value + `"]}]}}`))
	}))
	defer srv.Close()

	windows, err := ParseWindows("1h,6h,3d")
	if err != nil {
		t.Fatal(err)
	}
	client := &triage.Client{BaseURL: srv.URL, HTTP: srv.Client()}
	burns, err := Fetch(context.Background(), client, "checkout", windows, Templates{}, time.Unix(1700000000, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(queries) != 3 || !strings.Contains(queries[0], `job="checkout"`) || !strings.Contains(queries[0], "increase(") {
		t.Fatalf("queries = %v", queries)
	}
	rep := Assemble("checkout", 0.999, burns, false)
	if rep.Decision.Action != ActionPage {
		t.Fatalf("action = %s (%s)", rep.Decision.Action, rep.Decision.Why)
	}
	if rep.Burns[0].ErrorRatio != 0.02 || rep.Burns[2].Window.Label != "3d" {
		t.Fatalf("burns = %+v", rep.Burns)
	}
}

func TestFetchAllFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("upstream down"))
	}))
	defer srv.Close()
	windows, err := ParseWindows("5m,1h")
	if err != nil {
		t.Fatal(err)
	}
	_, err = Fetch(context.Background(), &triage.Client{BaseURL: srv.URL, HTTP: srv.Client()}, "checkout", windows, Templates{}, time.Now())
	if err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("err = %v", err)
	}
}
