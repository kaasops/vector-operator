package configcheck

type error interface {
	Error() string
}

type ErrConfigCheck struct {
	Reason string
	Err    error
}

func (e *ErrConfigCheck) Error() string {
	return e.Reason
}
