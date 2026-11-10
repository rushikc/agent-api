package e2e

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/rushikesh/devhub/internal/registry"
)

type Summary struct {
	Passed   int       `json:"passed"`
	Failed   int       `json:"failed"`
	Skipped  int       `json:"skipped"`
	Errors   int       `json:"errors"`
	Duration float64   `json:"duration"`
	Failures []Failure `json:"failures,omitempty"`
}

type Failure struct {
	Name      string `json:"name"`
	Classname string `json:"classname,omitempty"`
	Message   string `json:"message"`
	File      string `json:"file,omitempty"`
}

type junitSuite struct {
	XMLName   xml.Name     `xml:"testsuite"`
	Tests     int          `xml:"tests,attr"`
	Failures  int          `xml:"failures,attr"`
	Skipped   int          `xml:"skipped,attr"`
	Time      string       `xml:"time,attr"`
	TestCases []junitCase  `xml:"testcase"`
	Suites    []junitSuite `xml:"testsuite"`
}

type junitSuites struct {
	XMLName xml.Name     `xml:"testsuites"`
	Time    string       `xml:"time,attr"`
	Suites  []junitSuite `xml:"testsuite"`
}

type junitCase struct {
	Name      string `xml:"name,attr"`
	Classname string `xml:"classname,attr"`
	Time      string `xml:"time,attr"`
	Failure   *struct {
		Message string `xml:"message,attr"`
		Body    string `xml:",chardata"`
	} `xml:"failure"`
	Error *struct {
		Message string `xml:"message,attr"`
		Body    string `xml:",chardata"`
	} `xml:"error"`
	Skipped *struct{} `xml:"skipped"`
}

func ParseJUnit(path string) (Summary, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Summary{}, err
	}
	return ParseJUnitBytes(b)
}

func ParseJUnitBytes(b []byte) (Summary, error) {
	var suites junitSuites
	if err := xml.Unmarshal(b, &suites); err == nil && len(suites.Suites) > 0 {
		var s Summary
		s.Duration += parseTime(suites.Time)
		for _, su := range suites.Suites {
			accSuite(&s, su)
		}
		return s, nil
	}
	var one junitSuite
	if err := xml.Unmarshal(b, &one); err != nil {
		return Summary{}, err
	}
	var s Summary
	accSuite(&s, one)
	return s, nil
}

func parseTime(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func accSuite(s *Summary, su junitSuite) {
	s.Duration += parseTime(su.Time)
	for _, ch := range su.Suites {
		accSuite(s, ch)
	}
	for _, tc := range su.TestCases {
		msg := func(message, body string) string {
			out := message
			if out == "" {
				out = strings.TrimSpace(body)
			}
			if len(out) > 500 {
				out = out[:500]
			}
			return out
		}
		switch {
		case tc.Failure != nil:
			s.Failed++
			s.Failures = append(s.Failures, Failure{
				Name: tc.Name, Classname: tc.Classname, Message: msg(tc.Failure.Message, tc.Failure.Body),
			})
		case tc.Error != nil:
			s.Errors++
			s.Failures = append(s.Failures, Failure{
				Name: tc.Name, Classname: tc.Classname, Message: msg(tc.Error.Message, tc.Error.Body),
			})
		case tc.Skipped != nil:
			s.Skipped++
		default:
			s.Passed++
		}
	}
}

func FormatSummary(s Summary) string {
	out := fmt.Sprintf("%d passed, %d failed", s.Passed, s.Failed)
	if s.Skipped > 0 {
		out += fmt.Sprintf(", %d skipped", s.Skipped)
	}
	if s.Errors > 0 {
		out += fmt.Sprintf(", %d errors", s.Errors)
	}
	return out
}

func SummaryText(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var s Summary
	if json.Unmarshal([]byte(raw), &s) == nil && (s.Passed+s.Failed+s.Skipped+s.Errors > 0 || strings.HasPrefix(raw, "{")) {
		if s.Passed+s.Failed+s.Skipped+s.Errors == 0 && len(s.Failures) == 0 {
			return raw
		}
		return FormatSummary(s)
	}
	return raw
}

func DecorateRun(r registry.E2ERun) registry.E2ERun {
	r.SummaryText = SummaryText(r.Summary)
	return r
}

func DecorateRuns(rows []registry.E2ERun) []registry.E2ERun {
	for i := range rows {
		rows[i] = DecorateRun(rows[i])
	}
	return rows
}

func FindJUnit(dir string) string {
	files := FindJUnitFiles(dir)
	if len(files) == 0 {
		return ""
	}
	return files[0]
}

func FindJUnitFiles(dir string) []string {
	var found []string
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		n := strings.ToLower(info.Name())
		if strings.HasSuffix(n, ".xml") && (strings.Contains(n, "junit") || n == "results.xml" || n == "test-results.xml" || n == "junit.xml") {
			found = append(found, path)
		}
		return nil
	})
	return found
}

func SummarizeJUnitDir(dir string) (Summary, bool) {
	var all Summary
	ok := false
	seen := map[string]bool{}
	for _, p := range FindJUnitFiles(dir) {
		abs, err := filepath.Abs(p)
		if err != nil {
			abs = p
		}
		if canon, err := filepath.EvalSymlinks(abs); err == nil {
			abs = canon
		}
		b, err := os.ReadFile(abs)
		if err != nil {
			continue
		}
		sum, err := ParseJUnitBytes(b)
		if err != nil {
			continue
		}
		h := sha256.Sum256(b)
		key := hex.EncodeToString(h[:])
		if seen[key] {
			continue
		}
		seen[key] = true
		ok = true
		all.Passed += sum.Passed
		all.Failed += sum.Failed
		all.Skipped += sum.Skipped
		all.Errors += sum.Errors
		all.Duration += sum.Duration
		all.Failures = append(all.Failures, sum.Failures...)
	}
	return all, ok
}
