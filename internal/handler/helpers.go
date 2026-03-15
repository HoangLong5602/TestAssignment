package handler

import (
	"encoding/json"
	"net/http"
)

func requirePOST(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		http.Error(w, ErrMethodNotAllowed, http.StatusMethodNotAllowed)
		return false
	}
	return true
}

func getRoomID(r *http.Request) string {
	return r.URL.Query().Get(QueryRoomID)
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set(HeaderContentType, ContentTypeJSON)
	_ = json.NewEncoder(w).Encode(v)
}
