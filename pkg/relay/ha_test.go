package relay

import (
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/freitascorp/devopsclaw/pkg/fleet"
)

func haTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestNewHACoordinator(t *testing.T) {
	srv := NewWSServer(ServerConfig{}, fleet.NewMemoryStore(), haTestLogger())
	ha := NewHACoordinator(HAConfig{
		InstanceID:    "relay-1",
		PeerAddrs:     []string{"relay-2:9443", "relay-3:9443"},
		AdvertiseAddr: "relay-1:9443",
	}, srv, haTestLogger())

	if ha == nil {
		t.Fatal("expected non-nil HACoordinator")
	}
	if ha.config.HealthInterval != 10*time.Second {
		t.Errorf("HealthInterval = %v, want 10s", ha.config.HealthInterval)
	}
	if ha.config.DrainTimeout != 30*time.Second {
		t.Errorf("DrainTimeout = %v, want 30s", ha.config.DrainTimeout)
	}
	if ha.status != "active" {
		t.Errorf("initial status = %q, want active", ha.status)
	}
}

func TestHACoordinator_ClusterStatus(t *testing.T) {
	srv := NewWSServer(ServerConfig{}, fleet.NewMemoryStore(), haTestLogger())
	ha := NewHACoordinator(HAConfig{
		InstanceID:    "relay-1",
		AdvertiseAddr: "relay-1:9443",
	}, srv, haTestLogger())

	status := ha.ClusterStatus()
	if status.Self == nil {
		t.Fatal("expected non-nil Self")
	}
	if status.Self.InstanceID != "relay-1" {
		t.Errorf("Self.InstanceID = %q, want relay-1", status.Self.InstanceID)
	}
	if status.TotalInstances != 1 {
		t.Errorf("TotalInstances = %d, want 1", status.TotalInstances)
	}
	if status.HealthyInstances != 1 {
		t.Errorf("HealthyInstances = %d, want 1", status.HealthyInstances)
	}
}

func TestHACoordinator_PreferredInstance(t *testing.T) {
	srv := NewWSServer(ServerConfig{}, fleet.NewMemoryStore(), haTestLogger())
	ha := NewHACoordinator(HAConfig{
		InstanceID: "relay-1",
	}, srv, haTestLogger())

	// With no peers, should always return self
	pref := ha.PreferredInstance("any-node-id")
	if pref != "relay-1" {
		t.Errorf("PreferredInstance = %q, want relay-1 (only instance)", pref)
	}
}

func TestHACoordinator_PreferredInstance_WithPeers(t *testing.T) {
	srv := NewWSServer(ServerConfig{}, fleet.NewMemoryStore(), haTestLogger())
	ha := NewHACoordinator(HAConfig{
		InstanceID: "relay-1",
	}, srv, haTestLogger())

	// Add healthy peers
	ha.mu.Lock()
	ha.peers["relay-2"] = &PeerState{InstanceID: "relay-2", Status: "healthy"}
	ha.peers["relay-3"] = &PeerState{InstanceID: "relay-3", Status: "healthy"}
	ha.mu.Unlock()

	// Consistent hashing should distribute nodes across instances
	seen := map[string]int{}
	for i := 0; i < 300; i++ {
		nodeID := fmt.Sprintf("node-%d", i)
		pref := ha.PreferredInstance(nodeID)
		seen[pref]++
	}

	// With 3 instances and 300 nodes, each should get some
	if len(seen) < 2 {
		t.Errorf("expected distribution across instances, got %v", seen)
	}
}

func TestHACoordinator_ShouldAcceptNode(t *testing.T) {
	srv := NewWSServer(ServerConfig{}, fleet.NewMemoryStore(), haTestLogger())
	ha := NewHACoordinator(HAConfig{
		InstanceID: "relay-1",
	}, srv, haTestLogger())

	// With no peers, should always accept
	if !ha.ShouldAcceptNode("any-node") {
		t.Error("should accept node when no peers")
	}
}
