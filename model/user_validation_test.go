package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateUsername(t *testing.T) {
	valid := []string{"user", "user_01", "user-01", "abcdefghijklmnopqrst"}
	for _, username := range valid {
		require.NoError(t, ValidateUsername(username), username)
	}

	invalid := []string{"", "abc", "abcdefghijklmnopqrstu", "user name", "user.name", "用户1234"}
	for _, username := range invalid {
		require.Error(t, ValidateUsername(username), username)
	}
}
