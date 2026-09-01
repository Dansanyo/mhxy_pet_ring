package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"time"

	"pet-ring/internal/domain"
	"pet-ring/internal/store"
)

type Options struct {
	DeviceSalt string
	RateLimit  int
	RateWindow time.Duration
	Now        func() time.Time
}

type api struct {
	store   store.EventStore
	salt    string
	now     func() time.Time
	limiter *fixedWindowLimiter
}

var safeIdentifier = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func New(eventStore store.EventStore, options Options) http.Handler {
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.RateLimit <= 0 {
		options.RateLimit = 240
	}
	if options.RateWindow <= 0 {
		options.RateWindow = time.Hour
	}
	service := &api{
		store:   eventStore,
		salt:    options.DeviceSalt,
		now:     options.Now,
		limiter: newFixedWindowLimiter(options.RateLimit, options.RateWindow, options.Now),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", service.health)
	mux.HandleFunc("GET /api/v1/model", service.model)
	mux.HandleFunc("POST /api/v1/events/tasks", service.taskEvent)
	mux.HandleFunc("DELETE /api/v1/events/tasks", service.deleteTaskEvents)
	mux.HandleFunc("POST /api/v1/events/rewards", service.rewardEvent)
	return securityHeaders(mux)
}

func (a *api) health(response http.ResponseWriter, _ *http.Request) {
	writeJSON(response, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *api) model(response http.ResponseWriter, request *http.Request) {
	model, err := a.store.AggregateModel(request.Context())
	if err != nil {
		writeError(response, http.StatusInternalServerError, "model unavailable")
		return
	}
	writeJSON(response, http.StatusOK, model)
}

type taskEventRequest struct {
	EventID         string `json:"eventId"`
	DeviceID        string `json:"deviceId"`
	RingNumber      int    `json:"ringNumber"`
	PlayerLevel     int    `json:"playerLevel"`
	TaskType        string `json:"taskType"`
	Resolution      string `json:"resolution"`
	RequiredQuality *int   `json:"requiredQuality"`
	ActualQuality   *int   `json:"actualQuality"`
}

func (a *api) taskEvent(response http.ResponseWriter, request *http.Request) {
	var input taskEventRequest
	if err := decodeJSON(response, request, &input); err != nil {
		writeError(response, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateIdentity(input.EventID, input.DeviceID); err != nil {
		writeError(response, http.StatusBadRequest, err.Error())
		return
	}
	if input.RingNumber < 1 || input.RingNumber > 100 || input.PlayerLevel < 1 || input.PlayerLevel > 200 {
		writeError(response, http.StatusBadRequest, "ring number or player level is out of range")
		return
	}
	resolution := input.Resolution
	if resolution == "" {
		resolution = domain.ResolutionFulfilled
	}
	score, err := domain.ScoreResolution(input.TaskType, resolution, input.RequiredQuality, input.ActualQuality)
	if err != nil {
		writeError(response, http.StatusBadRequest, err.Error())
		return
	}
	requestedScore, err := domain.RequestedTaskScore(input.TaskType, input.RequiredQuality, input.ActualQuality)
	if err != nil {
		writeError(response, http.StatusBadRequest, err.Error())
		return
	}
	deviceHash := a.deviceHash(input.DeviceID)
	if !a.limiter.allow(deviceHash + "|" + clientIP(request)) {
		writeError(response, http.StatusTooManyRequests, "too many submissions")
		return
	}
	err = a.store.InsertTaskEvent(request.Context(), store.TaskEvent{
		EventID: input.EventID, DeviceHash: deviceHash, RingNumber: input.RingNumber,
		LevelBand: playerLevelBand(input.PlayerLevel), TaskType: input.TaskType,
		Resolution: resolution, RequestedScore: requestedScore, Score: score, CreatedAt: a.now().UTC(),
	})
	if errors.Is(err, store.ErrDuplicate) {
		writeError(response, http.StatusConflict, "event already submitted")
		return
	}
	if err != nil {
		writeError(response, http.StatusInternalServerError, "could not store event")
		return
	}
	writeJSON(response, http.StatusAccepted, map[string]any{"accepted": true, "score": score})
}

type deleteTaskEventsRequest struct {
	DeviceID string   `json:"deviceId"`
	EventIDs []string `json:"eventIds"`
}

func (a *api) deleteTaskEvents(response http.ResponseWriter, request *http.Request) {
	var input deleteTaskEventsRequest
	if err := decodeJSON(response, request, &input); err != nil {
		writeError(response, http.StatusBadRequest, err.Error())
		return
	}
	if len(input.EventIDs) == 0 || len(input.EventIDs) > 100 {
		writeError(response, http.StatusBadRequest, "event ids must contain between 1 and 100 items")
		return
	}
	for _, eventID := range input.EventIDs {
		if err := validateIdentity(eventID, input.DeviceID); err != nil {
			writeError(response, http.StatusBadRequest, err.Error())
			return
		}
	}
	deviceHash := a.deviceHash(input.DeviceID)
	if !a.limiter.allow(deviceHash + "|" + clientIP(request)) {
		writeError(response, http.StatusTooManyRequests, "too many submissions")
		return
	}
	deleted, err := a.store.DeleteTaskEvents(request.Context(), deviceHash, input.EventIDs)
	if err != nil {
		writeError(response, http.StatusInternalServerError, "could not delete events")
		return
	}
	writeJSON(response, http.StatusOK, map[string]int64{"deleted": deleted})
}

type rewardEventRequest struct {
	EventID     string `json:"eventId"`
	DeviceID    string `json:"deviceId"`
	PlayerLevel int    `json:"playerLevel"`
	FinalScore  int    `json:"finalScore"`
	RewardType  string `json:"rewardType"`
	RewardLevel int    `json:"rewardLevel"`
}

func (a *api) rewardEvent(response http.ResponseWriter, request *http.Request) {
	var input rewardEventRequest
	if err := decodeJSON(response, request, &input); err != nil {
		writeError(response, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateIdentity(input.EventID, input.DeviceID); err != nil {
		writeError(response, http.StatusBadRequest, err.Error())
		return
	}
	if input.PlayerLevel < 1 || input.PlayerLevel > 200 || input.FinalScore < -1000 || input.FinalScore > 2000 {
		writeError(response, http.StatusBadRequest, "player level or score is out of range")
		return
	}
	if !validReward(input.RewardType, input.RewardLevel) {
		writeError(response, http.StatusBadRequest, "invalid reward type or level")
		return
	}
	deviceHash := a.deviceHash(input.DeviceID)
	if !a.limiter.allow(deviceHash + "|" + clientIP(request)) {
		writeError(response, http.StatusTooManyRequests, "too many submissions")
		return
	}
	err := a.store.InsertRewardEvent(request.Context(), store.RewardEvent{
		EventID: input.EventID, DeviceHash: deviceHash,
		LevelBand: playerLevelBand(input.PlayerLevel), FinalScore: input.FinalScore,
		RewardType: input.RewardType, RewardLevel: input.RewardLevel, CreatedAt: a.now().UTC(),
	})
	if errors.Is(err, store.ErrDuplicate) {
		writeError(response, http.StatusConflict, "event already submitted")
		return
	}
	if err != nil {
		writeError(response, http.StatusInternalServerError, "could not store event")
		return
	}
	writeJSON(response, http.StatusAccepted, map[string]bool{"accepted": true})
}

func validateIdentity(eventID, deviceID string) error {
	if len(eventID) < 8 || len(eventID) > 64 || !safeIdentifier.MatchString(eventID) {
		return fmt.Errorf("invalid event id")
	}
	if len(deviceID) < 16 || len(deviceID) > 128 || !safeIdentifier.MatchString(deviceID) {
		return fmt.Errorf("invalid device id")
	}
	return nil
}

func validReward(rewardType string, rewardLevel int) bool {
	switch rewardType {
	case "book", "iron":
		return rewardLevel >= 90 && rewardLevel <= 150 && rewardLevel%10 == 0
	case "war_soul":
		return rewardLevel == 160
	case "training_fruit", "training_exp", "furniture_plan", "other":
		return rewardLevel == 0
	default:
		return false
	}
}

func playerLevelBand(level int) int {
	bands := [...]int{69, 89, 109, 129, 138, 155, 159, 170, 175}
	best := bands[0]
	bestDistance := abs(level - best)
	for _, band := range bands[1:] {
		distance := abs(level - band)
		if distance < bestDistance {
			best, bestDistance = band, distance
		}
	}
	return best
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func (a *api) deviceHash(deviceID string) string {
	digest := sha256.Sum256([]byte(a.salt + "\x00" + deviceID))
	return hex.EncodeToString(digest[:])
}

func clientIP(request *http.Request) string {
	host, _, err := net.SplitHostPort(request.RemoteAddr)
	if err == nil {
		return host
	}
	return request.RemoteAddr
}

func decodeJSON(response http.ResponseWriter, request *http.Request, target any) error {
	request.Body = http.MaxBytesReader(response, request.Body, 16*1024)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid JSON body")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("request body must contain one JSON object")
	}
	return nil
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("X-Content-Type-Options", "nosniff")
		response.Header().Set("Referrer-Policy", "no-referrer")
		response.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		next.ServeHTTP(response, request)
	})
}

func writeError(response http.ResponseWriter, status int, message string) {
	writeJSON(response, status, map[string]string{"error": message})
}

func writeJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json; charset=utf-8")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}
