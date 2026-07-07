package app

import (
	"errors"
	"fmt"
)

const (
	codeOK       = 0
	codeRuntime  = 1
	codeNotFound = 2
	codeUsage    = 3
	codeBadInput = 4
	codeStorage  = 5
	codePartial  = 6
)

type kind int

const (
	kindRuntime kind = iota
	kindUsage
	kindNotFound
	kindBadInput
	kindStorage
	kindPartial
)

type appError struct {
	kind kind
	msg  string
	err  error
}

func (e *appError) Error() string {
	if e.err == nil {
		return e.msg
	}
	if e.msg == "" {
		return e.err.Error()
	}
	return e.msg + ": " + e.err.Error()
}

func exitCode(err error) int {
	var ae *appError
	if !errors.As(err, &ae) {
		return codeRuntime
	}
	switch ae.kind {
	case kindUsage:
		return codeUsage
	case kindNotFound:
		return codeNotFound
	case kindBadInput:
		return codeBadInput
	case kindStorage:
		return codeStorage
	case kindPartial:
		return codePartial
	default:
		return codeRuntime
	}
}

func isAppError(err error) bool {
	var ae *appError
	return errors.As(err, &ae)
}

func usageErr(err error) error        { return &appError{kind: kindUsage, err: err} }
func usagef(f string, a ...any) error { return &appError{kind: kindUsage, msg: fmt.Sprintf(f, a...)} }
func runtimeErr(err error) error      { return &appError{kind: kindRuntime, err: err} }
func storageErr(err error) error      { return &appError{kind: kindStorage, err: err} }

func storageWrap(err error) error {
	if err == nil {
		return nil
	}
	return storageErr(err)
}

func badInputErr(err error) error {
	if err == nil {
		return nil
	}
	if isAppError(err) {
		return err
	}
	return &appError{kind: kindBadInput, err: err}
}

func badInputf(f string, a ...any) error {
	return &appError{kind: kindBadInput, msg: fmt.Sprintf(f, a...)}
}

func notFoundf(f string, a ...any) error {
	return &appError{kind: kindNotFound, msg: fmt.Sprintf(f, a...)}
}

func partialErr(err error) error {
	return &appError{kind: kindPartial, err: err}
}
