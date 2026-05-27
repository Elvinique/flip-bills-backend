package response

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newContext(w *httptest.ResponseRecorder) *gin.Context {
	c, _ := gin.CreateTestContext(w)
	return c
}

func TestOK(t *testing.T) {
	w := httptest.NewRecorder()
	c := newContext(w)
	OK(c, "test ok", gin.H{"key": "value"})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp APIResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if !resp.Success {
		t.Fatal("success should be true")
	}
	if resp.Message != "test ok" {
		t.Fatalf("message = %q, want 'test ok'", resp.Message)
	}
}

func TestCreated(t *testing.T) {
	w := httptest.NewRecorder()
	c := newContext(w)
	Created(c, "created", nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", w.Code)
	}
}

func TestBadRequest(t *testing.T) {
	w := httptest.NewRecorder()
	c := newContext(w)
	BadRequest(c, "bad input", "field required")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	var resp APIResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Success {
		t.Fatal("success should be false")
	}
	if resp.Error == nil {
		t.Fatal("error should be set")
	}
}

func TestUnauthorized(t *testing.T) {
	w := httptest.NewRecorder()
	c := newContext(w)
	Unauthorized(c, "token expired")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestForbidden(t *testing.T) {
	w := httptest.NewRecorder()
	c := newContext(w)
	Forbidden(c, "KYC upgrade required")
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
}

func TestNotFound(t *testing.T) {
	w := httptest.NewRecorder()
	c := newContext(w)
	NotFound(c, "booking not found")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestInternalError(t *testing.T) {
	w := httptest.NewRecorder()
	c := newContext(w)
	InternalError(c, "something went wrong", nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

func TestUnprocessableEntity(t *testing.T) {
	w := httptest.NewRecorder()
	c := newContext(w)
	UnprocessableEntity(c, "validation failed", "amount too low")
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", w.Code)
	}
	var resp APIResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Success {
		t.Fatal("success should be false")
	}
}
