# Remove born_on/born_at/died_on/died_at Properties — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the four redundant person properties `born_on`, `born_at`, `died_on`, `died_at` and make birth/death Event entities the single source of truth.

**Architecture:** Add a `FindPersonEvent` helper in `go-glx/` that locates a person's birth/death event by scanning participants. Update all consumers (20+ files across `go-glx/` and `glx/`) to use this helper instead of reading person properties. Add a `glx migrate` command for existing archives.

**Tech Stack:** Go, YAML (gopkg.in/yaml.v3), Cobra CLI

**References:**
- Design spec: `docs/superpowers/specs/2026-03-29-remove-birth-death-properties-design.md`
- Issue: #345

---

## File Structure

**New files:**
- `go-glx/event_lookup.go` — `FindPersonEvent` helper
- `go-glx/event_lookup_test.go` — tests for the helper
- `glx/cmd_migrate.go` — migration CLI command
- `glx/migrate_runner.go` — migration logic
- `glx/migrate_runner_test.go` — migration tests

**Modified files (library — `go-glx/`):**
- `constants.go` — rename 4 constants to `Deprecated*`
- `validation.go` — add banned property error
- `validation_temporal.go` — rewrite to use event dates
- `validation_temporal_test.go` — update test fixtures
- `gedcom_individual.go` — remove property setting, convert to event assertions
- `gedcom_evidence.go` — add `createEventAssertion` function
- `gedcom_export_person.go` — update skip map
- `duplicates.go` — use FindPersonEvent for scoring
- `diff.go` — use FindPersonEvent for disambiguation
- `census.go` — create event-targeted assertions
- `rename.go` — remove born_at/died_at from property rename comment (handled by event PlaceID)

**Modified files (CLI — `glx/`):**
- `timeline_runner.go` — remove property synthesis, use events only
- `vitals_runner.go` — remove property fallback, use events only
- `summary_runner.go` — remove skip list entries
- `analyze_gaps.go` — check for events instead of properties
- `analyze_suggestions.go` — extract year from events
- `analyze_consistency.go` — use event dates for all checks
- `analyze_child_census.go` — use event dates
- `query_runner.go` — birthplace filter and display from events
- `coverage_runner.go` — read from events
- `coverage_state_census.go` — use event PlaceID
- `places_runner.go` — remove born_at/died_at from place tracking
- `tree_runner.go` — use event dates for labels
- `tree_suggestions.go` — use event dates/places
- `census_runner.go` — update assertion property check
- `duplicates_runner.go` — use event dates

**Modified files (spec/examples):**
- `specification/5-standard-vocabularies/person-properties.glx` — remove 4 entries
- `specification/schema/v1/` — remove if referenced
- `specification/4-entity-types/person.md` — update docs
- 9 example archive files — remove properties, ensure events exist
- `CHANGELOG.md`

---

### Task 1: Add FindPersonEvent Helper

**Files:**
- Create: `go-glx/event_lookup.go`
- Create: `go-glx/event_lookup_test.go`

- [ ] **Step 1: Write the failing test**

```go
// go-glx/event_lookup_test.go
package glx

import "testing"

func TestFindPersonEvent(t *testing.T) {
	archive := &GLXFile{
		Events: map[string]*Event{
			"event-birth-alice": {
				Type: EventTypeBirth,
				Date: "1850-03-15",
				Participants: []Participant{
					{Person: "person-alice", Role: ParticipantRolePrincipal},
				},
			},
			"event-birth-bob": {
				Type: EventTypeBirth,
				Date: "1855-07-20",
				Participants: []Participant{
					{Person: "person-bob", Role: ParticipantRolePrincipal},
					{Person: "person-alice", Role: ParticipantRoleWitness},
				},
			},
			"event-death-alice": {
				Type: EventTypeDeath,
				Date: "1920-11-01",
				PlaceID: "place-london",
				Participants: []Participant{
					{Person: "person-alice", Role: ParticipantRolePrincipal},
				},
			},
		},
	}

	t.Run("finds birth event for principal", func(t *testing.T) {
		id, event := FindPersonEvent(archive, "person-alice", EventTypeBirth)
		if event == nil {
			t.Fatal("expected to find birth event for alice")
		}
		if id != "event-birth-alice" {
			t.Errorf("got id %q, want %q", id, "event-birth-alice")
		}
		if string(event.Date) != "1850-03-15" {
			t.Errorf("got date %q, want %q", event.Date, "1850-03-15")
		}
	})

	t.Run("does not match witness role", func(t *testing.T) {
		// Alice is a witness to Bob's birth, not the subject
		id, event := FindPersonEvent(archive, "person-alice", EventTypeBirth)
		if id == "event-birth-bob" {
			t.Error("should not match event where person is witness")
		}
		if event != nil && id == "event-birth-bob" {
			t.Error("returned wrong event")
		}
	})

	t.Run("finds death event", func(t *testing.T) {
		id, event := FindPersonEvent(archive, "person-alice", EventTypeDeath)
		if event == nil {
			t.Fatal("expected to find death event for alice")
		}
		if id != "event-death-alice" {
			t.Errorf("got id %q, want %q", id, "event-death-alice")
		}
		if event.PlaceID != "place-london" {
			t.Errorf("got place %q, want %q", event.PlaceID, "place-london")
		}
	})

	t.Run("returns nil for missing person", func(t *testing.T) {
		id, event := FindPersonEvent(archive, "person-unknown", EventTypeBirth)
		if event != nil {
			t.Errorf("expected nil, got event %q", id)
		}
	})

	t.Run("returns nil for missing event type", func(t *testing.T) {
		id, event := FindPersonEvent(archive, "person-bob", EventTypeDeath)
		if event != nil {
			t.Errorf("expected nil, got event %q", id)
		}
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `make test`
Expected: FAIL — `FindPersonEvent` not defined

- [ ] **Step 3: Write minimal implementation**

```go
// go-glx/event_lookup.go
package glx

// FindPersonEvent finds the first event of the given type where the specified
// person is a principal participant (not a witness, informant, or other role).
// Returns the event ID and the event, or ("", nil) if not found.
func FindPersonEvent(archive *GLXFile, personID, eventType string) (string, *Event) {
	for id, event := range archive.Events {
		if event == nil || event.Type != eventType {
			continue
		}
		for _, p := range event.Participants {
			if p.Person == personID && isSubjectRole(p.Role) {
				return id, event
			}
		}
	}
	return "", nil
}

