package cmd

import (
	"errors"
	"strings"
	"testing"
)

func TestCollectDownloadErrorsRunsAllInputs(t *testing.T) {
	first := errors.New("first failed")
	second := errors.New("second failed")
	called := 0
	err := collectDownloadErrors([]string{"/one", "/two"}, func(string) error {
		called++
		if called == 1 {
			return first
		}
		return second
	})
	if called != 2 || !errors.Is(err, first) || !errors.Is(err, second) {
		t.Fatalf("called=%d error=%v", called, err)
	}
	if message := err.Error(); !strings.Contains(message, "/one") || !strings.Contains(message, "/two") {
		t.Fatalf("error lacks input paths: %v", err)
	}
}
