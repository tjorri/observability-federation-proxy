package tenant

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestNewWatcher(t *testing.T) {
	client := fake.NewSimpleClientset()

	tests := []struct {
		name    string
		cfg     WatcherConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			cfg: WatcherConfig{
				ClusterName: "test-cluster",
				Client:      client,
			},
			wantErr: false,
		},
		{
			name: "valid config with patterns",
			cfg: WatcherConfig{
				ClusterName:     "test-cluster",
				Client:          client,
				IncludePatterns: []string{"^game-.*", "^app-.*"},
				ExcludePatterns: []string{"^kube-.*"},
				RefreshInterval: 30 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "missing client",
			cfg: WatcherConfig{
				ClusterName: "test-cluster",
			},
			wantErr: true,
			errMsg:  "kubernetes client is required",
		},
		{
			name: "missing cluster name",
			cfg: WatcherConfig{
				Client: client,
			},
			wantErr: true,
			errMsg:  "cluster name is required",
		},
		{
			name: "invalid include pattern",
			cfg: WatcherConfig{
				ClusterName:     "test-cluster",
				Client:          client,
				IncludePatterns: []string{"[invalid"},
			},
			wantErr: true,
			errMsg:  "invalid include pattern",
		},
		{
			name: "invalid exclude pattern",
			cfg: WatcherConfig{
				ClusterName:     "test-cluster",
				Client:          client,
				ExcludePatterns: []string{"[invalid"},
			},
			wantErr: true,
			errMsg:  "invalid exclude pattern",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, err := NewWatcher(tt.cfg)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				} else if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if w == nil {
					t.Error("expected non-nil watcher")
				}
			}
		})
	}
}

func TestWatcher_ShouldInclude(t *testing.T) {
	client := fake.NewSimpleClientset()

	tests := []struct {
		name            string
		includePatterns []string
		excludePatterns []string
		namespace       string
		want            bool
	}{
		{
			name:      "no patterns - include all",
			namespace: "default",
			want:      true,
		},
		{
			name:      "no patterns - include system",
			namespace: "kube-system",
			want:      true,
		},
		{
			name:            "include pattern matches",
			includePatterns: []string{"^game-.*"},
			namespace:       "game-prod",
			want:            true,
		},
		{
			name:            "include pattern doesn't match",
			includePatterns: []string{"^game-.*"},
			namespace:       "default",
			want:            false,
		},
		{
			name:            "exclude pattern matches",
			excludePatterns: []string{"^kube-.*"},
			namespace:       "kube-system",
			want:            false,
		},
		{
			name:            "exclude pattern doesn't match",
			excludePatterns: []string{"^kube-.*"},
			namespace:       "default",
			want:            true,
		},
		{
			name:            "include and exclude - include matches, exclude doesn't",
			includePatterns: []string{"^game-.*"},
			excludePatterns: []string{".*-test$"},
			namespace:       "game-prod",
			want:            true,
		},
		{
			name:            "include and exclude - both match (exclude wins)",
			includePatterns: []string{"^game-.*"},
			excludePatterns: []string{".*-test$"},
			namespace:       "game-test",
			want:            false,
		},
		{
			name:            "multiple include patterns - first matches",
			includePatterns: []string{"^game-.*", "^app-.*"},
			namespace:       "game-prod",
			want:            true,
		},
		{
			name:            "multiple include patterns - second matches",
			includePatterns: []string{"^game-.*", "^app-.*"},
			namespace:       "app-prod",
			want:            true,
		},
		{
			name:            "multiple include patterns - none match",
			includePatterns: []string{"^game-.*", "^app-.*"},
			namespace:       "default",
			want:            false,
		},
		{
			name:            "multiple exclude patterns - first matches",
			excludePatterns: []string{"^kube-.*", "^observability$"},
			namespace:       "kube-system",
			want:            false,
		},
		{
			name:            "multiple exclude patterns - second matches",
			excludePatterns: []string{"^kube-.*", "^observability$"},
			namespace:       "observability",
			want:            false,
		},
		{
			name:            "exact match pattern",
			excludePatterns: []string{"^default$"},
			namespace:       "default",
			want:            false,
		},
		{
			name:            "exact match pattern - doesn't match similar",
			excludePatterns: []string{"^default$"},
			namespace:       "default-app",
			want:            true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, err := NewWatcher(WatcherConfig{
				ClusterName:     "test",
				Client:          client,
				IncludePatterns: tt.includePatterns,
				ExcludePatterns: tt.excludePatterns,
			})
			if err != nil {
				t.Fatalf("failed to create watcher: %v", err)
			}

			got := w.shouldInclude(tt.namespace)
			if got != tt.want {
				t.Errorf("shouldInclude(%q) = %v, want %v", tt.namespace, got, tt.want)
			}
		})
	}
}

func TestWatcher_BuildOrgIDHeader(t *testing.T) {
	client := fake.NewSimpleClientset()

	w, err := NewWatcher(WatcherConfig{
		ClusterName: "test",
		Client:      client,
	})
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}

	// Manually set tenants for testing
	w.mu.Lock()
	w.tenants = []string{"tenant-a", "tenant-b", "tenant-c"}
	w.mu.Unlock()

	tests := []struct {
		name      string
		maxLength int
		want      string
	}{
		{
			name:      "no limit",
			maxLength: 0,
			want:      "tenant-a|tenant-b|tenant-c",
		},
		{
			name:      "large limit",
			maxLength: 1000,
			want:      "tenant-a|tenant-b|tenant-c",
		},
		{
			name:      "exact fit",
			maxLength: 27, // len("tenant-a|tenant-b|tenant-c") = 27
			want:      "tenant-a|tenant-b|tenant-c",
		},
		{
			name:      "truncate to two tenants",
			maxLength: 18, // len("tenant-a|tenant-b") = 17
			want:      "tenant-a|tenant-b",
		},
		{
			name:      "truncate to one tenant",
			maxLength: 10,
			want:      "tenant-a",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := w.BuildOrgIDHeader(tt.maxLength)
			if got != tt.want {
				t.Errorf("BuildOrgIDHeader(%d) = %q, want %q", tt.maxLength, got, tt.want)
			}
		})
	}
}

