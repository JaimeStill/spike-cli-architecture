package files_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/standards-lab/blobfs"

	"github.com/JaimeStill/spike-cli-architecture/domain/files"
)

func TestParseRef(t *testing.T) {
	tests := []struct {
		arg     string
		want    files.Ref
		wantErr error
	}{
		{"/reports", files.Ref{Path: "/reports"}, nil},
		{"reports", files.Ref{Path: "reports"}, nil},
		{"id:00000000-0000-7000-8000-00000000000A", files.Ref{ID: "00000000-0000-7000-8000-00000000000a"}, nil},
		{"id:nope", files.Ref{}, blobfs.ErrInvalidID},
		{"id:" + blobfs.RootID, files.Ref{}, blobfs.ErrInvalidID},
	}
	for _, tt := range tests {
		t.Run(tt.arg, func(t *testing.T) {
			got, err := files.ParseRef(tt.arg)

			if !errors.Is(err, tt.wantErr) || (tt.wantErr == nil && err != nil) {
				t.Fatalf("ParseRef(%q) error = %v, want %v", tt.arg, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ParseRef(%q) = %+v, want %+v", tt.arg, got, tt.want)
			}
		})
	}
}

func TestParseFilter(t *testing.T) {
	tests := []struct {
		term string
		want files.Filter
	}{
		{"name:eq:a.txt", files.Filter{Field: "name", Op: "eq", Value: "a.txt"}},
		{"created_at:ge:2026-01-01T00:00:00Z", files.Filter{Field: "created_at", Op: "ge", Value: "2026-01-01T00:00:00Z"}},
		{"etag:null", files.Filter{Field: "etag", Op: "null"}},
		{"status:in:available,pending", files.Filter{Field: "status", Op: "in", Value: []any{"available", "pending"}}},
	}
	for _, tt := range tests {
		t.Run(tt.term, func(t *testing.T) {
			got, err := files.ParseFilter(tt.term)

			if err != nil {
				t.Fatalf("ParseFilter(%q) = %v", tt.term, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseFilter(%q) = %+v, want %+v", tt.term, got, tt.want)
			}
		})
	}
	for _, term := range []string{"name", ":eq:x", "name::x", "name:eq", "etag:null:x", "status:in"} {
		if _, err := files.ParseFilter(term); err == nil {
			t.Errorf("ParseFilter(%q) = nil, want a refusal", term)
		}
	}
}

func TestParseSort(t *testing.T) {
	tests := []struct {
		term string
		want files.Sort
	}{
		{"name", files.Sort{Field: "name"}},
		{"name:asc", files.Sort{Field: "name"}},
		{"size:desc", files.Sort{Field: "size", Descending: true}},
	}
	for _, tt := range tests {
		got, err := files.ParseSort(tt.term)
		if err != nil || got != tt.want {
			t.Errorf("ParseSort(%q) = %+v, %v, want %+v", tt.term, got, err, tt.want)
		}
	}
	for _, term := range []string{"", ":desc", "name:up"} {
		if _, err := files.ParseSort(term); err == nil {
			t.Errorf("ParseSort(%q) = nil, want a refusal", term)
		}
	}
}
