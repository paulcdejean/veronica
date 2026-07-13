package gcp

import (
	"maps"
	"testing"
)

func TestParseAllowlist(t *testing.T) {
	items := []metadataItem{
		{Key: "voice-contact-plain", Value: "+15125551234"},
		{Key: "voice-contact-spaced", Value: " +1 512 555 4321 "},
		{Key: "voice-contact-empty", Value: ""},
		{Key: "voice-contact-words", Value: "call me maybe"},
		{Key: "voice-contact-no-plus", Value: "15125559999"},
		{Key: "voice-contact-leading-zero", Value: "+05125559999"},
		{Key: "ssh-keys", Value: "+15125550000"},
	}
	want := map[string]bool{
		"+15125551234": true,
		"+15125554321": true,
	}
	if got := parseAllowlist(items); !maps.Equal(got, want) {
		t.Errorf("parseAllowlist() = %v, want %v", got, want)
	}
}
