package e2e

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
)

type Summary struct {
	Passed   int       `json:"passed"`
	Failed   int       `json:"failed"`
	Skipped  int       `json:"skipped"`
	Failures []Failure `json:"failures,omitempty"`
}

type Failure struct {
	Name    string `json:"name"`
	Message string `json:"message"`
	File    string `json:"file,omitempty"`
}

type junitSuite struct {
	XMLName   xml.Name     `xml:"testsuite"`
	Tests     int          `xml:"tests,attr"`
	Failures  int          `xml:"failures,attr"`
	Skipped   int          `xml:"skipped,attr"`
	TestCases []junitCase  `xml:"testcase"`
	Suites    []junitSuite `xml:"testsuite"`
}

type junitSuites struct {
	XMLName xml.Name     `xml:"testsuites"`
	Suites  []junitSuite `xml:"testsuite"`
}

type junitCase struct {
	Name      string `xml:"name,attr"`
	Classname string `xml:"classname,attr"`
	Failure   *struct {
		Message string `xml:"message,attr"`
		Body    string `xml:",chardata"`
	} `xml:"failure"`
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

func accSuite(s *Summary, su junitSuite) {
	for _, ch := range su.Suites {
		accSuite(s, ch)
	}
	for _, tc := range su.TestCases {
		name := tc.Classname
		if name != "" && tc.Name != "" {
			name = name + "/" + tc.Name
		} else {
			name = tc.Name
		}
		switch {
		case tc.Failure != nil:
			s.Failed++
			msg := tc.Failure.Message
			if msg == "" {
				msg = strings.TrimSpace(tc.Failure.Body)
			}
			if len(msg) > 500 {
				msg = msg[:500]
			}
			s.Failures = append(s.Failures, Failure{Name: name, Message: msg})
		case tc.Skipped != nil:
			s.Skipped++
		default:
			s.Passed++
		}
	}
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
	for _, p := range FindJUnitFiles(dir) {
		sum, err := ParseJUnit(p)
		if err != nil {
			continue
		}
		ok = true
		all.Passed += sum.Passed
		all.Failed += sum.Failed
		all.Skipped += sum.Skipped
		all.Failures = append(all.Failures, sum.Failures...)
	}
	return all, ok
}
