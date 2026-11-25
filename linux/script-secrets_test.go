package linux

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/stretchr/testify/require"
)

const testSecret = "s3cr3t-marker-value"

func scriptReadFixture(t *testing.T, env map[string]interface{}) (*schema.Resource, *schema.ResourceData, *linuxPool, func() []string) {
	t.Helper()
	pool, commands := recordingPool(t, func(command string) (string, string, int) {
		switch {
		case strings.HasPrefix(command, "scp "):
			return "\x00\x00\x00\x00", "", 0
		case strings.Contains(command, "TOKEN="):
			return "", "script failed", 1
		}
		return "", "", 0
	})
	res := scriptResource()
	rd := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{
		"lifecycle_commands": []interface{}{map[string]interface{}{
			"create": "true", "read": "exit 1", "update": "", "delete": "true",
		}},
		"sensitive_environment": env,
	})
	rd.SetId("managed")
	return res, rd, pool, commands
}

func TestScriptSensitiveEnvironmentIsNotDisclosed(t *testing.T) {
	env := map[string]interface{}{"TOKEN": testSecret}

	t.Run("failed read is not stored in state", func(t *testing.T) {
		res, rd, pool, commands := scriptReadFixture(t, env)

		diags := res.ReadContext(context.Background(), rd, pool)
		require.False(t, diags.HasError(), "%v", diags)
		require.NotEmpty(t, rd.Get(attrScriptFaultyOutput), "the failure must still be recorded")
		require.NotContains(t, rd.Get(attrScriptFaultyOutput), testSecret)
		require.NotContains(t, rd.Get(attrScriptDirtyOutput), testSecret)
		require.NotContains(t, rd.Get(attrScriptOutput), testSecret)
		require.True(t, containsAny(commands(), "TOKEN="+testSecret), "the script still receives the value")
	})

	t.Run("failed create is not reported in diagnostics", func(t *testing.T) {
		res, rd, pool, _ := scriptReadFixture(t, env)

		diags := res.CreateContext(context.Background(), rd, pool)
		require.True(t, diags.HasError())
		for _, d := range diags {
			require.NotContains(t, d.Summary+d.Detail, testSecret)
		}
	})
}

func containsAny(commands []string, substr string) bool {
	for _, c := range commands {
		if strings.Contains(c, substr) {
			return true
		}
	}
	return false
}

func TestScriptEnvironmentNamesAreValidated(t *testing.T) {
	pool, commands := recordingPool(t, func(string) (string, string, int) { return "", "", 0 })
	sc := &script{l: pool.def, body: "true", env: env{"A; touch /tmp/pwned #": "x"}}

	_, err := sc.exec(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "environment variable name")
	require.Empty(t, commands(), "nothing may be run for an invalid name")
}
