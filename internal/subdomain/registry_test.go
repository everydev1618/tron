package subdomain

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("NewRegistry returned nil")
	}
	if r.subdomains == nil {
		t.Error("subdomains map not initialized")
	}
	if r.ports == nil {
		t.Error("ports map not initialized")
	}
	if r.projects == nil {
		t.Error("projects map not initialized")
	}
}

func TestAllocate(t *testing.T) {
	r := NewRegistry()

	alloc, err := r.Allocate("test-project")
	if err != nil {
		t.Fatalf("Allocate failed: %v", err)
	}

	if alloc.Subdomain == "" {
		t.Error("Subdomain is empty")
	}
	if len(alloc.Subdomain) != SubdomainLength {
		t.Errorf("Subdomain length = %d, want %d", len(alloc.Subdomain), SubdomainLength)
	}
	if alloc.Port < MinPort || alloc.Port > MaxPort {
		t.Errorf("Port %d not in range [%d, %d]", alloc.Port, MinPort, MaxPort)
	}
	if alloc.URL == "" {
		t.Error("URL is empty")
	}

	// Allocate same project should return same allocation
	alloc2, err := r.Allocate("test-project")
	if err != nil {
		t.Fatalf("Second allocate failed: %v", err)
	}
	if alloc2.Subdomain != alloc.Subdomain {
		t.Errorf("Subdomain mismatch: %s != %s", alloc2.Subdomain, alloc.Subdomain)
	}
	if alloc2.Port != alloc.Port {
		t.Errorf("Port mismatch: %d != %d", alloc2.Port, alloc.Port)
	}
}

func TestAllocateMultipleProjects(t *testing.T) {
	r := NewRegistry()

	alloc1, err := r.Allocate("project-1")
	if err != nil {
		t.Fatalf("Allocate project-1 failed: %v", err)
	}

	alloc2, err := r.Allocate("project-2")
	if err != nil {
		t.Fatalf("Allocate project-2 failed: %v", err)
	}

	// Should have different subdomains and ports
	if alloc1.Subdomain == alloc2.Subdomain {
		t.Error("Both projects got same subdomain")
	}
	if alloc1.Port == alloc2.Port {
		t.Error("Both projects got same port")
	}
}

func TestRelease(t *testing.T) {
	r := NewRegistry()

	alloc, _ := r.Allocate("test-project")
	subdomain := alloc.Subdomain
	port := alloc.Port

	r.Release("test-project")

	// Subdomain should no longer be valid
	if r.IsValidSubdomain(subdomain) {
		t.Error("Subdomain still valid after release")
	}

	// Project lookup should fail
	if _, ok := r.GetByProject("test-project"); ok {
		t.Error("Project still found after release")
	}

	// Port should be available for reuse
	_, exists := r.ports[port]
	if exists {
		t.Error("Port still in use after release")
	}
}

func TestGetBySubdomain(t *testing.T) {
	r := NewRegistry()

	alloc, _ := r.Allocate("test-project")

	port, ok := r.GetBySubdomain(alloc.Subdomain)
	if !ok {
		t.Error("GetBySubdomain returned false")
	}
	if port != alloc.Port {
		t.Errorf("Port = %d, want %d", port, alloc.Port)
	}

	// Non-existent subdomain
	_, ok = r.GetBySubdomain("nonexistent")
	if ok {
		t.Error("GetBySubdomain returned true for non-existent subdomain")
	}
}

func TestGetByProject(t *testing.T) {
	r := NewRegistry()

	alloc, _ := r.Allocate("test-project")

	got, ok := r.GetByProject("test-project")
	if !ok {
		t.Error("GetByProject returned false")
	}
	if got.Subdomain != alloc.Subdomain {
		t.Errorf("Subdomain = %s, want %s", got.Subdomain, alloc.Subdomain)
	}
	if got.Port != alloc.Port {
		t.Errorf("Port = %d, want %d", got.Port, alloc.Port)
	}

	// Non-existent project
	_, ok = r.GetByProject("nonexistent")
	if ok {
		t.Error("GetByProject returned true for non-existent project")
	}
}

func TestIsValidSubdomain(t *testing.T) {
	r := NewRegistry()

	alloc, _ := r.Allocate("test-project")

	if !r.IsValidSubdomain(alloc.Subdomain) {
		t.Error("IsValidSubdomain returned false for valid subdomain")
	}

	if r.IsValidSubdomain("nonexistent") {
		t.Error("IsValidSubdomain returned true for invalid subdomain")
	}
}

func TestList(t *testing.T) {
	r := NewRegistry()

	r.Allocate("project-1")
	r.Allocate("project-2")
	r.Allocate("project-3")

	allocations := r.List()
	if len(allocations) != 3 {
		t.Errorf("List returned %d allocations, want 3", len(allocations))
	}
}

func TestHandleCaddyAsk(t *testing.T) {
	r := NewRegistry()
	alloc, _ := r.Allocate("test-project")

	tests := []struct {
		name       string
		domain     string
		wantStatus int
	}{
		{
			name:       "valid subdomain",
			domain:     alloc.Subdomain + "." + Domain,
			wantStatus: http.StatusOK,
		},
		{
			name:       "invalid subdomain",
			domain:     "nonexistent." + Domain,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "wrong domain",
			domain:     alloc.Subdomain + ".example.com",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "no domain param",
			domain:     "",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/internal/caddy-ask"
			if tt.domain != "" {
				url += "?domain=" + tt.domain
			}

			req := httptest.NewRequest(http.MethodGet, url, nil)
			w := httptest.NewRecorder()

			r.HandleCaddyAsk(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("Status = %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}

func TestSubdomainFormat(t *testing.T) {
	r := NewRegistry()

	// Allocate several and verify format
	for i := 0; i < 10; i++ {
		alloc, err := r.Allocate("project-" + string(rune('a'+i)))
		if err != nil {
			t.Fatalf("Allocate failed: %v", err)
		}

		// Subdomain should be lowercase alphanumeric
		for _, c := range alloc.Subdomain {
			if !((c >= 'a' && c <= 'z') || (c >= '2' && c <= '7')) {
				t.Errorf("Invalid character in subdomain: %c (subdomain: %s)", c, alloc.Subdomain)
			}
		}
	}
}
