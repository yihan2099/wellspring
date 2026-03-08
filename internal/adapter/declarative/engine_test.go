package declarative_test

import (
	"testing"

	"github.com/wellspring-cli/wellspring/internal/adapter/declarative"
)

func TestLoadFromBytes(t *testing.T) {
	yaml := []byte(`
name: test
category: news
description: "Test adapter"
auth: none
base_url: https://example.com/api

endpoints:
  list:
    path: /items.json
    method: GET
  item:
    path: /item/{id}.json
    method: GET

mapping:
  title: .title
  url: .url
  value: .score
  time: .time | unix
  meta:
    author: .by

rate_limit:
  requests: 10
  per: 1m
`)

	a, err := declarative.LoadFromBytes(yaml)
	if err != nil {
		t.Fatalf("LoadFromBytes failed: %v", err)
	}

	if a.Name() != "test" {
		t.Errorf("expected name 'test', got %q", a.Name())
	}
	if a.Category() != "news" {
		t.Errorf("expected category 'news', got %q", a.Category())
	}
	if a.RequiresAuth() {
		t.Error("expected no auth required")
	}
	if a.Description() != "Test adapter" {
		t.Errorf("expected description 'Test adapter', got %q", a.Description())
	}

	endpoints := a.Endpoints()
	if len(endpoints) != 2 {
		t.Errorf("expected 2 endpoints, got %d", len(endpoints))
	}

	rl := a.RateLimit()
	if rl.Requests != 10 {
		t.Errorf("expected 10 requests, got %d", rl.Requests)
	}
}

func TestLoadFromBytesInvalid(t *testing.T) {
	tests := []struct {
		name string
		yaml []byte
	}{
		{"empty name", []byte(`name: ""
base_url: https://example.com`)},
		{"empty base_url", []byte(`name: test
base_url: ""`)},
		{"invalid yaml", []byte(`{{{invalid`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := declarative.LoadFromBytes(tt.yaml)
			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestLoadFromBytesAuth(t *testing.T) {
	yaml := []byte(`
name: authed
category: finance
auth: apiKey
base_url: https://example.com
endpoints:
  list:
    path: /list
`)

	a, err := declarative.LoadFromBytes(yaml)
	if err != nil {
		t.Fatalf("LoadFromBytes failed: %v", err)
	}

	if !a.RequiresAuth() {
		t.Error("expected auth required")
	}
}
