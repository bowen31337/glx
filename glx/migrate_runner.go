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
	"sort"

	glxlib "github.com/genealogix/glx/go-glx"
)

// MigrateReport summarizes the changes made by a migration run.
type MigrateReport struct {
	EventsCreated      int
	EventsMerged       int
	PropertiesRemoved  int
	AssertionsMigrated int
}

// migrateBirthDeathProperties converts deprecated born_on/born_at/died_on/died_at
// person properties into birth/death events. It modifies the archive in place.
func migrateBirthDeathProperties(archive *glxlib.GLXFile) (*MigrateReport, error) {
	report := &MigrateReport{}

	if archive.Events == nil {
		archive.Events = make(map[string]*glxlib.Event)
	}

	// Sort person IDs for deterministic output order.
	personIDs := make([]string, 0, len(archive.Persons))
	for id := range archive.Persons {
		personIDs = append(personIDs, id)
	}
	sort.Strings(personIDs)

	for _, personID := range personIDs {
		person := archive.Persons[personID]
		if person == nil || len(person.Properties) == 0 {
			continue
		}

		bornOn, hasBornOn := person.Properties[glxlib.DeprecatedPropertyBornOn]
		bornAt, hasBornAt := person.Properties[glxlib.DeprecatedPropertyBornAt]
		diedOn, hasDiedOn := person.Properties[glxlib.DeprecatedPropertyDiedOn]
		diedAt, hasDiedAt := person.Properties[glxlib.DeprecatedPropertyDiedAt]

		if !hasBornOn && !hasBornAt && !hasDiedOn && !hasDiedAt {
			continue
		}

		// Handle birth properties.
		if hasBornOn || hasBornAt {
			birthEventID, err := migrateEventProperties(
				archive, personID, glxlib.EventTypeBirth,
				bornOn, hasBornOn, bornAt, hasBornAt, report,
			)
			if err != nil {
				return nil, fmt.Errorf("person %s birth: %w", personID, err)
			}
			migrateAssertions(archive, personID, birthEventID,
				glxlib.DeprecatedPropertyBornOn, glxlib.DeprecatedPropertyBornAt, report)
		}

		// Handle death properties.
		if hasDiedOn || hasDiedAt {
			deathEventID, err := migrateEventProperties(
				archive, personID, glxlib.EventTypeDeath,
				diedOn, hasDiedOn, diedAt, hasDiedAt, report,
			)
			if err != nil {
				return nil, fmt.Errorf("person %s death: %w", personID, err)
			}
			migrateAssertions(archive, personID, deathEventID,
				glxlib.DeprecatedPropertyDiedOn, glxlib.DeprecatedPropertyDiedAt, report)
		}

		// Remove deprecated properties from the person.
		for _, prop := range []string{
			glxlib.DeprecatedPropertyBornOn,
			glxlib.DeprecatedPropertyBornAt,
			glxlib.DeprecatedPropertyDiedOn,
			glxlib.DeprecatedPropertyDiedAt,
		} {
			if _, exists := person.Properties[prop]; exists {
				delete(person.Properties, prop)
				report.PropertiesRemoved++
			}
		}
		if len(person.Properties) == 0 {
			person.Properties = nil
		}
	}

	return report, nil
}

// migrateEventProperties creates or merges a birth/death event for a person.
// Returns the event ID (existing or newly created).
func migrateEventProperties(
	archive *glxlib.GLXFile,
	personID, eventType string,
	dateVal any, hasDate bool,
	placeVal any, hasPlace bool,
	report *MigrateReport,
) (string, error) {
	eventID, existing := glxlib.FindPersonEvent(archive, personID, eventType)

	if existing != nil {
		// Merge: fill in missing fields only.
		merged := false
		if hasDate && existing.Date == "" {
			if dateStr, ok := dateVal.(string); ok && dateStr != "" {
				existing.Date = glxlib.DateString(dateStr)
				merged = true
			}
		}
		if hasPlace && existing.PlaceID == "" {
			if placeStr, ok := placeVal.(string); ok && placeStr != "" {
				existing.PlaceID = placeStr
				merged = true
			}
		}
		if merged {
			report.EventsMerged++
		}
		return eventID, nil
	}

	// Create a new event.
	newID, err := glxlib.GenerateRandomID()
	if err != nil {
		return "", fmt.Errorf("generating event ID: %w", err)
	}
	eventID = "event-" + newID

	event := &glxlib.Event{
		Type: eventType,
		Participants: []glxlib.Participant{
			{Person: personID, Role: glxlib.ParticipantRolePrincipal},
		},
	}

	if hasDate {
		if dateStr, ok := dateVal.(string); ok && dateStr != "" {
			event.Date = glxlib.DateString(dateStr)
		}
	}
	if hasPlace {
		if placeStr, ok := placeVal.(string); ok && placeStr != "" {
			event.PlaceID = placeStr
		}
	}

	archive.Events[eventID] = event
	report.EventsCreated++

	return eventID, nil
}

// migrateAssertions converts assertions that reference deprecated person properties
// to reference the corresponding event instead.
func migrateAssertions(
	archive *glxlib.GLXFile,
	personID, eventID string,
	dateProperty, placeProperty string,
	report *MigrateReport,
) {
	for _, assertion := range archive.Assertions {
		if assertion == nil || assertion.Subject.Person != personID {
			continue
		}

		switch assertion.Property {
		case dateProperty:
			assertion.Subject = glxlib.EntityRef{Event: eventID}
			assertion.Property = "date"
			report.AssertionsMigrated++
		case placeProperty:
			assertion.Subject = glxlib.EntityRef{Event: eventID}
			assertion.Property = "place"
			report.AssertionsMigrated++
		}
	}
}
