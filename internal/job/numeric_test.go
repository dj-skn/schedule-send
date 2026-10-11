package job

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadNumericFields(t *testing.T) {
	for _, field := range []struct {
		name          string
		def, min, max int64
		get           func(State) int64
	}{
		{"force", 0, 0, 1, func(s State) int64 { return int64(s.Force) }},
		{"quiet", 0, 0, 1, func(s State) int64 { return int64(s.Quiet) }},
		{"grace", 4, 1, 60, func(s State) int64 { return int64(s.Grace) }},
		{"run_at", 0, 0, 1<<63 - 1, func(s State) int64 { return s.RunAt }},
	} {
		t.Run(field.name, func(t *testing.T) {
			for _, tc := range []struct {
				name, value string
				want        int64
				invalid     bool
			}{
				{"absent", "", field.def, false},
				{"null", "null", field.def, false},
				{"minimum", fmt.Sprint(field.min), field.min, false},
				{"maximum", fmt.Sprint(field.max), field.max, false},
				{"string", `"4"`, 0, true},
				{"boolean", "true", 0, true},
				{"array", "[]", 0, true},
				{"object", "{}", 0, true},
				{"negative", "-1", 0, true},
				{"fraction", "0.5", 0, true},
				{"integral decimal", "1.0", 0, true},
				{"exponent", "1e0", 0, true},
				{"overflow", "9223372036854775808", 0, true},
				{"underflow", "-9223372036854775809", 0, true},
			} {
				t.Run(tc.name, func(t *testing.T) {
					path := filepath.Join(t.TempDir(), "state.json")
					raw := "{}"
					if tc.value != "" {
						raw = fmt.Sprintf(`{%q:%s}`, field.name, tc.value)
					}
					if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
						t.Fatal(err)
					}
					state, err := Load(path)
					if tc.invalid {
						if err == nil || !strings.Contains(err.Error(), "job field "+field.name) {
							t.Fatalf("Load(%s) = %+v, %v; want field-specific error", raw, state, err)
						}
					} else if err != nil || field.get(state) != tc.want {
						t.Fatalf("Load(%s) = %+v, %v; want %d", raw, state, err, tc.want)
					}
				})
			}
		})
	}
}

func TestLoadNumericRangesAndExactEpochs(t *testing.T) {
	for _, tc := range []struct {
		raw   string
		want  int64
		field string
	}{
		{`{"force":2}`, 0, "force"},
		{`{"quiet":2}`, 0, "quiet"},
		{`{"grace":0}`, 0, "grace"},
		{`{"grace":61}`, 0, "grace"},
		{`{"run_at":9007199254740993}`, 9007199254740993, ""},
		{`{"run_at":9223372036854775807}`, 9223372036854775807, ""},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.json")
			if err := os.WriteFile(path, []byte(tc.raw), 0600); err != nil {
				t.Fatal(err)
			}
			state, err := Load(path)
			if tc.field != "" {
				if err == nil || !strings.Contains(err.Error(), "job field "+tc.field) {
					t.Fatalf("Load = %+v, %v; want %s error", state, err, tc.field)
				}
			} else {
				if err != nil || state.RunAt != tc.want {
					t.Fatalf("epoch = %d, %v; want %d", state.RunAt, err, tc.want)
				}
				encoded := Encode(state)
				if !strings.Contains(string(encoded), fmt.Sprintf(`"run_at": %d`, tc.want)) {
					t.Fatalf("epoch lost on encode: %s", encoded)
				}
			}
		})
	}
}
