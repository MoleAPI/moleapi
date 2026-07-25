package main

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInjectUmamiAnalyticsEscapesEnvValues(t *testing.T) {
	previousIndexPage := indexPage
	t.Cleanup(func() {
		indexPage = previousIndexPage
		require.NoError(t, os.Unsetenv("UMAMI_WEBSITE_ID"))
		require.NoError(t, os.Unsetenv("UMAMI_SCRIPT_URL"))
	})

	indexPage = []byte("<head><!--umami-->\n</head>")
	require.NoError(t, os.Setenv("UMAMI_WEBSITE_ID", `site" onclick="x`))
	require.NoError(t, os.Setenv("UMAMI_SCRIPT_URL", `https://umami.example/script.js?q="x"`))

	InjectUmamiAnalytics()

	html := string(indexPage)
	require.Contains(t, html, `src="https://umami.example/script.js?q=&#34;x&#34;"`)
	require.Contains(t, html, `data-website-id="site&#34; onclick=&#34;x"`)
	require.NotContains(t, html, `onclick="x"`)
}

func TestInjectUmamiAnalyticsSkipsInvalidScriptURL(t *testing.T) {
	previousIndexPage := indexPage
	t.Cleanup(func() {
		indexPage = previousIndexPage
		require.NoError(t, os.Unsetenv("UMAMI_WEBSITE_ID"))
		require.NoError(t, os.Unsetenv("UMAMI_SCRIPT_URL"))
	})

	indexPage = []byte("<head><!--umami-->\n</head>")
	require.NoError(t, os.Setenv("UMAMI_WEBSITE_ID", "site-id"))
	require.NoError(t, os.Setenv("UMAMI_SCRIPT_URL", "javascript:alert(1)"))

	InjectUmamiAnalytics()

	require.False(t, strings.Contains(string(indexPage), "<script defer"))
}
