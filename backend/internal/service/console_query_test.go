package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nazxf/opencloud/backend/internal/model"
)

func TestDetectStatementType(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  string
	}{
		{name: "select", query: "SELECT * FROM users", want: model.StatementSelect},
		{name: "select lowercase", query: "  select id from users", want: model.StatementSelect},
		{name: "explain", query: "EXPLAIN SELECT * FROM users", want: model.StatementExplain},
		{name: "show", query: "SHOW TABLES", want: model.StatementShow},
		{name: "describe", query: "DESCRIBE users", want: model.StatementDescribe},
		{name: "desc", query: "desc users", want: model.StatementDescribe},
		{name: "with cte", query: "WITH x AS (SELECT 1) SELECT * FROM x", want: model.StatementSelect},
		{name: "insert unknown", query: "INSERT INTO users VALUES (1)", want: model.StatementUnknown},
		{name: "empty", query: "   ", want: model.StatementUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, detectStatementType(tt.query))
		})
	}
}

func TestContainsMultipleStatements(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  bool
	}{
		{name: "single", query: "SELECT 1;", want: false},
		{name: "multiple", query: "SELECT 1; SELECT 2;", want: true},
		{name: "semicolon in string", query: "SELECT 'a;b'", want: false},
		{name: "no semicolon", query: "SELECT 1", want: false},
		{name: "separator without trailing semicolon", query: "SELECT 1; SELECT 2", want: true},
		{name: "escaped quote then separator", query: `SELECT 'it''s; fine'; DROP`, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, containsMultipleStatements(tt.query))
		})
	}
}

func TestValidateQuerySafety(t *testing.T) {
	s := &ConsoleQueryService{}

	t.Run("accepts plain select", func(t *testing.T) {
		require.NoError(t, s.validateQuerySafety("SELECT * FROM orders WHERE id = 1", false))
	})
	t.Run("accepts semicolon inside string literal", func(t *testing.T) {
		require.NoError(t, s.validateQuerySafety("SELECT * FROM notes WHERE body = 'a;b'", false))
	})
	t.Run("accepts keyword-looking value inside string literal", func(t *testing.T) {
		require.NoError(t, s.validateQuerySafety("SELECT * FROM notes WHERE body = 'CREATE TABLE demo'", false))
	})
	t.Run("rejects separator mid-query", func(t *testing.T) {
		err := s.validateQuerySafety("SELECT 1; SELECT 2", false)
		require.Error(t, err)
	})
	t.Run("rejects update", func(t *testing.T) {
		require.Error(t, s.validateQuerySafety("UPDATE orders SET paid = true", false))
	})
	t.Run("rejects replace and call", func(t *testing.T) {
		require.Error(t, s.validateQuerySafety("REPLACE INTO t VALUES (1)", false))
		require.Error(t, s.validateQuerySafety("CALL do_stuff()", false))
	})
	t.Run("trailing semicolon allowed by default", func(t *testing.T) {
		require.NoError(t, s.validateQuerySafety("SELECT 1;", false))
	})
	t.Run("trailing semicolon rejected in strict mode", func(t *testing.T) {
		require.Error(t, s.validateQuerySafety("SELECT 1;", true))
	})
	t.Run("keyword outside literal still rejected", func(t *testing.T) {
		require.Error(t, s.validateQuerySafety("SELECT * FROM information_schema.tables", false))
	})
}

func TestStripStringLiterals(t *testing.T) {
	require.Equal(t, "SELECT  FROM t WHERE x = ", stripStringLiterals(`SELECT 'a;b' FROM t WHERE x = 'DROP ''TABLE'`))
	require.Equal(t, "SELECT  1", stripStringLiterals(`SELECT 'it\'s' 1`))
}

func TestCalculateExpirationBounds(t *testing.T) {
	// TTL outside [15,60] clamps to the 15-minute floor.
	require.Equal(t, time.Duration(15)*time.Minute, time.Until(calculateExpiration(0)).Round(time.Second))
	require.Equal(t, time.Duration(15)*time.Minute, time.Until(calculateExpiration(5)).Round(time.Second))
	require.Equal(t, time.Duration(30)*time.Minute, time.Until(calculateExpiration(30)).Round(time.Second))
	require.Equal(t, time.Duration(60)*time.Minute, time.Until(calculateExpiration(60)).Round(time.Second))
	require.Equal(t, time.Duration(15)*time.Minute, time.Until(calculateExpiration(120)).Round(time.Second))
}
