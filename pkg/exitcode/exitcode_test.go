package exitcode

import "testing"

func TestValues(t *testing.T) {
	tests := []struct {
		code Code
		want int
		name string
	}{
		{OK, 0, "ok"},
		{Failure, 1, "failure"},
		{Usage, 2, "usage"},
		{DataErr, 65, "dataerr"},
		{NoInput, 66, "noinput"},
		{Unavailable, 69, "unavailable"},
		{Software, 70, "software"},
		{CantCreate, 73, "cantcreate"},
		{TempFail, 75, "tempfail"},
		{NoPerm, 77, "noperm"},
		{Config, 78, "config"},
		{Interrupted, 130, "interrupted"},
		{Code(42), 42, "exit 42"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if int(tt.code) != tt.want {
				t.Errorf("value = %d, want %d", int(tt.code), tt.want)
			}
			if got := tt.code.String(); got != tt.name {
				t.Errorf("String() = %q, want %q", got, tt.name)
			}
		})
	}
}