func TestWatcher_BuildOrgIDHeader_Empty(t *testing.T) {
	client := fake.NewSimpleClientset()

	w, err := NewWatcher(WatcherConfig{
		ClusterName: "test",
		Client:      client,
	})
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}

	// Empty tenants
	got := w.BuildOrgIDHeader(0)
	if got != "" {
		t.Errorf("expected empty header for no tenants, got %q", got)
	}
}

func TestWatcher_Tenants(t *testing.T) {
	client := fake.NewSimpleClientset()

	w, err := NewWatcher(WatcherConfig{
		ClusterName: "test",
		Client:      client,
	})
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}

	// Set tenants
	w.mu.Lock()
	w.tenants = []string{"a", "b", "c"}
	w.mu.Unlock()

	// Get tenants
	tenants := w.Tenants()
	if len(tenants) != 3 {
		t.Fatalf("expected 3 tenants, got %d", len(tenants))
	}

	// Verify it's a copy (modifying shouldn't affect original)
	tenants[0] = "modified"
	original := w.Tenants()
	if original[0] == "modified" {
		t.Error("Tenants() should return a copy, not the original slice")
	}
}

func TestWatcher_TenantCount(t *testing.T) {
	client := fake.NewSimpleClientset()

	w, err := NewWatcher(WatcherConfig{
		ClusterName: "test",
		Client:      client,
	})
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}

	if w.TenantCount() != 0 {
		t.Errorf("expected 0 tenants initially, got %d", w.TenantCount())
	}

	w.mu.Lock()
	w.tenants = []string{"a", "b", "c"}
	w.mu.Unlock()

	if w.TenantCount() != 3 {
		t.Errorf("expected 3 tenants, got %d", w.TenantCount())
	}
}

func TestWatcher_RefreshTenants(t *testing.T) {
	// Create fake client with some namespaces
	client := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "game-prod"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "game-staging"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "observability"}},
	)

	w, err := NewWatcher(WatcherConfig{
		ClusterName:     "test",
		Client:          client,
		IncludePatterns: []string{"^game-.*"},
		ExcludePatterns: []string{".*-staging$"},
	})
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}

	// Start the watcher briefly to sync cache
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	w.informerFactory.Start(w.stopCh)
	syncMap := w.informerFactory.WaitForCacheSync(ctx.Done())
	for _, synced := range syncMap {
		if !synced {
			t.Fatal("failed to sync namespace cache")
		}
	}

	// Trigger refresh
	w.refreshTenants()

	// Check results
	tenants := w.Tenants()
	if len(tenants) != 1 {
		t.Fatalf("expected 1 tenant (game-prod), got %d: %v", len(tenants), tenants)
	}
	if tenants[0] != "game-prod" {
		t.Errorf("expected tenant 'game-prod', got %q", tenants[0])
	}

	w.Stop()
}

func TestWatcher_ListNamespaces(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app"}},
	)

	w, err := NewWatcher(WatcherConfig{
		ClusterName: "test",
		Client:      client,
	})
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}

	namespaces, err := w.ListNamespaces(context.Background())
	if err != nil {
		t.Fatalf("failed to list namespaces: %v", err)
	}

	if len(namespaces) != 3 {
		t.Fatalf("expected 3 namespaces, got %d", len(namespaces))
	}

	// Should be sorted
	expected := []string{"app", "default", "kube-system"}
	for i, ns := range namespaces {
		if ns != expected[i] {
			t.Errorf("namespace[%d] = %q, want %q", i, ns, expected[i])
		}
	}
}

func TestWatcher_TruncateOrgIDHeader(t *testing.T) {
	client := fake.NewSimpleClientset()

	w, err := NewWatcher(WatcherConfig{
		ClusterName: "test",
		Client:      client,
	})
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}

	tests := []struct {
		name      string
		tenants   []string
		maxLength int
		want      string
	}{
		{
			name:      "empty tenants",
			tenants:   []string{},
			maxLength: 100,
			want:      "",
		},
		{
			name:      "single tenant fits",
			tenants:   []string{"abc"},
			maxLength: 10,
			want:      "abc",
		},
		{
			name:      "single tenant truncated",
			tenants:   []string{"abcdefghij"},
			maxLength: 5,
			want:      "abcde",
		},
		{
			name:      "multiple tenants all fit",
			tenants:   []string{"a", "b", "c"},
			maxLength: 10,
			want:      "a|b|c",
		},
		{
			name:      "multiple tenants partial fit",
			tenants:   []string{"aaa", "bbb", "ccc"},
			maxLength: 7,
			want:      "aaa|bbb",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := w.truncateOrgIDHeader(tt.tenants, tt.maxLength)
			if got != tt.want {
				t.Errorf("truncateOrgIDHeader() = %q, want %q", got, tt.want)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsSubstring(s, substr)
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
