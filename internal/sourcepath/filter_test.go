package sourcepath

import "testing"

func TestGeneratedAndTestInputsAreExcluded(t *testing.T) {
	for _, p := range []string{"web/src/app-links.test.ts", "AppTests/Flow.swift", "App/FlowTests.swift", "__tests__/thing.js"} {
		if !IsTest(p) {
			t.Errorf("test not recognized: %s", p)
		}
	}
	for _, p := range []string{"App/Flow.swift", "web/src/page.tsx", "Shared/Tracker.swift"} {
		if IsTest(p) {
			t.Errorf("production excluded: %s", p)
		}
	}
	if !SkipDir("out") || !SkipDir(".build") || SkipDir("Shared") {
		t.Fatal("source directory filtering changed")
	}
}
