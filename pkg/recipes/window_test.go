package recipes

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDuration(t *testing.T) {
	for in, want := range map[string]time.Duration{
		"30m":  30 * time.Minute,
		"2h":   2 * time.Hour,
		"1d":   24 * time.Hour,
		"1w":   7 * 24 * time.Hour,
		"1d6h": 30 * time.Hour,
	} {
		got, err := ParseDuration(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got, in)
	}
	for _, bad := range []string{"", "0h", "-2h", "2x", "h"} {
		_, err := ParseDuration(bad)
		assert.Error(t, err, bad)
	}
	assert.Equal(t, "1d6h", FormatDuration(30*time.Hour))
	assert.Equal(t, "7d", FormatDuration(7*24*time.Hour))
}

func TestResolveWindow(t *testing.T) {
	def := Timeframe{Declared: true, Default: 2 * time.Hour}

	w, err := ResolveWindow(def, "", "", testNow)
	require.NoError(t, err)
	assert.Equal(t, testNow.Add(-2*time.Hour), w.From)
	assert.Equal(t, testNow, w.To)

	w, err = ResolveWindow(def, "6h", "1h", testNow)
	require.NoError(t, err)
	assert.Equal(t, testNow.Add(-6*time.Hour), w.From)
	assert.Equal(t, testNow.Add(-time.Hour), w.To)

	w, err = ResolveWindow(def, "-1d", "now", testNow)
	require.NoError(t, err)
	assert.Equal(t, testNow.Add(-24*time.Hour), w.From)

	w, err = ResolveWindow(def, "2026-03-01T00:00:00Z", "2026-03-02T00:00:00Z", testNow)
	require.NoError(t, err)
	assert.Equal(t, "2026-03-01T00:00:00Z", w.FromRFC3339())

	_, err = ResolveWindow(def, "1h", "2h", testNow)
	assert.ErrorContains(t, err, "before")
	_, err = ResolveWindow(def, "yesterday", "", testNow)
	assert.Error(t, err)

	none := Timeframe{Declared: true, None: true}
	w, err = ResolveWindow(none, "", "", testNow)
	require.NoError(t, err)
	assert.Nil(t, w)
	_, err = ResolveWindow(none, "1h", "", testNow)
	assert.Error(t, err, "a state query takes no window")

	fixed := Timeframe{Declared: true, Default: 30 * time.Minute, Fixed: true}
	_, err = ResolveWindow(fixed, "2h", "", testNow)
	assert.Error(t, err, "a fixed window rejects --from")

	minWin := Timeframe{Declared: true, Default: 30 * 24 * time.Hour, Min: 7 * 24 * time.Hour}
	_, err = ResolveWindow(minWin, "1d", "", testNow)
	assert.ErrorContains(t, err, "7d")

	day := Timeframe{Declared: true, Default: 7 * 24 * time.Hour, Align: AlignUTCDay}
	w, err = ResolveWindow(day, "", "", testNow)
	require.NoError(t, err)
	assert.Equal(t, "2026-03-03T00:00:00Z", w.FromRFC3339())
	assert.Equal(t, "2026-03-10T00:00:00Z", w.ToRFC3339())
}

func TestResolveQueryWindow(t *testing.T) {
	w, err := ResolveQueryWindow("", "", testNow)
	require.NoError(t, err)
	assert.Nil(t, w)

	w, err = ResolveQueryWindow("2h", "", testNow)
	require.NoError(t, err)
	assert.Equal(t, testNow.Add(-2*time.Hour), w.From)
	assert.Equal(t, testNow, w.To)

	_, err = ResolveQueryWindow("", "1h", testNow)
	assert.Error(t, err)
}

func TestResolveWindowMax(t *testing.T) {
	tf := Timeframe{Declared: true, Default: 2 * time.Hour, Max: 24 * time.Hour}
	_, err := ResolveWindow(tf, "7d", "", testNow)
	assert.ErrorContains(t, err, "at most 1d")
	w, err := ResolveWindow(tf, "1d", "", testNow)
	require.NoError(t, err)
	assert.Equal(t, 24*time.Hour, w.To.Sub(w.From))
}
