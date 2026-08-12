package reporting

import "testing"

func TestCanonicalDictionaryIsCompleteUniqueAndStable(t *testing.T) {
	items := Definitions()
	if len(items) < 12 {
		t.Fatalf("expected cross-domain dictionary, got %d", len(items))
	}
	if err := Validate(items); err != nil {
		t.Fatal(err)
	}
	for i := 1; i < len(items); i++ {
		if items[i-1].ID >= items[i].ID {
			t.Fatalf("dictionary not stable: %s then %s", items[i-1].ID, items[i].ID)
		}
	}
}

func TestIncompleteAndDuplicateMetricsFailClosed(t *testing.T) {
	items := Definitions()
	items = append(items, items[0])
	if Validate(items) == nil {
		t.Fatal("duplicate metric accepted")
	}
	broken := Definitions()[:1]
	broken[0].ReconciliationRule = ""
	if Validate(broken) == nil {
		t.Fatal("incomplete metric accepted")
	}
}
