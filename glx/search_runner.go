// Copyright 2025 Oracynth, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	glxlib "github.com/genealogix/glx/go-glx"
)

// searchResult represents a single search match.
type searchResult struct {
	EntityType string // "persons", "events", etc.
	EntityID   string
	Field      string // which field matched
	Value      string // the matching value (truncated for display)
}

// searchArchive searches all entities for the given query string.
func searchArchive(archive *glxlib.GLXFile, query string, caseSensitive bool) []searchResult {
	var results []searchResult

	matchFn := containsMatch(query, caseSensitive)

	// Persons
	for _, id := range sortedKeys(archive.Persons) {
		person := archive.Persons[id]
		if person == nil {
			continue
		}
		if matchFn(id) {
			results = append(results, searchResult{"persons", id, "id", id})
		}
		for key, val := range person.Properties {
			if s := fmt.Sprint(val); matchFn(s) {
				results = append(results, searchResult{"persons", id, key, truncate(s, 80)})
			}
		}
		if matchFn(person.Notes) {
			results = append(results, searchResult{"persons", id, "notes", truncate(person.Notes, 80)})
		}
	}

	// Events
	for _, id := range sortedKeys(archive.Events) {
		ev := archive.Events[id]
		if ev == nil {
			continue
		}
		if matchFn(id) {
			results = append(results, searchResult{"events", id, "id", id})
		}
		if matchFn(ev.Title) {
			results = append(results, searchResult{"events", id, "title", ev.Title})
		}
		if matchFn(ev.Type) {
			results = append(results, searchResult{"events", id, "type", ev.Type})
		}
		if matchFn(string(ev.Date)) {
			results = append(results, searchResult{"events", id, "date", string(ev.Date)})
		}
		if matchFn(ev.Notes) {
			results = append(results, searchResult{"events", id, "notes", truncate(ev.Notes, 80)})
		}
	}

	// Places
	for _, id := range sortedKeys(archive.Places) {
		place := archive.Places[id]
		if place == nil {
			continue
		}
		if matchFn(id) {
			results = append(results, searchResult{"places", id, "id", id})
		}
		if matchFn(place.Name) {
			results = append(results, searchResult{"places", id, "name", place.Name})
		}
		if matchFn(place.Notes) {
			results = append(results, searchResult{"places", id, "notes", truncate(place.Notes, 80)})
		}
	}

	// Sources
	for _, id := range sortedKeys(archive.Sources) {
		src := archive.Sources[id]
		if src == nil {
			continue
		}
		if matchFn(id) {
			results = append(results, searchResult{"sources", id, "id", id})
		}
		if matchFn(src.Title) {
			results = append(results, searchResult{"sources", id, "title", src.Title})
		}
		if matchFn(src.Description) {
			results = append(results, searchResult{"sources", id, "description", truncate(src.Description, 80)})
		}
	}

	// Citations
	for _, id := range sortedKeys(archive.Citations) {
		cit := archive.Citations[id]
		if cit == nil {
			continue
		}
		if matchFn(id) {
			results = append(results, searchResult{"citations", id, "id", id})
		}
		for key, val := range cit.Properties {
			if s := fmt.Sprint(val); matchFn(s) {
				results = append(results, searchResult{"citations", id, key, truncate(s, 80)})
			}
		}
	}

	// Repositories
	for _, id := range sortedKeys(archive.Repositories) {
		repo := archive.Repositories[id]
		if repo == nil {
			continue
		}
		if matchFn(id) {
			results = append(results, searchResult{"repositories", id, "id", id})
		}
		if matchFn(repo.Name) {
			results = append(results, searchResult{"repositories", id, "name", repo.Name})
		}
	}

	// Assertions
	for _, id := range sortedKeys(archive.Assertions) {
		a := archive.Assertions[id]
		if a == nil {
			continue
		}
		if matchFn(id) {
			results = append(results, searchResult{"assertions", id, "id", id})
		}
		if matchFn(a.Property) {
			results = append(results, searchResult{"assertions", id, "property", a.Property})
		}
		if matchFn(a.Value) {
			results = append(results, searchResult{"assertions", id, "value", a.Value})
		}
		if matchFn(a.Notes) {
			results = append(results, searchResult{"assertions", id, "notes", truncate(a.Notes, 80)})
		}
	}

	// Relationships
	for _, id := range sortedKeys(archive.Relationships) {
		rel := archive.Relationships[id]
		if rel == nil {
			continue
		}
		if matchFn(id) {
			results = append(results, searchResult{"relationships", id, "id", id})
		}
		if matchFn(rel.Notes) {
			results = append(results, searchResult{"relationships", id, "notes", truncate(rel.Notes, 80)})
		}
	}

	// Media
	for _, id := range sortedKeys(archive.Media) {
		m := archive.Media[id]
		if m == nil {
			continue
		}
		if matchFn(id) {
			results = append(results, searchResult{"media", id, "id", id})
		}
		if matchFn(m.Title) {
			results = append(results, searchResult{"media", id, "title", m.Title})
		}
		if matchFn(m.Description) {
			results = append(results, searchResult{"media", id, "description", truncate(m.Description, 80)})
		}
	}

	return deduplicateResults(results)
}