// isSubjectRole returns true for participant roles that indicate the person
// is the subject of the event (their own birth, death, etc.) rather than
// a witness, informant, or other auxiliary role.
func isSubjectRole(role string) bool {
	switch role {
	case ParticipantRolePrincipal, "subject", "":
		return true
	default:
		return false
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `make test`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add go-glx/event_lookup.go go-glx/event_lookup_test.go
git commit -m "feat: Add FindPersonEvent helper for event-based lookups"
```

---

### Task 2: Rewrite Temporal Validation to Use Events

**Files:**
- Modify: `go-glx/validation_temporal.go`
- Modify: `go-glx/validation_temporal_test.go`

This is the most critical consumer — without this change, died-before-born and parent-child age checks silently pass for everyone.

- [ ] **Step 1: Update test fixtures to use events instead of properties**

In `go-glx/validation_temporal_test.go`, update all test cases. The tests currently set `person.Properties["born_on"]` etc. Change them to create Event entities with the person as principal participant.

Example — the death-before-birth test currently looks like:
```go
Persons: map[string]*Person{
    "person-1": {Properties: map[string]any{"born_on": "1850", "died_on": "1820"}},
},
```

Change to:
```go
Persons: map[string]*Person{
    "person-1": {Properties: map[string]any{}},
},
Events: map[string]*Event{
    "event-birth-1": {
        Type: EventTypeBirth, Date: "1850",
        Participants: []Participant{{Person: "person-1", Role: ParticipantRolePrincipal}},
    },
    "event-death-1": {
        Type: EventTypeDeath, Date: "1820",
        Participants: []Participant{{Person: "person-1", Role: ParticipantRolePrincipal}},
    },
},
```

Apply this pattern to ALL test cases in the file. Each test that sets `born_on`/`died_on` properties must be converted to use birth/death events.

- [ ] **Step 2: Run tests to verify they fail**

Run: `make test`
Expected: FAIL — validation still reads from properties, events are ignored

- [ ] **Step 3: Rewrite validation functions to use FindPersonEvent**

In `go-glx/validation_temporal.go`, update all three validation functions:

**`validateDeathBeforeBirth`** — replace:
```go
birthYear := ExtractPropertyYear(person.Properties, PersonPropertyBornOn)
deathYear := ExtractPropertyYear(person.Properties, PersonPropertyDiedOn)
```
with:
```go
birthYear := extractEventYear(glx, id, EventTypeBirth)
deathYear := extractEventYear(glx, id, EventTypeDeath)
```

**`validateParentChildAges`** — replace all `ExtractPropertyYear(parent.Properties, PersonPropertyBornOn)` and `ExtractPropertyYear(child.Properties, PersonPropertyBornOn)` calls with `extractEventYear(glx, parentID, EventTypeBirth)` and `extractEventYear(glx, childID, EventTypeBirth)`.

**`validateMarriageBeforeBirth`** — same pattern.

Add a private helper at the top of the file:
```go
func extractEventYear(archive *GLXFile, personID, eventType string) int {
	_, event := FindPersonEvent(archive, personID, eventType)
	if event == nil {
		return 0
	}
	return ExtractFirstYear(string(event.Date))
}
```

Also update the `Field` values in `ValidationWarning` from `PersonPropertyDiedOn` to `"death"` (since the data source is now the event, not the property).

- [ ] **Step 4: Run tests to verify they pass**

Run: `make test`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add go-glx/validation_temporal.go go-glx/validation_temporal_test.go
git commit -m "refactor: Temporal validation uses event dates instead of person properties"
```

---

### Task 3: Update Library Consumers (duplicates, diff, census, export)

**Files:**
- Modify: `go-glx/duplicates.go`
- Modify: `go-glx/diff.go`
- Modify: `go-glx/census.go`
- Modify: `go-glx/gedcom_export_person.go`

- [ ] **Step 1: Update duplicates.go**

In `go-glx/duplicates.go`, the `scorePair` function (around line 280) reads all four property constants. Update `scoreYearSimilarity` and `scorePlaceSimilarity` calls to use events.

Replace the four scoring blocks:
```go
byScore, byDetail := scoreYearSimilarity(propsA, propsB, PersonPropertyBornOn)
```
with event-based lookups. The `scorePair` function receives the archive — add it as a parameter if not already present, or use `FindPersonEvent` to extract years/places:

```go
// Birth year
_, birthA := FindPersonEvent(archive, idA, EventTypeBirth)
_, birthB := FindPersonEvent(archive, idB, EventTypeBirth)
byScore, byDetail := scoreEventYearSimilarity(birthA, birthB)
signals = append(signals, DuplicateSignal{"Birth year", weightBirthYear, byScore, byDetail})
totalScore += weightBirthYear * byScore

// Birth place
bpScore, bpDetail := scoreEventPlaceSimilarity(birthA, birthB, archive)
signals = append(signals, DuplicateSignal{"Birth place", weightBirthPlace, bpScore, bpDetail})
totalScore += weightBirthPlace * bpScore

// Death year
_, deathA := FindPersonEvent(archive, idA, EventTypeDeath)
_, deathB := FindPersonEvent(archive, idB, EventTypeDeath)
dyScore, dyDetail := scoreEventYearSimilarity(deathA, deathB)
signals = append(signals, DuplicateSignal{"Death year", weightDeathYear, dyScore, dyDetail})
totalScore += weightDeathYear * dyScore

// Death place
dpScore, dpDetail := scoreEventPlaceSimilarity(deathA, deathB, archive)
signals = append(signals, DuplicateSignal{"Death place", weightDeathPlace, dpScore, dpDetail})
totalScore += weightDeathPlace * dpScore
```

Add helper functions:
```go
func scoreEventYearSimilarity(eventA, eventB *Event) (float64, string) {
	yearA, yearB := 0, 0
	if eventA != nil {
		yearA = ExtractFirstYear(string(eventA.Date))
	}
	if eventB != nil {
		yearB = ExtractFirstYear(string(eventB.Date))
	}
	if yearA == 0 || yearB == 0 {
		return 0, "missing"
	}
	diff := yearA - yearB
	if diff < 0 {
		diff = -diff
	}
	if diff == 0 {
		return 1.0, fmt.Sprintf("exact match (%d)", yearA)
	}
	if diff <= 2 {
		return 0.7, fmt.Sprintf("%d vs %d (±%d)", yearA, yearB, diff)
	}
	return 0, fmt.Sprintf("%d vs %d (±%d)", yearA, yearB, diff)
}

func scoreEventPlaceSimilarity(eventA, eventB *Event, archive *GLXFile) (float64, string) {
	placeA, placeB := "", ""
	if eventA != nil {
		placeA = eventA.PlaceID
	}
	if eventB != nil {
		placeB = eventB.PlaceID
	}
	if placeA == "" || placeB == "" {
		return 0, "missing"
	}
	if placeA == placeB {
		return 1.0, "exact match"
	}
	return 0, "different"
}
```

Note: check existing `scoreYearSimilarity` and `scorePlaceSimilarity` signatures for the exact scoring logic and match it. The above is a template — match the existing scoring thresholds and detail format.

- [ ] **Step 2: Update diff.go**

In `go-glx/diff.go` around line 340, the `personLabel` function reads `born_on`/`died_on` from properties:
```go
if born, ok := props["born_on"].(string); ok && born != "" {
    parts = append(parts, "b. "+born)
}
if died, ok := props["died_on"].(string); ok && died != "" {
    parts = append(parts, "d. "+died)
}
```

The function needs access to the archive to call `FindPersonEvent`. Check the function signature — it likely takes `(id string, props map[string]any)`. Add an `archive *GLXFile` parameter and update the call site.

Replace:
```go
if born, ok := props["born_on"].(string); ok && born != "" {
    parts = append(parts, "b. "+born)
}
if died, ok := props["died_on"].(string); ok && died != "" {
    parts = append(parts, "d. "+died)
}
```
with:
```go
if archive != nil {
    if _, birthEvent := FindPersonEvent(archive, id, EventTypeBirth); birthEvent != nil && birthEvent.Date != "" {
        parts = append(parts, "b. "+string(birthEvent.Date))
    }
    if _, deathEvent := FindPersonEvent(archive, id, EventTypeDeath); deathEvent != nil && deathEvent.Date != "" {
        parts = append(parts, "d. "+string(deathEvent.Date))
    }
}
```

- [ ] **Step 3: Update census.go**

In `go-glx/census.go` around lines 466-506, the `AddCensus` function creates assertions with `Subject: EntityRef{Person: personID}` and `Property: PersonPropertyBornOn`/`PersonPropertyBornAt`. These must target events instead.

For birth year assertions (from age estimation):
```go
// Before: assertion targets person property
result.Assertions[assertionID] = &Assertion{
    Subject:    EntityRef{Person: personID},
    Property:   PersonPropertyBornOn,
    Value:      fmt.Sprintf("ABT %d", birthYear),
    ...
}
```

After: find or create a birth event and target it:
```go
birthEventID, _ := FindPersonEvent(existing, personID, EventTypeBirth)
if birthEventID == "" {
    // Check in the current batch too
    for eid, evt := range result.Events {
        if evt.Type == EventTypeBirth {
            for _, p := range evt.Participants {
                if p.Person == personID && isSubjectRole(p.Role) {
                    birthEventID = eid
                    break
                }
            }
        }
        if birthEventID != "" {
            break
        }
    }
}
if birthEventID == "" {
    // Create a new birth event
    birthEventID = generateEventID(fmt.Sprintf("event-birth-%s", pidSlug), existing, result)
    result.Events[birthEventID] = &Event{
        Type:  EventTypeBirth,
        Title: "Birth",
        Participants: []Participant{
            {Person: personID, Role: ParticipantRolePrincipal},
        },
    }
}
result.Assertions[assertionID] = &Assertion{
    Subject:    EntityRef{Event: birthEventID},
    Property:   "date",
    Value:      fmt.Sprintf("ABT %d", birthYear),
    Citations:  []string{citationID},
    Confidence: ConfidenceLevelLow,
    Notes:      fmt.Sprintf("Estimated from age %d in %d census.", *member.Age, census.Year),
}
```

Apply the same pattern for birthplace assertions — target the birth event with `Property: "place"` and `Value: birthplaceRef`.

Note: Check whether `generateEventID` exists or if there's a different ID generation pattern. Match the existing pattern used elsewhere for event ID generation.

- [ ] **Step 4: Update gedcom_export_person.go**

In `go-glx/gedcom_export_person.go`, the `skipPersonProperties` map references the four constants. After the properties are removed, persons won't have these properties, so the skip entries are unnecessary. However, to be safe during migration, keep the entries but use the renamed constants:

```go
var skipPersonProperties = map[string]bool{
    PersonPropertyName:       true,
    PersonPropertyGender:     true,
    DeprecatedPropertyBornOn: true,
    DeprecatedPropertyBornAt: true,
    DeprecatedPropertyDiedOn: true,
    DeprecatedPropertyDiedAt: true,
    PersonPropertyResidence:  true,
    PropertyNotes:            true,
    PropertyMedia:            true,
    PropertySources:          true,
    PropertyCitations:        true,
}
```

Note: This step must be done AFTER Task 7 (constant rename). If doing tasks in order, use the old constant names for now and update during Task 7.

- [ ] **Step 5: Run tests**

Run: `make test`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add go-glx/duplicates.go go-glx/diff.go go-glx/census.go go-glx/gedcom_export_person.go
git commit -m "refactor: Library consumers use event lookups instead of born/died properties"
```

---

### Task 4: Update GEDCOM Importer

**Files:**
- Modify: `go-glx/gedcom_individual.go`
- Modify: `go-glx/gedcom_evidence.go`

- [ ] **Step 1: Add createEventAssertion function**

In `go-glx/gedcom_evidence.go`, add a function parallel to `createPropertyAssertion` but targeting events:

```go
func createEventAssertion(eventID, property string, value any, sourceRecord *GEDCOMRecord, conv *ConversionContext) {
	if property == "" || value == nil {
		return
	}

	refs := extractEvidence(sourceRecord, conv)
	createEventAssertionWithEvidence(eventID, property, value, refs, conv)
}

func createEventAssertionWithEvidence(eventID, property string, value any, refs evidenceRefs, conv *ConversionContext) {
	if property == "" || value == nil {
		return
	}

	if !refs.hasEvidence() {
		return
	}

	assertionID := generateAssertionID(conv)

	var valueStr string
	switch v := value.(type) {
	case string:
		valueStr = v
	case int:
		valueStr = strconv.Itoa(v)
	case float64:
		valueStr = fmt.Sprintf("%f", v)
	default:
		valueStr = fmt.Sprintf("%v", v)
	}

	assertion := &Assertion{
		Subject:   EntityRef{Event: eventID},
		Property:  property,
		Value:     valueStr,
		Sources:   refs.SourceIDs,
		Citations: refs.CitationIDs,
	}

	conv.GLX.Assertions[assertionID] = assertion
	conv.Stats.AssertionsCreated++
}
```

- [ ] **Step 2: Update convertIndividualEvent to create event assertions instead of property assertions**

In `go-glx/gedcom_individual.go`, replace the block at lines 295-311:

```go
// OLD: Set person properties and create property assertions
if eventType == EventTypeBirth && eventDate != "" {
    person.Properties[PersonPropertyBornOn] = eventDate
    createPropertyAssertion(personID, PersonPropertyBornOn, eventDate, eventRecord, conv)
    if eventPlace != "" {
        person.Properties[PersonPropertyBornAt] = eventPlace
        createPropertyAssertion(personID, PersonPropertyBornAt, eventPlace, eventRecord, conv)
    }
} else if eventType == EventTypeDeath && eventDate != "" {
    person.Properties[PersonPropertyDiedOn] = eventDate
    createPropertyAssertion(personID, PersonPropertyDiedOn, eventDate, eventRecord, conv)
    if eventPlace != "" {
        person.Properties[PersonPropertyDiedAt] = eventPlace
        createPropertyAssertion(personID, PersonPropertyDiedAt, eventPlace, eventRecord, conv)
    }
}
```

with:

```go
// Create event assertions for birth/death evidence chain
if (eventType == EventTypeBirth || eventType == EventTypeDeath) && eventDate != "" {
    createEventAssertion(eventID, "date", eventDate, eventRecord, conv)
    if eventPlace != "" {
        createEventAssertion(eventID, "place", eventPlace, eventRecord, conv)
    }
}
```

- [ ] **Step 3: Run tests**

Run: `make test`
Expected: Some GEDCOM import tests may fail if they assert on person properties containing `born_on`/`died_on`. Update those test assertions to check for events instead.

- [ ] **Step 4: Fix any failing GEDCOM import tests**

Look for test assertions like:
```go
if person.Properties["born_on"] != "1850" {
```
Replace with event-based checks:
```go
_, birthEvent := FindPersonEvent(result, personID, EventTypeBirth)
if birthEvent == nil || string(birthEvent.Date) != "1850" {
```

- [ ] **Step 5: Run tests again**

Run: `make test`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add go-glx/gedcom_individual.go go-glx/gedcom_evidence.go
git commit -m "feat: GEDCOM importer creates event assertions instead of person property assertions"
```

---

### Task 5: Update CLI Analysis Tools

**Files:**
- Modify: `glx/analyze_consistency.go`
- Modify: `glx/analyze_gaps.go`
- Modify: `glx/analyze_suggestions.go`
- Modify: `glx/analyze_child_census.go`

- [ ] **Step 1: Update analyze_consistency.go**

This file has 8+ call sites reading `born_on`/`died_on` via `glxlib.ExtractPropertyYear(person.Properties, ...)`. Replace all with event-based lookups.

The file has access to the archive. Replace every instance of:
```go
glxlib.ExtractPropertyYear(person.Properties, "born_on")
```
with:
```go
extractEventYear(archive, personID, glxlib.EventTypeBirth)
```

Add a helper at the top of the file (or use the one from the library if exported — check if `extractEventYear` from Task 2 is exported. If not, add a local helper):
```go
func extractEventYear(archive *glxlib.GLXFile, personID, eventType string) int {
	_, event := glxlib.FindPersonEvent(archive, personID, eventType)
	if event == nil {
		return 0
	}
	return glxlib.ExtractFirstYear(string(event.Date))
}
```

Do the same for `died_on` → `EventTypeDeath`.

- [ ] **Step 2: Update analyze_gaps.go**

Replace `checkMissingBirth` and `checkMissingDeath`:

```go
// Before:
bornOn := propertyString(person.Properties, "born_on")
bornAt := propertyString(person.Properties, "born_at")

// After:
_, birthEvent := glxlib.FindPersonEvent(archive, id, glxlib.EventTypeBirth)
bornOn := ""
bornAt := ""
if birthEvent != nil {
    bornOn = string(birthEvent.Date)
    bornAt = birthEvent.PlaceID
}
```

Same pattern for `checkMissingDeath` with death events.

- [ ] **Step 3: Update analyze_suggestions.go**

Replace:
```go
glxlib.ExtractPropertyYear(person.Properties, "born_on")
```
with:
```go
extractEventYear(archive, personID, glxlib.EventTypeBirth)
```

And:
```go
propertyString(person.Properties["died_on"])
```
with:
```go
_, deathEvent := glxlib.FindPersonEvent(archive, personID, glxlib.EventTypeDeath)
// use deathEvent.Date if non-nil
```

- [ ] **Step 4: Update analyze_child_census.go**

Same pattern — replace property reads with `FindPersonEvent` and event field access.

- [ ] **Step 5: Run tests**

Run: `make test`
Expected: PASS (or fix any failing test fixtures)

- [ ] **Step 6: Commit**

```bash
git add glx/analyze_consistency.go glx/analyze_gaps.go glx/analyze_suggestions.go glx/analyze_child_census.go
git commit -m "refactor: Analysis tools use event lookups instead of born/died properties"
```

---

### Task 6: Update CLI Display Tools

**Files:**
- Modify: `glx/timeline_runner.go`
- Modify: `glx/vitals_runner.go`
- Modify: `glx/summary_runner.go`
- Modify: `glx/tree_runner.go`
- Modify: `glx/duplicates_runner.go`

- [ ] **Step 1: Update timeline_runner.go**

Remove the entire birth/death synthesis block (lines ~181-207). The timeline already processes events from the event entities — the synthesis block was a fallback for when properties existed without events. After this change, events are the only source.

Delete:
```go
if !foundEventTypes["birth"] {
    date := propertyString(person.Properties, "born_on")
    if date != "" {
        placeID := propertyString(person.Properties, "born_at")
        // ... synthesis code ...
    }
}

if !foundEventTypes["death"] {
    date := propertyString(person.Properties, "died_on")
    if date != "" {
        placeID := propertyString(person.Properties, "died_at")
        // ... synthesis code ...
    }
}
```

- [ ] **Step 2: Update vitals_runner.go**

Replace the property-first-then-event pattern:
```go
birth := formatPropertyDatePlace(person.Properties, "born_on", "born_at", archive)
if birth == "" {
    birth = findEventByType(personID, "birth", eventIDs, archive)
}
```

with direct event lookup:
```go
birth := findEventByType(personID, "birth", eventIDs, archive)
```

Or use `FindPersonEvent` from the library:
```go
_, birthEvent := glxlib.FindPersonEvent(archive, personID, glxlib.EventTypeBirth)
birth := ""
if birthEvent != nil {
    birth = formatEventDatePlace(birthEvent, archive)
}
```

Same for death. Add `formatEventDatePlace` helper if needed:
```go
func formatEventDatePlace(event *glxlib.Event, archive *glxlib.GLXFile) string {
	parts := []string{}
	if event.Date != "" {
		parts = append(parts, string(event.Date))
	}
	if event.PlaceID != "" {
		if place, ok := archive.Places[event.PlaceID]; ok {
			parts = append(parts, placeDisplayName(place))
		}
	}
	return strings.Join(parts, ", ")
}
```

Check if `findEventByType` already does what's needed — it may already be sufficient and simpler to just remove the property fallback.

- [ ] **Step 3: Update summary_runner.go**

Remove `born_on`, `born_at`, `died_on`, `died_at` from `summarySkippedProperties`:
```go
var summarySkippedProperties = map[string]bool{
    "name": true, "primary_name": true,
    "gender": true, "sex": true,
}
```

These properties won't exist on persons anymore, so skipping them is unnecessary.

- [ ] **Step 4: Update tree_runner.go**

Replace property reads for display labels with event lookups:
```go
// Before:
propertyString(person.Properties, "born_on")
propertyString(person.Properties, "died_on")

// After:
_, birthEvent := glxlib.FindPersonEvent(archive, personID, glxlib.EventTypeBirth)
bornOn := ""
if birthEvent != nil {
    bornOn = string(birthEvent.Date)
}
_, deathEvent := glxlib.FindPersonEvent(archive, personID, glxlib.EventTypeDeath)
diedOn := ""
if deathEvent != nil {
    diedOn = string(deathEvent.Date)
}
```

- [ ] **Step 5: Update duplicates_runner.go**

Replace:
```go
born := propertyString(person.Properties, glxlib.PersonPropertyBornOn)
```
with:
```go
_, birthEvent := glxlib.FindPersonEvent(archive, personID, glxlib.EventTypeBirth)
born := ""
if birthEvent != nil {
    born = string(birthEvent.Date)
}
```

- [ ] **Step 6: Run tests**

Run: `make test`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add glx/timeline_runner.go glx/vitals_runner.go glx/summary_runner.go glx/tree_runner.go glx/duplicates_runner.go
git commit -m "refactor: Display tools use event lookups instead of born/died properties"
```

---

### Task 7: Update CLI Query, Coverage, and Places Tools

**Files:**
- Modify: `glx/query_runner.go`
- Modify: `glx/coverage_runner.go`
- Modify: `glx/coverage_state_census.go`
- Modify: `glx/places_runner.go`
- Modify: `glx/tree_suggestions.go`
- Modify: `glx/census_runner.go`

- [ ] **Step 1: Update query_runner.go**

Replace `personMatchesBirthplace`:
```go
func personMatchesBirthplace(person *glxlib.Person, query string, archive *glxlib.GLXFile) bool {
    query = strings.ToLower(strings.TrimSpace(query))
    _, birthEvent := glxlib.FindPersonEvent(archive, personID, glxlib.EventTypeBirth)
    if birthEvent == nil || birthEvent.PlaceID == "" {
        return false
    }
    // Match against place ID and resolved place name
    // ... keep existing matching logic but use birthEvent.PlaceID ...
}
```

Note: `personMatchesBirthplace` needs the personID. Check the current call site to see if personID is available — it likely is since the caller iterates persons by ID.

Also update the display lines:
```go
// Before:
bornOn := propertyString(person.Properties, "born_on")
diedOn := propertyString(person.Properties, "died_on")

// After:
_, birthEvent := glxlib.FindPersonEvent(archive, personID, glxlib.EventTypeBirth)
bornOn := ""
if birthEvent != nil { bornOn = string(birthEvent.Date) }
_, deathEvent := glxlib.FindPersonEvent(archive, personID, glxlib.EventTypeDeath)
diedOn := ""
if deathEvent != nil { diedOn = string(deathEvent.Date) }
```

And the `BornBefore`/`BornAfter` filter:
```go
// Before:
year := extractPropertyYear(person.Properties, "born_on")

// After:
year := extractEventYear(archive, personID, glxlib.EventTypeBirth)
```

- [ ] **Step 2: Update coverage_runner.go**

Replace property reads:
```go
// Before:
bornOn := propertyString(props, glxlib.PersonPropertyBornOn)
bornAt := propertyString(props, glxlib.PersonPropertyBornAt)
diedOn := propertyString(props, glxlib.PersonPropertyDiedOn)
diedAt := propertyString(props, glxlib.PersonPropertyDiedAt)

// After:
_, birthEvent := glxlib.FindPersonEvent(archive, personID, glxlib.EventTypeBirth)
bornOn, bornAt := "", ""
if birthEvent != nil {
    bornOn = string(birthEvent.Date)
    bornAt = birthEvent.PlaceID
}
_, deathEvent := glxlib.FindPersonEvent(archive, personID, glxlib.EventTypeDeath)
diedOn, diedAt := "", ""
if deathEvent != nil {
    diedOn = string(deathEvent.Date)
    diedAt = deathEvent.PlaceID
}
```

The `coverageResult` struct fields `BornOn`, `BornAt`, `DiedOn`, `DiedAt` can keep those names — they describe what the data IS, not where it came from.

- [ ] **Step 3: Update coverage_state_census.go**

Replace `placeRefsFromProperty` calls on `born_at`/`died_at` with event PlaceID lookups.

- [ ] **Step 4: Update places_runner.go**

Remove `born_at` and `died_at` from `placeRefPropertyKeys` and `placeRefProperties`:
```go
var placeRefPropertyKeys = []string{"buried_at", "residence"}
var placeRefProperties = map[string]bool{
    "buried_at": true, "residence": true,
}
```

Add event-based place tracking: when collecting place usages, also scan events for place references (birth/death events use `PlaceID`). Check if the places runner already tracks event PlaceIDs elsewhere — it likely does. If so, removing `born_at`/`died_at` is sufficient since the same places will be found via event scanning.

- [ ] **Step 5: Update tree_suggestions.go and census_runner.go**

`tree_suggestions.go` — replace property reads with event lookups (same pattern as other files).

`census_runner.go` — replace assertion property check:
```go
// Before:
if a.Property == glxlib.PersonPropertyBornAt || a.Property == glxlib.PersonPropertyResidence {

// After:
if a.Property == "place" || a.Property == glxlib.PersonPropertyResidence {
```

Note: After migration, birthplace assertions will have `Property: "place"` on event subjects. Check if `a.Subject.Event != ""` should also be checked to distinguish event-place assertions from other "place" properties.

- [ ] **Step 6: Run tests**

Run: `make test`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add glx/query_runner.go glx/coverage_runner.go glx/coverage_state_census.go glx/places_runner.go glx/tree_suggestions.go glx/census_runner.go
git commit -m "refactor: Query, coverage, and places tools use event lookups"
```

---

### Task 8: Rename Constants and Add Banned Property Validation

**Files:**
- Modify: `go-glx/constants.go`
- Modify: `go-glx/validation.go`

- [ ] **Step 1: Rename constants**

In `go-glx/constants.go`, rename:
```go
const (
    DeprecatedPropertyBornOn = "born_on"
    DeprecatedPropertyBornAt = "born_at"
    DeprecatedPropertyDiedOn = "died_on"
    DeprecatedPropertyDiedAt = "died_at"
)
```

- [ ] **Step 2: Fix any remaining references to old constant names**

Run: `grep -rn 'PersonPropertyBorn\|PersonPropertyDied' go-glx/ glx/`

Any remaining references must use the `Deprecated*` names or have been replaced by event lookups in prior tasks. Fix any stragglers — by this point, all behavioral code should already be using events. Only validation and migration should reference the deprecated constants.

Update the comment in `go-glx/rename.go` (line 185) that says "Person properties (born_at, died_at, etc. can contain place IDs)" — remove the born_at/died_at mention since those properties no longer exist. The generic `replaceInProperties` call on person properties still works for other place-referencing properties like `residence`.

Also update `gedcom_export_person.go` skip map if not done in Task 3:
```go
DeprecatedPropertyBornOn: true,
DeprecatedPropertyBornAt: true,
DeprecatedPropertyDiedOn: true,
DeprecatedPropertyDiedAt: true,
```

- [ ] **Step 3: Add banned property validation**

In `go-glx/validation.go`, in the `validateProperties` function, add a check before the unknown property warning:

```go
// Banned (removed) properties — error, not warning
var removedProperties = map[string]string{
    DeprecatedPropertyBornOn: "use birth events instead",
    DeprecatedPropertyBornAt: "use birth events instead",
    DeprecatedPropertyDiedOn: "use death events instead",
    DeprecatedPropertyDiedAt: "use death events instead",
}

// Inside validateProperties, in the !exists branch:
if msg, removed := removedProperties[propName]; removed {
    result.Errors = append(result.Errors, ValidationError{
        SourceType: entityType,
        SourceID:   entityID,
        Field:      "properties." + propName,
        Message:    fmt.Sprintf("%s[%s]: property '%s' has been removed — %s. Run 'glx migrate' to convert.", entityType, entityID, propName, msg),
    })
    continue
}
// ... existing unknown property warning ...
```

- [ ] **Step 4: Add validation test**

Add a test case in the validation test file for the banned property error:
```go
t.Run("removed property born_on", func(t *testing.T) {
    archive := &GLXFile{
        Persons: map[string]*Person{
            "person-1": {Properties: map[string]any{"born_on": "1850"}},
        },
    }
    result := archive.Validate()
    if len(result.Errors) == 0 {
        t.Fatal("expected error for removed property")
    }
    found := false
    for _, e := range result.Errors {
        if strings.Contains(e.Message, "born_on") && strings.Contains(e.Message, "removed") {
            found = true
            break
        }
    }
    if !found {
        t.Error("expected error mentioning removed property 'born_on'")
    }
})
```

- [ ] **Step 5: Run tests**

Run: `make test`
Expected: PASS — but check for tests that still set `born_on` etc. on person properties and now get unexpected validation errors. Fix those test fixtures.

- [ ] **Step 6: Commit**

```bash
git add go-glx/constants.go go-glx/validation.go go-glx/gedcom_export_person.go
git commit -m "feat: Rename born/died constants to Deprecated, add banned property validation"
```

---

### Task 9: Remove from Vocabulary and Update Specification

**Files:**
- Modify: `specification/5-standard-vocabularies/person-properties.glx`
- Modify: `specification/4-entity-types/person.md`
- Check: `specification/schema/v1/` for references

- [ ] **Step 1: Remove from person-properties.glx**

Remove these entries from `specification/5-standard-vocabularies/person-properties.glx`:

```yaml
  born_on:
    label: "Birth Date"
    description: "Date of birth"
    value_type: date

  born_at:
    label: "Birth Place"
    description: "Place of birth"
    reference_type: places

  died_on:
    label: "Death Date"
    description: "Date of death"
    value_type: date

  died_at:
    label: "Death Place"
    description: "Place of death"
    reference_type: places
```

- [ ] **Step 2: Check and update JSON schemas**

Run: `grep -rn 'born_on\|born_at\|died_on\|died_at' specification/schema/`

Remove any references found.

- [ ] **Step 3: Update person.md specification**

In `specification/4-entity-types/person.md`, remove documentation of these four properties. Add a note that birth and death information is stored on Event entities of type `birth`/`death`.

- [ ] **Step 4: Run tests**

Run: `make test`
Expected: PASS — vocabulary changes may cause tests that embed vocabularies to need updating.

- [ ] **Step 5: Commit**

```bash
git add specification/
git commit -m "feat: Remove born_on/born_at/died_on/died_at from vocabulary and spec"
```

---

### Task 10: Update Example Archives

**Files:**
- Modify: 9 example archive files (see list below)

- [ ] **Step 1: Update each example archive**

For each file, remove `born_on`, `born_at`, `died_on`, `died_at` from person properties. Ensure a corresponding birth/death event exists in the archive. If one doesn't exist, add it.

**Files to update:**
1. `docs/examples/single-file/archive.glx`
2. `docs/examples/complete-family/persons/person-john-smith.glx`
3. `docs/examples/complete-family/persons/person-mary-brown.glx`
4. `docs/examples/complete-family/persons/person-jane-smith.glx`
5. `docs/examples/complete-family/assertions/assertion-john-birth.glx` — change assertion subject from `{person: ...}` with `property: born_on` to `{event: ...}` with `property: date`
6. `docs/examples/complete-family/assertions/assertion-john-birthplace.glx` — same, use `property: place`
7. `docs/examples/temporal-properties/archive.glx`
8. `docs/examples/assertion-workflow/archive.glx` — update all `born_on`/`born_at` assertions to target events
9. `docs/examples/participant-assertions/archive.glx` — add birth events for persons that have `born_on` but no birth event

For assertion files, the migration pattern is:
```yaml
# Before:
assertions:
  assertion-john-birth:
    subject: { person: person-john-smith-1850 }
    property: born_on
    value: "1850-01-15"

# After:
assertions:
  assertion-john-birth:
    subject: { event: event-birth-john }
    property: date
    value: "1850-01-15"
```

- [ ] **Step 2: Run the check-examples skill if available**

Run: `make test`

Also check if any example validation tests exist that verify example archives.

- [ ] **Step 3: Commit**

```bash
git add docs/examples/
git commit -m "feat: Update example archives to use events instead of born/died properties"
```

---

### Task 11: Add Migration Tool

**Files:**
- Create: `glx/cmd_migrate.go`
- Create: `glx/migrate_runner.go`
- Create: `glx/migrate_runner_test.go`

- [ ] **Step 1: Write the migration test**

```go
// glx/migrate_runner_test.go
package main

import (
	"testing"

	glxlib "github.com/genealogix/glx/go-glx"
)

func TestMigrateRemovesBornDiedProperties(t *testing.T) {
	t.Run("creates birth event from properties when no event exists", func(t *testing.T) {
		archive := &glxlib.GLXFile{
			Persons: map[string]*glxlib.Person{
				"person-1": {Properties: map[string]any{
					"name":    "John Smith",
					"born_on": "1850-03-15",
					"born_at": "place-leeds",
				}},
			},
			Events: map[string]*glxlib.Event{},
			Places: map[string]*glxlib.Place{
				"place-leeds": {Properties: map[string]any{"name": "Leeds"}},
			},
		}

		report := migrateArchive(archive)

		// Properties should be removed
		if _, ok := archive.Persons["person-1"].Properties["born_on"]; ok {
			t.Error("born_on should be removed")
		}
		if _, ok := archive.Persons["person-1"].Properties["born_at"]; ok {
			t.Error("born_at should be removed")
		}

		// Birth event should be created
		_, birthEvent := glxlib.FindPersonEvent(archive, "person-1", glxlib.EventTypeBirth)
		if birthEvent == nil {
			t.Fatal("expected birth event to be created")
		}
		if string(birthEvent.Date) != "1850-03-15" {
			t.Errorf("birth date = %q, want %q", birthEvent.Date, "1850-03-15")
		}
		if birthEvent.PlaceID != "place-leeds" {
			t.Errorf("birth place = %q, want %q", birthEvent.PlaceID, "place-leeds")
		}

		if report.EventsCreated != 1 {
			t.Errorf("events created = %d, want 1", report.EventsCreated)
		}
	})

	t.Run("merges into existing event with missing date", func(t *testing.T) {
		archive := &glxlib.GLXFile{
			Persons: map[string]*glxlib.Person{
				"person-1": {Properties: map[string]any{
					"born_on": "1850-03-15",
					"born_at": "place-leeds",
				}},
			},
			Events: map[string]*glxlib.Event{
				"event-birth-1": {
					Type:    glxlib.EventTypeBirth,
					PlaceID: "place-leeds",
					// Date is missing
					Participants: []glxlib.Participant{
						{Person: "person-1", Role: glxlib.ParticipantRolePrincipal},
					},
				},
			},
		}

		migrateArchive(archive)

		// Date should be filled in from property
		if string(archive.Events["event-birth-1"].Date) != "1850-03-15" {
			t.Errorf("date = %q, want %q", archive.Events["event-birth-1"].Date, "1850-03-15")
		}
		// Properties should be removed
		if _, ok := archive.Persons["person-1"].Properties["born_on"]; ok {
			t.Error("born_on should be removed")
		}
	})

	t.Run("does not overwrite existing event data", func(t *testing.T) {
		archive := &glxlib.GLXFile{
			Persons: map[string]*glxlib.Person{
				"person-1": {Properties: map[string]any{
					"born_on": "1850-03-15",
				}},
			},
			Events: map[string]*glxlib.Event{
				"event-birth-1": {
					Type: glxlib.EventTypeBirth,
					Date: "1850-03-20", // Different date — keep this one
					Participants: []glxlib.Participant{
						{Person: "person-1", Role: glxlib.ParticipantRolePrincipal},
					},
				},
			},
		}

		migrateArchive(archive)

		// Existing date should NOT be overwritten
		if string(archive.Events["event-birth-1"].Date) != "1850-03-20" {
			t.Errorf("date = %q, want %q (should not overwrite)", archive.Events["event-birth-1"].Date, "1850-03-20")
		}
	})

	t.Run("converts property assertions to event assertions", func(t *testing.T) {
		archive := &glxlib.GLXFile{
			Persons: map[string]*glxlib.Person{
				"person-1": {Properties: map[string]any{
					"born_on": "1850-03-15",
				}},
			},
			Events: map[string]*glxlib.Event{
				"event-birth-1": {
					Type: glxlib.EventTypeBirth,
					Date: "1850-03-15",
					Participants: []glxlib.Participant{
						{Person: "person-1", Role: glxlib.ParticipantRolePrincipal},
					},
				},
			},
			Assertions: map[string]*glxlib.Assertion{
				"assertion-1": {
					Subject:  glxlib.EntityRef{Person: "person-1"},
					Property: "born_on",
					Value:    "1850-03-15",
					Sources:  []string{"source-1"},
				},
			},
		}

		migrateArchive(archive)

		a := archive.Assertions["assertion-1"]
		if a.Subject.Person != "" {
			t.Error("assertion should no longer target person")
		}
		if a.Subject.Event != "event-birth-1" {
			t.Errorf("assertion event = %q, want %q", a.Subject.Event, "event-birth-1")
		}
		if a.Property != "date" {
			t.Errorf("assertion property = %q, want %q", a.Property, "date")
		}
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `make test`
Expected: FAIL — `migrateArchive` not defined

- [ ] **Step 3: Write migration runner**

```go
// glx/migrate_runner.go
package main

import (
	"fmt"

	glxlib "github.com/genealogix/glx/go-glx"
)

type migrateReport struct {
	EventsCreated      int
	EventsMerged       int
	PropertiesRemoved  int
	AssertionsMigrated int
}

func migrateArchive(archive *glxlib.GLXFile) migrateReport {
	var report migrateReport

	if archive.Events == nil {
		archive.Events = make(map[string]*glxlib.Event)
	}

	for personID, person := range archive.Persons {
		if person == nil {
			continue
		}
		report.migrateVitalProperty(archive, personID, person, "born_on", "born_at", glxlib.EventTypeBirth, "Birth")
		report.migrateVitalProperty(archive, personID, person, "died_on", "died_at", glxlib.EventTypeDeath, "Death")
	}

	// Migrate assertions
	propertyToEventProp := map[string]string{
		glxlib.DeprecatedPropertyBornOn: "date",
		glxlib.DeprecatedPropertyBornAt: "place",
		glxlib.DeprecatedPropertyDiedOn: "date",
		glxlib.DeprecatedPropertyDiedAt: "place",
	}
	propertyToEventType := map[string]string{
		glxlib.DeprecatedPropertyBornOn: glxlib.EventTypeBirth,
		glxlib.DeprecatedPropertyBornAt: glxlib.EventTypeBirth,
		glxlib.DeprecatedPropertyDiedOn: glxlib.EventTypeDeath,
		glxlib.DeprecatedPropertyDiedAt: glxlib.EventTypeDeath,
	}

	for _, assertion := range archive.Assertions {
		if assertion == nil || assertion.Subject.Person == "" {
			continue
		}
		eventProp, isDeprecated := propertyToEventProp[assertion.Property]
		if !isDeprecated {
			continue
		}
		eventType := propertyToEventType[assertion.Property]
		eventID, _ := glxlib.FindPersonEvent(archive, assertion.Subject.Person, eventType)
		if eventID == "" {
			continue
		}
		assertion.Subject = glxlib.EntityRef{Event: eventID}
		assertion.Property = eventProp
		report.AssertionsMigrated++
	}

	return report
}

func (r *migrateReport) migrateVitalProperty(archive *glxlib.GLXFile, personID string, person *glxlib.Person, dateProp, placeProp, eventType, title string) {
	dateVal, hasDate := person.Properties[dateProp]
	placeVal, hasPlace := person.Properties[placeProp]

	if !hasDate && !hasPlace {
		return
	}

	dateStr, _ := dateVal.(string)
	placeStr, _ := placeVal.(string)

	eventID, existingEvent := glxlib.FindPersonEvent(archive, personID, eventType)

	if existingEvent == nil {
		// Create new event
		eventID = generateMigrateEventID(archive, eventType, personID)
		event := &glxlib.Event{
			Type:  eventType,
			Title: title,
			Date:  glxlib.DateString(dateStr),
			PlaceID: placeStr,
			Participants: []glxlib.Participant{
				{Person: personID, Role: glxlib.ParticipantRolePrincipal},
			},
		}
		archive.Events[eventID] = event
		r.EventsCreated++
	} else {
		// Merge into existing event — fill gaps, don't overwrite
		merged := false
		if existingEvent.Date == "" && dateStr != "" {
			existingEvent.Date = glxlib.DateString(dateStr)
			merged = true
		}
		if existingEvent.PlaceID == "" && placeStr != "" {
			existingEvent.PlaceID = placeStr
			merged = true
		}
		if merged {
			r.EventsMerged++
		}
	}

	// Remove properties
	if hasDate {
		delete(person.Properties, dateProp)
		r.PropertiesRemoved++
	}
	if hasPlace {
		delete(person.Properties, placeProp)
		r.PropertiesRemoved++
	}
}

func generateMigrateEventID(archive *glxlib.GLXFile, eventType, personID string) string {
	base := fmt.Sprintf("event-%s-%s", eventType, personID)
	if _, exists := archive.Events[base]; !exists {
		return base
	}
	for i := 2; ; i++ {
		id := fmt.Sprintf("%s-%d", base, i)
		if _, exists := archive.Events[id]; !exists {
			return id
		}
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `make test`
Expected: PASS

- [ ] **Step 5: Write the CLI command**

```go
// glx/cmd_migrate.go
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	glxlib "github.com/genealogix/glx/go-glx"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate [archive]",
	Short: "Migrate an archive to the current format",
	Long:  "Converts deprecated person properties (born_on, born_at, died_on, died_at) to birth/death events.",
	Args:  cobra.ExactArgs(1),
	RunE:  runMigrate,
}

func runMigrate(_ *cobra.Command, args []string) error {
	return runMigrateArchive(args[0])
}

func runMigrateArchive(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading archive: %w", err)
	}

	archive, err := glxlib.ParseArchive(data)
	if err != nil {
		return fmt.Errorf("parsing archive: %w", err)
	}

	report := migrateArchive(archive)

	output, err := glxlib.SerializeToBytes(archive)
	if err != nil {
		return fmt.Errorf("serializing archive: %w", err)
	}

	if err := os.WriteFile(path, output, 0o644); err != nil {
		return fmt.Errorf("writing archive: %w", err)
	}

	fmt.Printf("Migration complete:\n")
	fmt.Printf("  Events created: %d\n", report.EventsCreated)
	fmt.Printf("  Events merged:  %d\n", report.EventsMerged)
	fmt.Printf("  Properties removed: %d\n", report.PropertiesRemoved)
	fmt.Printf("  Assertions migrated: %d\n", report.AssertionsMigrated)

	return nil
}
```

Note: Check the actual function names for `ParseArchive` and `SerializeToBytes` — they may differ. Match the existing patterns used by other CLI commands (e.g., `cmd_validate.go`, `cmd_import.go`).

- [ ] **Step 6: Register the command**

In the file that registers CLI commands (likely `glx/cli_commands.go` or `glx/main.go`), add:
```go
rootCmd.AddCommand(migrateCmd)
```

- [ ] **Step 7: Run tests**

Run: `make test`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add glx/cmd_migrate.go glx/migrate_runner.go glx/migrate_runner_test.go glx/cli_commands.go
git commit -m "feat: Add glx migrate command for born/died property conversion"
```

---

### Task 12: Update Documentation and Changelog

**Files:**
- Modify: `CHANGELOG.md`
- Modify: `glx/README.md`
- Check: `docs/quickstart.md`, `docs/guides/hands-on-cli-guide.md`
- Check: `website/.vitepress/config.js`

- [ ] **Step 1: Update CHANGELOG.md**

Add to the current unreleased section. Check which section is unreleased with `git tag --sort=-v:refname | head -1`.

Add under appropriate subsections:

```markdown
### Removed

#### Person Properties
- **BREAKING**: Removed `born_on`, `born_at`, `died_on`, `died_at` person properties. Birth and death information now lives exclusively on Event entities of type `birth`/`death`. Use `glx migrate` to convert existing archives.

### Added

- `glx migrate` command to convert deprecated person properties to birth/death events
```

- [ ] **Step 2: Update CLI documentation**

Add `glx migrate` to:
1. `glx/README.md` — add to command reference
2. `website/.vitepress/config.js` — add to sidebar if appropriate
3. `docs/guides/hands-on-cli-guide.md` — add section if it documents other commands

- [ ] **Step 3: Check quickstart.md**

Run: `grep -n 'born_on\|born_at\|died_on\|died_at' docs/quickstart.md`

Update any references to use events instead.

- [ ] **Step 4: Commit**

```bash
git add CHANGELOG.md glx/README.md docs/ website/
git commit -m "docs: Document born/died property removal and glx migrate command"
```

---

### Task 13: Final Verification

- [ ] **Step 1: Run full test suite**

Run: `make test`
Expected: ALL PASS

- [ ] **Step 2: Verify no remaining references to old constants**

Run: `grep -rn 'PersonPropertyBorn\|PersonPropertyDied' go-glx/ glx/`
Expected: No matches (all should be `DeprecatedProperty*` now)

- [ ] **Step 3: Verify no remaining property reads in non-migration code**

Run: `grep -rn '"born_on"\|"born_at"\|"died_on"\|"died_at"' go-glx/ glx/ --include='*.go'`
Expected: Only matches in `constants.go` (deprecated constant values), `validation.go` (banned property map), `migrate_runner.go` (migration logic), and test files.

- [ ] **Step 4: Build and smoke test**

```bash
make build
./bin/glx import glx/testdata/gedcom/shakespeare.ged -o /tmp/shakespeare-test.glx
./bin/glx validate /tmp/shakespeare-test.glx
```

Expected: Import succeeds with no born/died properties on persons. Validate passes.

- [ ] **Step 5: Test migration on an example**

```bash
./bin/glx migrate docs/examples/single-file/archive.glx
./bin/glx validate docs/examples/single-file/archive.glx
```

Wait — the examples were already updated in Task 10. To test migration, create a temporary test file with old-format properties and migrate it.

- [ ] **Step 6: Commit any final fixes**

If any issues were found, fix and commit.
