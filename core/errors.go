// AI GENERATED - Tom Barone
package core

// UserError carries a message suitable for showing to the user as-is.
// Driven adapters wrap technology errors in it; driving adapters
// unwrap it with errors.As.
type UserError struct {
	Msg string
	Err error
}

func (e *UserError) Error() string { return e.Msg }
func (e *UserError) Unwrap() error { return e.Err }
