package web

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/dir01/bookworm/internal/catalog"
)

func NewMux(svc *catalog.Service) *http.ServeMux {
	mux := http.NewServeMux()
	h := &httpServer{svc: svc}
	mux.HandleFunc("/search", h.searchHandler)
	mux.HandleFunc("/book", h.bookHandler)
	return mux
}

type httpServer struct {
	svc *catalog.Service
}

func (h *httpServer) searchHandler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")

	results, err := h.svc.Search(r.Context(), query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := struct {
		Books []*catalog.BookMetadata `json:"books"`
	}{Books: results}

	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(response)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *httpServer) bookHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	formatStr := r.URL.Query().Get("format")

	var format catalog.FileType
	switch formatStr {
	case "fb2":
		format = catalog.FB2
	case "epub":
		format = catalog.EPUB
	default:
		http.Error(w, "Invalid format", http.StatusBadRequest)
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	book, cleanup, err := h.svc.GetBook(r.Context(), id, format)
	defer cleanup()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=book-%d.%s", id, format))
	w.Header().Set("Content-Type", "application/octet-stream")

	_, err = io.Copy(w, book)
	if err != nil {
		http.Error(w, "Failed to write response", http.StatusInternalServerError)
	}
}
