package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"pet-ring/internal/httpapi"
	"pet-ring/internal/store"
)

func testHandler(t *testing.T) http.Handler {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "server.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return newHandler(httpapi.New(db, httpapi.Options{DeviceSalt: "test-salt"}))
}

func TestServesEmbeddedHomePage(t *testing.T) {
	handler := testHandler(t)
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	if !strings.Contains(response.Body.String(), "宠环模拟器") {
		t.Fatalf("home page missing title: %s", response.Body.String())
	}
}

func TestServesCSSWithCorrectContentType(t *testing.T) {
	handler := testHandler(t)
	request := httptest.NewRequest(http.MethodGet, "/styles.css", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	if contentType := response.Header().Get("Content-Type"); !strings.Contains(contentType, "text/css") {
		t.Fatalf("content type = %q", contentType)
	}
}

func TestFrontendRouteFallsBackToHomePage(t *testing.T) {
	handler := testHandler(t)
	request := httptest.NewRequest(http.MethodGet, "/history", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "宠环模拟器") {
		t.Fatalf("frontend fallback status/body = %d/%s", response.Code, response.Body.String())
	}
}

func TestAPIRouteIsNotHandledByFrontendFallback(t *testing.T) {
	handler := testHandler(t)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	if !strings.Contains(response.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("content type = %q", response.Header().Get("Content-Type"))
	}
}
