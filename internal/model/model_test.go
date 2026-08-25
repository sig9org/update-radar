package model

import (
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestSnapshotMarshalYAMLQuotesAllReleaseVersions(t *testing.T) {
	snapshot := Snapshot{
		Suggested: []string{"6.1(5e)(M)"},
		Latest:    []string{"6.2(2e)(F)"},
		Deferred:  []string{"2.3", "2.3(1e)"},
	}

	data, err := yaml.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		`- "6.1(5e)(M)"`,
		`- "6.2(2e)(F)"`,
		`- "2.3"`,
		`- "2.3(1e)"`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("marshaled YAML does not contain %q:\n%s", want, text)
		}
	}

	var decoded Snapshot
	if err := yaml.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded.Suggested, snapshot.Suggested) ||
		!reflect.DeepEqual(decoded.Latest, snapshot.Latest) ||
		!reflect.DeepEqual(decoded.Deferred, snapshot.Deferred) {
		t.Fatalf("decoded snapshot = %#v, want release versions %#v", decoded, snapshot)
	}
}
