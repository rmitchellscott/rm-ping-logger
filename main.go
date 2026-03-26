package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

type lokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][]string        `json:"values"`
}

type lokiPush struct {
	Streams []lokiStream `json:"streams"`
}

func main() {
	lokiURL := os.Getenv("LOKI_URL")
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("POST /analytics/v2/events", logHandler(lokiURL, http.StatusCreated, `{"message": "Success"}`))
	mux.HandleFunc("POST /v1/reports", logHandler(lokiURL, http.StatusOK, ""))
	mux.HandleFunc("POST /v2/reports", logHandler(lokiURL, http.StatusOK, ""))
	mux.HandleFunc("POST /report/v1", logHandler(lokiURL, http.StatusOK, ""))
	mux.HandleFunc("POST /v2/events", logHandler(lokiURL, http.StatusOK, ""))

	log.Printf("listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

func logHandler(lokiURL string, status int, responseBody string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		defer r.Body.Close()

		entry := map[string]any{
			"timestamp":      time.Now().UTC().Format(time.RFC3339Nano),
			"method":         r.Method,
			"path":           r.URL.Path,
			"query":          r.URL.RawQuery,
			"headers":        r.Header,
			"content_length": r.ContentLength,
			"remote_addr":    r.RemoteAddr,
			"body":           string(body),
		}

		entryJSON, _ := json.Marshal(entry)
		fmt.Println(string(entryJSON))

		if lokiURL != "" {
			go pushToLoki(lokiURL, r.URL.Path, entryJSON)
		}

		if responseBody != "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			fmt.Fprint(w, responseBody)
		} else {
			w.WriteHeader(status)
		}
	}
}

func pushToLoki(lokiURL string, path string, entryJSON []byte) {
	payload := lokiPush{
		Streams: []lokiStream{{
			Stream: map[string]string{
				"app":  "rm-ping-logger",
				"path": path,
			},
			Values: [][]string{
				{fmt.Sprintf("%d", time.Now().UnixNano()), string(entryJSON)},
			},
		}},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("loki marshal error: %v", err)
		return
	}

	resp, err := http.Post(lokiURL+"/loki/api/v1/push", "application/json", bytes.NewReader(data))
	if err != nil {
		log.Printf("loki push error: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		log.Printf("loki push status %d: %s", resp.StatusCode, string(respBody))
	}
}
