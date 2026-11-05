package serverhandlers

import "testing"

// The data of a tag is compared as a text, whatever type its column is read
// as.
func TestTagText(t *testing.T) {
	for _, tc := range []struct {
		in   any
		want string
	}{
		{nil, ""},
		{[]byte(`{"a": 1}`), `{"a": 1}`},
		{"x", "x"},
		{42, "42"},
	} {
		if got := tagText(tc.in); got != tc.want {
			t.Errorf("tagText(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
