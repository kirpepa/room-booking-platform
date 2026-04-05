package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var baseURL string

func TestMain(m *testing.M) {
	baseURL = os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}

	// Wait for gateway to be ready
	for i := 0; i < 30; i++ {
		resp, err := http.Get(baseURL + "/_info")
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			break
		}
		time.Sleep(1 * time.Second)
	}

	os.Exit(m.Run())
}

func TestInfoEndpoint(t *testing.T) {
	resp, err := http.Get(baseURL + "/_info")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestDummyLogin(t *testing.T) {
	t.Run("admin", func(t *testing.T) {
		token := dummyLogin(t, "admin")
		assert.NotEmpty(t, token)
	})

	t.Run("user", func(t *testing.T) {
		token := dummyLogin(t, "user")
		assert.NotEmpty(t, token)
	})

	t.Run("invalid role", func(t *testing.T) {
		resp := postJSON(t, "/dummyLogin", map[string]string{"role": "superadmin"})
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

func TestFullBookingFlow(t *testing.T) {
	adminToken := dummyLogin(t, "admin")
	userToken := dummyLogin(t, "user")

	// Step 1: Admin creates a room
	roomResp := postJSONAuth(t, "/rooms/create", map[string]interface{}{
		"name":        "Test Room E2E",
		"description": "E2E test room",
		"capacity":    10,
	}, adminToken)
	require.Equal(t, http.StatusCreated, roomResp.StatusCode)

	var roomBody struct {
		Room struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"room"`
	}
	decodeBody(t, roomResp, &roomBody)
	roomID := roomBody.Room.ID
	require.NotEmpty(t, roomID)
	t.Logf("Created room: %s", roomID)

	// Step 2: Admin creates schedule
	// Use tomorrow's day of week to ensure slots exist
	tomorrow := time.Now().UTC().Add(24 * time.Hour)
	dayOfWeek := int(tomorrow.Weekday())
	if dayOfWeek == 0 {
		dayOfWeek = 7 // Sunday
	}

	schedResp := postJSONAuth(t, fmt.Sprintf("/rooms/%s/schedule/create", roomID), map[string]interface{}{
		"roomId":     roomID,
		"daysOfWeek": []int{dayOfWeek},
		"startTime":  "09:00",
		"endTime":    "12:00",
	}, adminToken)
	require.Equal(t, http.StatusCreated, schedResp.StatusCode)

	var schedBody struct {
		Schedule struct {
			ID     string `json:"id"`
			RoomID string `json:"roomId"`
		} `json:"schedule"`
	}
	decodeBody(t, schedResp, &schedBody)
	t.Logf("Created schedule: %s", schedBody.Schedule.ID)

	// Step 3: User lists available slots
	dateStr := tomorrow.Format("2006-01-02")
	slotsResp := getAuth(t, fmt.Sprintf("/rooms/%s/slots/list?date=%s", roomID, dateStr), userToken)
	require.Equal(t, http.StatusOK, slotsResp.StatusCode)

	var slotsBody struct {
		Slots []struct {
			ID     string `json:"id"`
			RoomID string `json:"roomId"`
			Start  string `json:"start"`
			End    string `json:"end"`
		} `json:"slots"`
	}
	decodeBody(t, slotsResp, &slotsBody)
	require.NotEmpty(t, slotsBody.Slots, "expected at least one slot")
	t.Logf("Found %d available slots", len(slotsBody.Slots))

	slotID := slotsBody.Slots[0].ID

	// Step 4: User creates booking
	bookResp := postJSONAuth(t, "/bookings/create", map[string]interface{}{
		"slotId":               slotID,
		"createConferenceLink": true,
	}, userToken)
	require.Equal(t, http.StatusCreated, bookResp.StatusCode)

	var bookBody struct {
		Booking struct {
			ID     string `json:"id"`
			SlotID string `json:"slotId"`
			UserID string `json:"userId"`
			Status string `json:"status"`
		} `json:"booking"`
	}
	decodeBody(t, bookResp, &bookBody)
	bookingID := bookBody.Booking.ID
	assert.Equal(t, "active", bookBody.Booking.Status)
	assert.Equal(t, slotID, bookBody.Booking.SlotID)
	t.Logf("Created booking: %s", bookingID)

	// Verify slot is no longer available
	slotsResp2 := getAuth(t, fmt.Sprintf("/rooms/%s/slots/list?date=%s", roomID, dateStr), userToken)
	require.Equal(t, http.StatusOK, slotsResp2.StatusCode)

	var slotsBody2 struct {
		Slots []struct {
			ID string `json:"id"`
		} `json:"slots"`
	}
	decodeBody(t, slotsResp2, &slotsBody2)
	for _, s := range slotsBody2.Slots {
		assert.NotEqual(t, slotID, s.ID, "booked slot should not appear in available slots")
	}

	// Step 5: Verify /bookings/my
	myResp := getAuth(t, "/bookings/my", userToken)
	require.Equal(t, http.StatusOK, myResp.StatusCode)
	var myBody struct {
		Bookings []struct {
			ID string `json:"id"`
		} `json:"bookings"`
	}
	decodeBody(t, myResp, &myBody)
	found := false
	for _, b := range myBody.Bookings {
		if b.ID == bookingID {
			found = true
		}
	}
	assert.True(t, found, "booking should appear in /bookings/my")

	// Step 6: Double-booking same slot should fail
	doubleBook := postJSONAuth(t, "/bookings/create", map[string]interface{}{
		"slotId": slotID,
	}, userToken)
	assert.Equal(t, http.StatusConflict, doubleBook.StatusCode)
}

func TestCancelBookingFlow(t *testing.T) {
	adminToken := dummyLogin(t, "admin")
	userToken := dummyLogin(t, "user")

	// Create room
	roomResp := postJSONAuth(t, "/rooms/create", map[string]interface{}{
		"name": "Cancel Test Room",
	}, adminToken)
	require.Equal(t, http.StatusCreated, roomResp.StatusCode)
	var roomBody struct {
		Room struct{ ID string `json:"id"` } `json:"room"`
	}
	decodeBody(t, roomResp, &roomBody)
	roomID := roomBody.Room.ID

	// Create schedule
	tomorrow := time.Now().UTC().Add(24 * time.Hour)
	dayOfWeek := int(tomorrow.Weekday())
	if dayOfWeek == 0 {
		dayOfWeek = 7
	}
	postJSONAuth(t, fmt.Sprintf("/rooms/%s/schedule/create", roomID), map[string]interface{}{
		"roomId":     roomID,
		"daysOfWeek": []int{dayOfWeek},
		"startTime":  "14:00",
		"endTime":    "16:00",
	}, adminToken)

	// Get slots
	dateStr := tomorrow.Format("2006-01-02")
	slotsResp := getAuth(t, fmt.Sprintf("/rooms/%s/slots/list?date=%s", roomID, dateStr), userToken)
	var slotsBody struct {
		Slots []struct{ ID string `json:"id"` } `json:"slots"`
	}
	decodeBody(t, slotsResp, &slotsBody)
	require.NotEmpty(t, slotsBody.Slots)
	slotID := slotsBody.Slots[0].ID

	// Create booking
	bookResp := postJSONAuth(t, "/bookings/create", map[string]interface{}{"slotId": slotID}, userToken)
	require.Equal(t, http.StatusCreated, bookResp.StatusCode)
	var bookBody struct {
		Booking struct{ ID string `json:"id"` } `json:"booking"`
	}
	decodeBody(t, bookResp, &bookBody)
	bookingID := bookBody.Booking.ID

	// Cancel booking
	cancelResp := postJSONAuth(t, fmt.Sprintf("/bookings/%s/cancel", bookingID), nil, userToken)
	require.Equal(t, http.StatusOK, cancelResp.StatusCode)
	var cancelBody struct {
		Booking struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"booking"`
	}
	decodeBody(t, cancelResp, &cancelBody)
	assert.Equal(t, "cancelled", cancelBody.Booking.Status)

	// Idempotent cancel — second call should also return 200
	cancelResp2 := postJSONAuth(t, fmt.Sprintf("/bookings/%s/cancel", bookingID), nil, userToken)
	assert.Equal(t, http.StatusOK, cancelResp2.StatusCode)
	var cancelBody2 struct {
		Booking struct{ Status string `json:"status"` } `json:"booking"`
	}
	decodeBody(t, cancelResp2, &cancelBody2)
	assert.Equal(t, "cancelled", cancelBody2.Booking.Status)

	// Slot should be available again
	slotsResp2 := getAuth(t, fmt.Sprintf("/rooms/%s/slots/list?date=%s", roomID, dateStr), userToken)
	var slotsBody2 struct {
		Slots []struct{ ID string `json:"id"` } `json:"slots"`
	}
	decodeBody(t, slotsResp2, &slotsBody2)
	found := false
	for _, s := range slotsBody2.Slots {
		if s.ID == slotID {
			found = true
		}
	}
	assert.True(t, found, "cancelled slot should be available again")
}

func TestAdminCannotBook(t *testing.T) {
	adminToken := dummyLogin(t, "admin")

	resp := postJSONAuth(t, "/bookings/create", map[string]interface{}{
		"slotId": "00000000-0000-0000-0000-000000000099",
	}, adminToken)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestScheduleImmutable(t *testing.T) {
	adminToken := dummyLogin(t, "admin")

	// Create room
	roomResp := postJSONAuth(t, "/rooms/create", map[string]interface{}{
		"name": "Immutable Schedule Room",
	}, adminToken)
	var roomBody struct {
		Room struct{ ID string `json:"id"` } `json:"room"`
	}
	decodeBody(t, roomResp, &roomBody)
	roomID := roomBody.Room.ID

	// Create schedule first time
	schedResp1 := postJSONAuth(t, fmt.Sprintf("/rooms/%s/schedule/create", roomID), map[string]interface{}{
		"roomId":     roomID,
		"daysOfWeek": []int{1, 2, 3},
		"startTime":  "09:00",
		"endTime":    "10:00",
	}, adminToken)
	require.Equal(t, http.StatusCreated, schedResp1.StatusCode)

	// Try creating again — should be 409
	schedResp2 := postJSONAuth(t, fmt.Sprintf("/rooms/%s/schedule/create", roomID), map[string]interface{}{
		"roomId":     roomID,
		"daysOfWeek": []int{4, 5},
		"startTime":  "10:00",
		"endTime":    "11:00",
	}, adminToken)
	assert.Equal(t, http.StatusConflict, schedResp2.StatusCode)
}

func TestRegisterAndLogin(t *testing.T) {
	email := fmt.Sprintf("test-%d@example.com", time.Now().UnixNano())

	// Register
	regResp := postJSON(t, "/register", map[string]string{
		"email":    email,
		"password": "testpass123",
		"role":     "user",
	})
	require.Equal(t, http.StatusCreated, regResp.StatusCode)

	// Login
	loginResp := postJSON(t, "/login", map[string]string{
		"email":    email,
		"password": "testpass123",
	})
	require.Equal(t, http.StatusOK, loginResp.StatusCode)

	var loginBody struct {
		Token string `json:"token"`
	}
	decodeBody(t, loginResp, &loginBody)
	assert.NotEmpty(t, loginBody.Token)

	// Use token to list rooms
	roomsResp := getAuth(t, "/rooms/list", loginBody.Token)
	assert.Equal(t, http.StatusOK, roomsResp.StatusCode)
}

// --- helpers ---

func dummyLogin(t *testing.T, role string) string {
	t.Helper()
	resp := postJSON(t, "/dummyLogin", map[string]string{"role": role})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var body struct {
		Token string `json:"token"`
	}
	decodeBody(t, resp, &body)
	require.NotEmpty(t, body.Token)
	return body.Token
}

func postJSON(t *testing.T, path string, body interface{}) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	resp, err := http.Post(baseURL+path, "application/json", &buf)
	require.NoError(t, err)
	return resp
}

func postJSONAuth(t *testing.T, path string, body interface{}, token string) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req, err := http.NewRequest("POST", baseURL+path, &buf)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func getAuth(t *testing.T, path string, token string) *http.Response {
	t.Helper()
	req, err := http.NewRequest("GET", baseURL+path, nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func decodeBody(t *testing.T, resp *http.Response, v interface{}) {
	t.Helper()
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	err = json.Unmarshal(data, v)
	require.NoError(t, err, "body: %s", string(data))
}
