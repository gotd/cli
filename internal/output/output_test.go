package output

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

type sample struct {
	Name string `json:"name"`
}

func (s sample) MarshalText(w io.Writer) error {
	_, err := io.WriteString(w, "name="+s.Name)
	return err
}

func TestParseFormat(t *testing.T) {
	for _, tc := range []struct {
		in      string
		want    Format
		wantErr bool
	}{
		{"text", Text, false},
		{"json", JSON, false},
		{"yaml", "", true},
		{"", "", true},
	} {
		got, err := ParseFormat(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseFormat(%q): expected error", tc.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseFormat(%q): unexpected error: %v", tc.in, err)
		}
		if got != tc.want {
			t.Errorf("ParseFormat(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestPrinterJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := New(JSON, &buf).Emit(sample{Name: "durov"}); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, `"schema": 1`) {
		t.Errorf("missing schema in %q", got)
	}
	if !strings.Contains(got, `"name": "durov"`) {
		t.Errorf("missing data in %q", got)
	}
}

func TestPrinterTextUsesMarshaler(t *testing.T) {
	var buf bytes.Buffer
	if err := New(Text, &buf).Emit(sample{Name: "durov"}); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != "name=durov" {
		t.Errorf("text = %q, want %q", got, "name=durov")
	}
}
