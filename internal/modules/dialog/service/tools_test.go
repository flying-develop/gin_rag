package service

import (
	"context"
	"testing"
	"time"

	// Встроенная база таймзон — тестовый бинарь должен уметь резолвить зоны
	// независимо от образа, в котором прогоняются тесты.
	_ "time/tzdata"

	"github.com/stretchr/testify/require"
)

func TestCurrentTimeTool_Execute_ValidTimezone(t *testing.T) {
	out, err := currentTimeTool{}.Execute(context.Background(), `{"timezone":"Europe/Moscow"}`)
	require.NoError(t, err)

	_, perr := time.Parse(time.RFC3339, out)
	require.NoError(t, perr, "результат — валидный RFC3339: %q", out)
}

func TestCurrentTimeTool_Execute_DefaultsToUTC(t *testing.T) {
	for _, args := range []string{"", "{}"} {
		out, err := currentTimeTool{}.Execute(context.Background(), args)
		require.NoError(t, err)

		parsed, perr := time.Parse(time.RFC3339, out)
		require.NoError(t, perr, "args=%q → %q", args, out)
		_, offset := parsed.Zone()
		require.Zero(t, offset, "по умолчанию UTC (нулевое смещение)")
	}
}

func TestCurrentTimeTool_Execute_UnknownTimezone(t *testing.T) {
	out, err := currentTimeTool{}.Execute(context.Background(), `{"timezone":"Mars/Olympus"}`)
	require.NoError(t, err, "неизвестная зона деградирует в текст, а не в ошибку")
	require.Contains(t, out, "unknown timezone")
}

func TestCurrentTimeTool_Execute_MalformedArguments(t *testing.T) {
	_, err := currentTimeTool{}.Execute(context.Background(), `{"timezone":`)
	require.Error(t, err)
}

func TestCurrentTimeTool_Definition(t *testing.T) {
	def := currentTimeTool{}.Definition()
	require.Equal(t, "get_current_time", currentTimeTool{}.Name())
	require.NotNil(t, def.Function)
	require.Equal(t, "get_current_time", def.Function.Name)

	params, ok := def.Function.Parameters.(map[string]any)
	require.True(t, ok)
	props, ok := params["properties"].(map[string]any)
	require.True(t, ok)
	require.Contains(t, props, "timezone")
}
