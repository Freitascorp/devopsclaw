package events

import "testing"

func TestDeviceEvent_FormatMessage_Add(t *testing.T) {
	ev := &DeviceEvent{
		Action:       ActionAdd,
		Kind:         KindUSB,
		DeviceID:     "1-2",
		Vendor:       "SanDisk",
		Product:      "Ultra USB",
		Serial:       "ABC123",
		Capabilities: "Mass Storage",
	}

	msg := ev.FormatMessage()

	if msg == "" {
		t.Fatal("expected non-empty message")
	}

	// Should contain "Connected"
	if !contains(msg, "Connected") {
		t.Error("expected 'Connected' in message")
	}
	if !contains(msg, "usb") {
		t.Error("expected 'usb' in message")
	}
	if !contains(msg, "SanDisk") {
		t.Error("expected vendor in message")
	}
	if !contains(msg, "Ultra USB") {
		t.Error("expected product in message")
	}
	if !contains(msg, "Mass Storage") {
		t.Error("expected capabilities in message")
	}
	if !contains(msg, "ABC123") {
		t.Error("expected serial in message")
	}
}

func TestDeviceEvent_FormatMessage_Remove(t *testing.T) {
	ev := &DeviceEvent{
		Action:  ActionRemove,
		Kind:    KindUSB,
		Vendor:  "Kingston",
		Product: "DataTraveler",
	}

	msg := ev.FormatMessage()

	if !contains(msg, "Disconnected") {
		t.Error("expected 'Disconnected' in message")
	}
	if !contains(msg, "Kingston") {
		t.Error("expected vendor in message")
	}
}

func TestDeviceEvent_FormatMessage_NoOptionalFields(t *testing.T) {
	ev := &DeviceEvent{
		Action:  ActionAdd,
		Kind:    KindGeneric,
		Vendor:  "Unknown",
		Product: "Device",
	}

	msg := ev.FormatMessage()

	// Should not contain "Capabilities:" or "Serial:" lines
	if contains(msg, "Capabilities:") {
		t.Error("should not contain capabilities when empty")
	}
	if contains(msg, "Serial:") {
		t.Error("should not contain serial when empty")
	}
}

func TestActionConstants(t *testing.T) {
	if ActionAdd != "add" {
		t.Errorf("expected 'add', got %s", ActionAdd)
	}
	if ActionRemove != "remove" {
		t.Errorf("expected 'remove', got %s", ActionRemove)
	}
	if ActionChange != "change" {
		t.Errorf("expected 'change', got %s", ActionChange)
	}
}

func TestKindConstants(t *testing.T) {
	if KindUSB != "usb" {
		t.Errorf("expected 'usb', got %s", KindUSB)
	}
	if KindBluetooth != "bluetooth" {
		t.Errorf("expected 'bluetooth', got %s", KindBluetooth)
	}
	if KindPCI != "pci" {
		t.Errorf("expected 'pci', got %s", KindPCI)
	}
	if KindGeneric != "generic" {
		t.Errorf("expected 'generic', got %s", KindGeneric)
	}
}

// helper
func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
