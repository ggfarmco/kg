package golang

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsExported(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"Foo", true},
		{"foo", false},
		{"", false},
		{"_Foo", false},
		{"ID", true},
		{"αlpha", false},
	}
	for _, tc := range cases {
		require.Equal(t, tc.want, IsExported(tc.in), "in=%q", tc.in)
	}
}
