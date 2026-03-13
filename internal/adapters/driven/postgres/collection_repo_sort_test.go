package postgres_test

import (
	"testing"

	"github.com/gravity-zero/evey/internal/adapters/driven/postgres"
)

func TestSafeSortCol(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"title", "title"},
		{"sort_title", "sort_title"},
		{"added_at", "added_at"},
		{"year", "year"},
		{"premiere_date", "premiere_date"},
		{"community_rating", "community_rating"},
		{"TITLE", "title"},
		{"Year", "year"},
		{"", "sort_title"},
		{"1 OR 1=1", "sort_title"},
		{"title; DROP TABLE media_items;--", "sort_title"},
		{"(SELECT 1)", "sort_title"},
		{"unknown_column", "sort_title"},
		{"file_path", "sort_title"},
		{"id", "sort_title"},
	}
	for _, c := range cases {
		got := postgres.SafeSortCol(c.input)
		if got != c.want {
			t.Errorf("SafeSortCol(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestSafeSortDir(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"asc", "ASC"},
		{"desc", "DESC"},
		{"ASC", "ASC"},
		{"DESC", "DESC"},
		{"Asc", "ASC"},
		{"Desc", "DESC"},
		{"", "ASC"},
		{"ASC; DROP TABLE users;--", "ASC"},
		{"1 OR 1=1", "ASC"},
		{"random", "ASC"},
		{"nulls last", "ASC"},
	}
	for _, c := range cases {
		got := postgres.SafeSortDir(c.input)
		if got != c.want {
			t.Errorf("SafeSortDir(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}
