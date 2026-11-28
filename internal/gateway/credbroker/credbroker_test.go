package credbroker

import (
	"strings"
	"testing"
)

func TestResolve(t *testing.T) {
	const secret = "s3cr3t-value-must-not-leak"
	t.Setenv("CREDBROKER_TEST_SET", secret)
	t.Setenv("CREDBROKER_TEST_EMPTY", "")

	cases := []struct {
		name    string
		ref     string
		want    string
		wantErr string
	}{
		{name: "env set returns value", ref: "env:CREDBROKER_TEST_SET", want: secret},
		{name: "env unset is empty value", ref: "env:CREDBROKER_TEST_UNSET_XYZ", wantErr: "empty value"},
		{name: "env empty is empty value", ref: "env:CREDBROKER_TEST_EMPTY", wantErr: "empty value"},
		{name: "no colon rejected", ref: "CREDBROKER_TEST_SET", wantErr: "not scheme:name"},
		{name: "empty name rejected", ref: "env:", wantErr: "not scheme:name"},
		{name: "vault deferred", ref: "vault:CREDBROKER_TEST_SET", wantErr: "deferred"},
		{name: "kms deferred", ref: "kms:CREDBROKER_TEST_SET", wantErr: "deferred"},
		{name: "unknown scheme rejected", ref: "file:CREDBROKER_TEST_SET", wantErr: "unknown secret scheme"},
	}
	r := New()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := r.Resolve(tc.ref)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Resolve(%q) error = %v", tc.ref, err)
				}
				if got != tc.want {
					t.Fatalf("Resolve(%q) = %q, want %q", tc.ref, got, tc.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("Resolve(%q) = %q, want error containing %q", tc.ref, got, tc.wantErr)
			}
			if got != "" {
				t.Errorf("Resolve(%q) returned value %q alongside error", tc.ref, got)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("Resolve(%q) error = %q, want it to contain %q", tc.ref, err, tc.wantErr)
			}
			// The error text must never carry a resolved secret value.
			if strings.Contains(err.Error(), secret) {
				t.Errorf("Resolve(%q) error leaks the secret value: %q", tc.ref, err)
			}
		})
	}
}
