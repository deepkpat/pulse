package errors

import "testing"

func TestIsTransientError(t *testing.T) {
	tests := []struct {
		errMsg string
		expect bool
	}{
		{"connection refused", true},
		{"i/o deadline exceeded", true},
		{"no such host", true},
		{"server is unavailable", true},
		{"context deadline exceeded", true},
		{"EOF", true},
		{"schema mismatch", false},
		{"auth failed", false},
		{"syntax error", false},
		{"no such table", false},
	}

	for _, tt := range tests {
		t.Run(tt.errMsg, func(t *testing.T) {
			err := &mockError{msg: tt.errMsg}
			if got := IsTransientError(err); got != tt.expect {
				t.Errorf("IsTransientError(%q) = %v, want %v", tt.errMsg, got, tt.expect)
			}
		})
	}
}

func TestIsPermanentError(t *testing.T) {
	tests := []struct {
		errMsg string
		expect bool
	}{
		{"schema mismatch", true},
		{"auth failed", true},
		{"syntax error", true},
		{"no such table", true},
		{"access denied", true},
		{"connection refused", false},
		{"timeout", false},
	}

	for _, tt := range tests {
		t.Run(tt.errMsg, func(t *testing.T) {
			err := &mockError{msg: tt.errMsg}
			if got := IsPermanentError(err); got != tt.expect {
				t.Errorf("IsPermanentError(%q) = %v, want %v", tt.errMsg, got, tt.expect)
			}
		})
	}
}

func TestClassifyError(t *testing.T) {
	tests := []struct {
		errMsg string
		expect string
	}{
		{"", "ok"},
		{"connection refused", "transient"},
		{"auth failed", "permanent"},
		{"some unknown weird error", "transient"}, // unknown defaults to transient
	}

	for _, tt := range tests {
		t.Run(tt.errMsg, func(t *testing.T) {
			var err error
			if tt.errMsg != "" {
				err = &mockError{msg: tt.errMsg}
			}
			if got := ClassifyError(err); got != tt.expect {
				t.Errorf("ClassifyError(%q) = %q, want %q", tt.errMsg, got, tt.expect)
			}
		})
	}
}

type mockError struct {
	msg string
}

func (m *mockError) Error() string {
	return m.msg
}
