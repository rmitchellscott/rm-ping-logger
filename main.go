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

type analyticsBody struct {
	DeviceType      string `json:"device_type"`
	SoftwareVersion string `json:"software_version"`
	OSVersion       string `json:"os_version"`
	ProductType     string `json:"product_type"`
	Events          []struct {
		EventName       string         `json:"event_name"`
		EventProperties map[string]any `json:"event_properties"`
		EventTimestamp  int64          `json:"event_timestamp"`
	} `json:"events"`
	UserProperties map[string]string `json:"user_properties"`
}

func logHandler(lokiURL string, status int, responseBody string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		defer r.Body.Close()

		var entry any

		var ab analyticsBody
		if err := json.Unmarshal(body, &ab); err == nil && len(ab.Events) > 0 {
			for _, evt := range ab.Events {
				entry = map[string]any{
					"path":             r.URL.Path,
					"device_type":      ab.DeviceType,
					"software_version": ab.SoftwareVersion,
					"os_version":       ab.OSVersion,
					"event_name":       evt.EventName,
					"event_properties": evt.EventProperties,
					"event_timestamp":  evt.EventTimestamp,
					"user_agent":       r.Header.Get("User-Agent"),
				}
				entryJSON, _ := json.Marshal(entry)
				fmt.Println(string(entryJSON))
				if lokiURL != "" {
					go pushToLoki(lokiURL, r.URL.Path, evt.EventName, entryJSON)
				}
			}
		} else {
			entry = map[string]any{
				"path":       r.URL.Path,
				"user_agent": r.Header.Get("User-Agent"),
				"body":       string(body),
			}
			entryJSON, _ := json.Marshal(entry)
			fmt.Println(string(entryJSON))
			if lokiURL != "" {
				go pushToLoki(lokiURL, r.URL.Path, "", entryJSON)
			}
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

func pushToLoki(lokiURL string, path string, eventName string, entryJSON []byte) {
	labels := map[string]string{
		"app":  "rm-ping-logger",
		"path": path,
	}
	if eventName != "" {
		labels["event_name"] = eventName
	}

	payload := lokiPush{
		Streams: []lokiStream{{
			Stream: labels,
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
