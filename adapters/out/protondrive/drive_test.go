package protondrive

import (
	"errors"
	"fmt"
	"testing"

	"github.com/ProtonMail/go-proton-api"

	"github.com/tom-barone/proton-drive-photos-export/core"
)

// Given a wrapped Proton API error
// When wrapErr runs
// Then the error carries the API message as a core.UserError
func TestWrapErrTranslatesAPIError(t *testing.T) {
	err := fmt.Errorf("logging in: %w", &proton.APIError{Message: "Incorrect login credentials"})
	wrapErr(&err)

	var ue *core.UserError
	if !errors.As(err, &ue) || ue.Msg != "Incorrect login credentials" {
		t.Fatalf("got %v, want UserError with the API message", err)
	}
}

// Given an ordinary error with no API error in its chain
// When wrapErr runs
// Then the error is left untouched
func TestWrapErrLeavesOtherErrors(t *testing.T) {
	orig := errors.New("plain failure")
	err := orig
	wrapErr(&err)

	if !errors.Is(err, orig) {
		t.Fatalf("got %v, want the original error", err)
	}
	var ue *core.UserError
	if errors.As(err, &ue) {
		t.Fatalf("got UserError %v, want none", ue)
	}
}
