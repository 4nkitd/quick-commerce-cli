package main

import "fmt"

type Provider string

const (
	Zepto   Provider = "zepto"
	Blinkit Provider = "blinkit"
)

var activeProvider = Zepto
var jsonOutput bool

func parseProvider(s string) (Provider, error) {
	switch Provider(s) {
	case Zepto, Blinkit:
		return Provider(s), nil
	default:
		return "", fmt.Errorf("unknown provider %q; choose zepto or blinkit", s)
	}
}

func (p Provider) website() string {
	if p == Blinkit {
		return "https://blinkit.com"
	}
	return "https://www.zepto.com"
}