// containsMatch returns a match function for the given query.
func containsMatch(query string, caseSensitive bool) func(string) bool {
	if caseSensitive {
		return func(s string) bool {
			return s != "" && strings.Contains(s, query)
		}
	}
	lowerQuery := strings.ToLower(query)
	return func(s string) bool {
		return s != "" && strings.Contains(strings.ToLower(s), lowerQuery)
	}
}

// truncate shortens a string to maxLen characters with "..." suffix.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// deduplicateResults removes duplicate (entityType, entityID) pairs,
// keeping the first match per entity.
func deduplicateResults(results []searchResult) []searchResult {
	seen := make(map[string]bool)
	var deduped []searchResult
	for _, r := range results {
		key := r.EntityType + ":" + r.EntityID
		if seen[key] {
			continue
		}
		seen[key] = true
		deduped = append(deduped, r)
	}
	return deduped
}

// showSearch loads an archive and performs a full-text search.
func showSearch(archivePath, query string, caseSensitive bool, typeFilter string) error {
	info, err := os.Stat(archivePath)
	if err != nil {
		return fmt.Errorf("cannot access path: %w", err)
	}

	var archive *glxlib.GLXFile
	if info.IsDir() {
		loaded, duplicates, loadErr := LoadArchiveWithOptions(archivePath, false)
		if loadErr != nil {
			return fmt.Errorf("failed to load archive: %w", loadErr)
		}
		for _, d := range duplicates {
			fmt.Fprintf(os.Stderr, "Warning: %s\n", d)
		}
		archive = loaded
	} else {
		loaded, loadErr := readSingleFileArchive(archivePath, false)
		if loadErr != nil {
			return loadErr
		}
		archive = loaded
	}

	results := searchArchive(archive, query, caseSensitive)

	// Filter by type if specified
	if typeFilter != "" {
		var filtered []searchResult
		for _, r := range results {
			if r.EntityType == typeFilter {
				filtered = append(filtered, r)
			}
		}
		results = filtered
	}

	if len(results) == 0 {
		fmt.Printf("No matches found for %q\n", query)
		return nil
	}

	// Group by entity type
	groups := make(map[string][]searchResult)
	for _, r := range results {
		groups[r.EntityType] = append(groups[r.EntityType], r)
	}

	typeOrder := []string{"persons", "events", "places", "sources", "citations", "repositories", "assertions", "relationships", "media"}

	fmt.Printf("Found %d match(es) for %q:\n", len(results), query)

	for _, typ := range typeOrder {
		group, ok := groups[typ]
		if !ok {
			continue
		}

		sort.Slice(group, func(i, j int) bool {
			return group[i].EntityID < group[j].EntityID
		})

		fmt.Printf("\n  %s (%d):\n", strings.Title(typ), len(group))
		for _, r := range group {
			fmt.Printf("    %s  %s: %s\n", r.EntityID, r.Field, r.Value)
		}
	}

	return nil
}
