package console_setting

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExceedsMaxCharactersUsesUTF16Length(t *testing.T) {
	require.False(t, exceedsMaxCharacters("中文", 2))
	require.True(t, exceedsMaxCharacters("😀", 1))
	require.False(t, exceedsMaxCharacters("😀", 2))
}
