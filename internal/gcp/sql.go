package gcp

import (
	"fmt"
	"regexp"
	"strings"
)

var commentRe = regexp.MustCompile(`(?s)/\*.*?\*/`)
var lineCommentRe = regexp.MustCompile(`(?m)--.*?$`)

var banned = []string{
	"INSERT", "UPDATE", "DELETE", "MERGE", "CREATE", "DROP", "ALTER", "TRUNCATE",
	"GRANT", "REVOKE", "CALL", "EXECUTE", "BEGIN", "COMMIT", "ROLLBACK",
	"DECLARE", "SET", "EXPORT", "LOAD", "SCRIPT",
}

func ValidateSelect(sql string) error {
	s := strings.TrimSpace(sql)
	s = commentRe.ReplaceAllString(s, " ")
	s = lineCommentRe.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	if s == "" {
		return fmt.Errorf("empty query")
	}
	upper := strings.ToUpper(s)
	if !strings.HasPrefix(upper, "SELECT") && !strings.HasPrefix(upper, "WITH") {
		return fmt.Errorf("only SELECT/WITH queries are allowed")
	}
	fields := tokenizeSQL(upper)
	for _, b := range banned {
		for _, f := range fields {
			if f == b {
				return fmt.Errorf("rejected statement containing %s", b)
			}
		}
	}
	return nil
}

func tokenizeSQL(upper string) []string {
	var tok []string
	cur := strings.Builder{}
	inStr := false
	for i := 0; i < len(upper); i++ {
		c := upper[i]
		if c == '\'' {
			inStr = !inStr
			continue
		}
		if inStr {
			continue
		}
		if (c >= 'A' && c <= 'Z') || c == '_' {
			cur.WriteByte(c)
			continue
		}
		if cur.Len() > 0 {
			tok = append(tok, cur.String())
			cur.Reset()
		}
	}
	if cur.Len() > 0 {
		tok = append(tok, cur.String())
	}
	return tok
}
