package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pet-ring/internal/store"
)

func testAPI(t *testing.T, options Options) (http.Handler, *store.SQLiteStore) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "api.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if options.DeviceSalt == "" {
		options.DeviceSalt = "test-salt-that-is-long-enough"
	}
	return New(db, options), db
}

func requestJSON(t *testing.T, handler http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var payload bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&payload).Encode(body); err != nil {
			t.Fatalf("encode request: %v", err)
		}
	}
	request := httptest.NewRequest(method, path, &payload)
	request.Header.Set("Content-Type", "application/json")
	request.RemoteAddr = "192.0.2.10:12345"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestHealthEndpoint(t *testing.T) {
	handler, _ := testAPI(t, Options{})
	response := requestJSON(t, handler, http.MethodGet, "/api/v1/health", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	if response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("missing security header")
	}
}

func TestTaskEventIsValidatedScoredAndStored(t *testing.T) {
	handler, db := testAPI(t, Options{})
	response := requestJSON(t, handler, http.MethodPost, "/api/v1/events/tasks", map[string]any{
		"eventId":         "event-task-0001",
		"deviceId":        "anonymous-device-00000001",
		"ringNumber":      12,
		"playerLevel":     175,
		"taskType":        "medicine",
		"requiredQuality": 63,
		"actualQuality":   58,
	})
	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	model, err := db.AggregateModel(context.Background())
	if err != nil {
		t.Fatalf("AggregateModel: %v", err)
	}
	if model.TaskSamples != 1 || model.TaskBuckets[0].Score != 0 {
		t.Fatalf("stored model = %+v", model)
	}
}

func TestTaskEventRejectsUnknownTask(t *testing.T) {
	handler, _ := testAPI(t, Options{})
	response := requestJSON(t, handler, http.MethodPost, "/api/v1/events/tasks", map[string]any{
		"eventId": "event-task-0002", "deviceId": "anonymous-device-00000001",
		"ringNumber": 1, "playerLevel": 175, "taskType": "made_up",
	})
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.Code)
	}
}

func TestSkippedTaskStoresActionScoreWithoutPollutingRequestedTaskScore(t *testing.T) {
	handler, db := testAPI(t, Options{})
	response := requestJSON(t, handler, http.MethodPost, "/api/v1/events/tasks", map[string]any{
		"eventId": "event-task-skip-0001", "deviceId": "anonymous-device-00000001",
		"ringNumber": 32, "playerLevel": 175, "taskType": "mutant_specific", "resolution": "skipped",
	})
	if response.Code != http.StatusAccepted || !strings.Contains(response.Body.String(), `"score":-20`) {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	model, err := db.AggregateModel(context.Background())
	if err != nil {
		t.Fatalf("AggregateModel: %v", err)
	}
	if got := model.TaskBuckets[0]; got.TaskType != "mutant_specific" || got.Score != 10 {
		t.Fatalf("aggregate = %+v, want requested mutant task with canonical score", got)
	}
}

func TestDuplicateEventReturnsConflict(t *testing.T) {
	handler, _ := testAPI(t, Options{})
	body := map[string]any{
		"eventId": "event-task-0003", "deviceId": "anonymous-device-00000001",
		"ringNumber": 1, "playerLevel": 175, "taskType": "find_person",
	}
	if first := requestJSON(t, handler, http.MethodPost, "/api/v1/events/tasks", body); first.Code != http.StatusAccepted {
		t.Fatalf("first status = %d", first.Code)
	}
	if second := requestJSON(t, handler, http.MethodPost, "/api/v1/events/tasks", body); second.Code != http.StatusConflict {
		t.Fatalf("second status = %d, want 409", second.Code)
	}
}

func TestRewardEventAndPublicModelContainOnlyAggregates(t *testing.T) {
	handler, _ := testAPI(t, Options{})
	response := requestJSON(t, handler, http.MethodPost, "/api/v1/events/rewards", map[string]any{
		"eventId": "event-reward-0001", "deviceId": "anonymous-device-00000001",
		"playerLevel": 175, "finalScore": 202, "rewardType": "book", "rewardLevel": 130,
	})
	if response.Code != http.StatusAccepted {
		t.Fatalf("submit status = %d, body = %s", response.Code, response.Body.String())
	}
	modelResponse := requestJSON(t, handler, http.MethodGet, "/api/v1/model", nil)
	if modelResponse.Code != http.StatusOK {
		t.Fatalf("model status = %d", modelResponse.Code)
	}
	bodyText := modelResponse.Body.String()
	if strings.Contains(bodyText, "anonymous-device") || strings.Contains(bodyText, "event-reward") || strings.Contains(bodyText, "deviceHash") {
		t.Fatalf("public model leaked event identity: %s", bodyText)
	}
	if !strings.Contains(bodyText, `"rewardSamples":1`) {
		t.Fatalf("public model missing aggregate: %s", bodyText)
	}
}

func TestRateLimitReturnsTooManyRequests(t *testing.T) {
	handler, _ := testAPI(t, Options{RateLimit: 1, RateWindow: time.Hour})
	first := requestJSON(t, handler, http.MethodPost, "/api/v1/events/tasks", map[string]any{
		"eventId": "event-task-1001", "deviceId": "anonymous-device-00000001",
		"ringNumber": 1, "playerLevel": 175, "taskType": "find_person",
	})
	if first.Code != http.StatusAccepted {
		t.Fatalf("first status = %d", first.Code)
	}
	second := requestJSON(t, handler, http.MethodPost, "/api/v1/events/tasks", map[string]any{
		"eventId": "event-task-1002", "deviceId": "anonymous-device-00000001",
		"ringNumber": 2, "playerLevel": 175, "taskType": "find_person",
	})
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want 429", second.Code)
	}
}

func TestMalformedJSONIsRejected(t *testing.T) {
	handler, _ := testAPI(t, Options{})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/events/tasks", strings.NewReader("{"))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.Code)
	}
}
