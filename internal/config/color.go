package config

var defaultPalette = []string{
	"#89B4FA",
	"#A6E3A1",
	"#F9E2AF",
	"#F38BA8",
	"#CBA6F7",
	"#94E2D5",
	"#FAB387",
	"#74C7EC",
}

func HashColor(name string) string {
	h := 0
	for _, r := range name {
		h = 31*h + int(r)
	}
	if h < 0 {
		h = -h
	}
	return defaultPalette[h%len(defaultPalette)]
}
