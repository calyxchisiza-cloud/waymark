package waymark

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"time"
)

// Handler registers names on a book.
func Handler(book *Book) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/names/{name}", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Addresses  []string `json:"addresses"`
			TTLSeconds int      `json:"ttl_seconds"`
		}
		dec := json.NewDecoder(io.LimitReader(r.Body, 1<<16))
		if err := dec.Decode(&body); err != nil {
			http.Error(w, ErrInvalid.Error(), http.StatusBadRequest)
			return
		}
		err := book.Set(r.PathValue("name"), body.Addresses, time.Duration(body.TTLSeconds)*time.Second)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("DELETE /v1/names/{name}", func(w http.ResponseWriter, r *http.Request) {
		err := book.Drop(r.PathValue("name"), r.URL.Query().Get("address"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	return mux
}

// ServeHTTP listens on a loopback address.
func ServeHTTP(addr string, book *Book) (*http.Server, error) {
	if err := loopback(addr); err != nil {
		return nil, err
	}
	srv := &http.Server{
		Addr:              addr,
		Handler:           Handler(book),
		ReadHeaderTimeout: 5 * time.Second,
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	go func() { _ = srv.Serve(ln) }()
	return srv, nil
}
