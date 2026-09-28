package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Audit needs-validation fix: the SSE ticket store had no cap, so one session
// could grow it for the whole 60 s TTL while the panel shares the resolver's
// process. The cap (and the mint over it) is pinned here.
func TestSSETicketStoreCapsPerSession(t *testing.T) {
	s := newSSETicketStore()
	tk := ""
	for i := 0; i < maxOutstandingSSETickets; i++ {
		got := s.mint("session-a")
		if got == "" {
			t.Fatalf("mint %d: unexpected refusal before the cap", i)
		}
		tk = got
	}
	if got := s.mint("session-a"); got != "" {
		t.Fatal("a session over the outstanding cap must be refused")
	}
	if got := s.mint("session-b"); got == "" {
		t.Fatal("a second session must not be blocked by another's cap")
	}
	if !s.consume(tk) {
		t.Fatal("a minted ticket must be consumable")
	}
	if got := s.mint("session-a"); got == "" {
		t.Fatal("consuming a ticket must return the owner's budget")
	}
}

// loginTestSession logs in through the real handler and returns the session
// token, so the request the auth gate wraps carries the same context key the
// production path sets.
func loginTestSession(t *testing.T, ws *WebServer) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(
		`{"username":"admin","password":"`+testAdminPassword+`"}`))
	rec := httptest.NewRecorder()
	ws.handleAuthLogin(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login status %d, want 200; body %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("parse login response: %v", err)
	}
	if out.Token == "" {
		t.Fatal("login returned no session token")
	}
	return out.Token
}

// The endpoint answers 429 over the cap, not a growing map. Driven through the
// real auth gate so the context key is set the way production sets it.
func TestSSETicketEndpointRefusesOverCap(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	token := loginTestSession(t, ws)
	for i := 0; i < maxOutstandingSSETickets; i++ {
		if tk := ws.sseTickets.mint(token); tk == "" {
			t.Fatalf("mint %d: unexpected refusal before the cap", i)
		}
	}

	handler := ws.requireAuth(ws.handleSSETicket)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/sse-ticket", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("over the cap: status %d, want 429; body %s", rec.Code, rec.Body.String())
	}

	// Consuming one frees a slot and the endpoint serves again.
	ws.sseTickets.mu.Lock()
	for k := range ws.sseTickets.tickets {
		ws.sseTickets.mu.Unlock()
		ws.sseTickets.consume(k)
		ws.sseTickets.mu.Lock()
		break
	}
	ws.sseTickets.mu.Unlock()

	rec = httptest.NewRecorder()
	handler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("after freeing a slot: status %d, want 200; body %s", rec.Code, rec.Body.String())
	}
}
