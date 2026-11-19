package api

import (
	"net/http"
	"net/http/httptest"
)

// serve performs an unauthenticated request against h.
func serve(h http.Handler, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}
