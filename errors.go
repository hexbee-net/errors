package errors

import (
	"errors"
	"fmt"
	"io"

	"github.com/apex/log"
)

//Error is used to be able to declare const errors
type Error string

func (e Error) Error() string { return string(e) }

// Fields is used to manipulate error fields.
type Fields map[string]interface{}

// Fields is used for compatibility with Apex Log WithFields method.
func (f Fields) Fields() log.Fields {
	return log.Fields(f)
}

type causer interface {
	Cause() error
}

// fundamental is an error that has a message and a stack, but no caller.
type fundamental struct {
	msg   string
	stack *stack
}

// New returns an error with the supplied message.
// New also records the stack trace at the point it was called.
func New(message string) error {
	return &fundamental{
		msg:   message,
		stack: callers(),
	}
}

// Errorf formats according to a format specifier and returns the string
// as a value that satisfies error.
// Errorf also records the stack trace at the point it was called.
func Errorf(format string, args ...interface{}) error {
	return &fundamental{
		msg:   fmt.Sprintf(format, args...),
		stack: callers(),
	}
}

func (f *fundamental) Error() string {
	return f.msg
}

func (f *fundamental) Format(s fmt.State, verb rune) {
	switch verb {
	case 'v':
		if s.Flag('+') {
			_, _ = io.WriteString(s, f.msg)
			f.stack.Format(s, verb)

			return
		}

		fallthrough
	case 's':
		_, _ = io.WriteString(s, f.msg)
	case 'q':
		_, _ = fmt.Fprintf(s, "%q", f.msg)
	}
}

// Wrap returns an error annotating err with a stack trace at the point Wrap is called, and the supplied message.
// If err is nil, Wrap returns nil.
func Wrap(err error, message string) error {
	if err == nil {
		return nil
	}

	err = &withMessage{
		cause: err,
		msg:   message,
	}

	return &withStack{
		err,
		callers(),
	}
}

// Wrapf returns an error annotating err with a stack trace at the point Wrapf is called, and the format specifier.
// If err is nil, Wrapf returns nil.
func Wrapf(err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}

	err = &withMessage{
		cause: err,
		msg:   fmt.Sprintf(format, args...),
	}

	return &withStack{
		err,
		callers(),
	}
}

// Unpack returns a slice of all the underlying errors, if possible.
// An error value has a cause if it implements the following
// interface:
//
//     type causer interface {
//            Cause() error
//     }
//
// If the error is nil, an empty slice will be returned.
func Unpack(err error) []error {
	stack := make([]error, 0)

	for err != nil {
		switch v := err.(type) {
		case *withStack:
		case *withFields:
		case *withMessage:
			stack = append(stack, errors.New(v.msg))
		default:
			stack = append(stack, err)
		}

		if cause, ok := err.(causer); ok {
			err = cause.Cause()
		} else {
			break
		}
	}

	// reverse the slice so that the first item is the top of the stack.
	for i := len(stack)/2 - 1; i >= 0; i-- {
		opp := len(stack) - 1 - i
		stack[i], stack[opp] = stack[opp], stack[i]
	}

	return stack
}

// GetFields retrieve all the fields associated with an error stack.
// If the error is nil, an empty slice will be returned.
func GetFields(err error) Fields {
	type fielder interface {
		Fields() Fields
	}

	fields := make(Fields)

	for err != nil {
		if f, ok := err.(fielder); ok {
			for k, v := range f.Fields() {
				fields[k] = v
			}
		}

		cause, ok := err.(causer)
		if !ok {
			break
		}

		err = cause.Cause()
	}

	return fields
}

// Cause returns the underlying cause of the error, if possible.
// An error value has a cause if it implements the following
// interface:
//
//     type causer interface {
//            Cause() error
//     }
//
// If the error does not implement Cause, the original error will
// be returned. If the error is nil, nil will be returned without further
// investigation.
func Cause(err error) error {
	for err != nil {
		cause, ok := err.(causer)
		if !ok {
			break
		}

		err = cause.Cause()
	}

	return err
}

// /////////////////////////////////////////////////////////////////////////////

type withStack struct {
	error
	*stack
}

// WithStack annotates err with a stack trace at the point WithStack was called.
// If err is nil, WithStack returns nil.
func WithStack(err error) error {
	if err == nil {
		return nil
	}

	return &withStack{
		err,
		callers(),
	}
}

func (w *withStack) Cause() error {
	return w.error
}

// Unwrap provides compatibility for Go 1.13 error chains.
func (w *withStack) Unwrap() error {
	return w.error
}

func (w *withStack) Format(s fmt.State, verb rune) {
	switch verb {
	case 'v':
		if s.Flag('+') {
			_, _ = fmt.Fprintf(s, "%+v", w.Cause())
			w.stack.Format(s, verb)

			return
		}

		fallthrough
	case 's':
		_, _ = io.WriteString(s, w.Error())
	case 'q':
		_, _ = fmt.Fprintf(s, "%q", w.Error())
	}
}

// /////////////////////////////////////////////////////////////////////////////

type withMessage struct {
	cause error
	msg   string
}

// WithMessage annotates err with a new message.
// If err is nil, WithMessage returns nil.
func WithMessage(err error, message string) error {
	if err == nil {
		return nil
	}

	return &withMessage{
		cause: err,
		msg:   message,
	}
}

// WithMessagef annotates err with the format specifier.
// If err is nil, WithMessagef returns nil.
func WithMessagef(err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}

	return &withMessage{
		cause: err,
		msg:   fmt.Sprintf(format, args...),
	}
}

func (w *withMessage) Error() string {
	return w.msg + ": " + w.cause.Error()
}

func (w *withMessage) Cause() error {
	return w.cause
}

// Unwrap provides compatibility for Go 1.13 error chains.
func (w *withMessage) Unwrap() error {
	return w.cause
}

func (w *withMessage) Format(s fmt.State, verb rune) {
	switch verb {
	case 'v':
		if s.Flag('+') {
			_, _ = fmt.Fprintf(s, "%+v\n", w.Cause())
			_, _ = io.WriteString(s, w.msg)

			return
		}

		fallthrough
	case 's', 'q':
		_, _ = io.WriteString(s, w.Error())
	}
}

// /////////////////////////////////////////////////////////////////////////////

type withFields struct {
	cause  error
	fields Fields
}

// WithField annotates err with the specified field.
// If err is nil, WithFields returns nil.
func WithField(err error, key string, value interface{}) error {
	if err == nil {
		return nil
	}

	return &withFields{
		err,
		Fields{key: value},
	}
}

// WithFields annotates err with fields.
// If err is nil, WithFields returns nil.
func WithFields(err error, fields Fields) error {
	if err == nil {
		return nil
	}

	f := make(Fields, len(fields))

	for k, v := range fields {
		f[k] = v
	}

	return &withFields{
		err,
		f,
	}
}

func (w *withFields) Error() string {
	return w.cause.Error()
}

func (w *withFields) Cause() error {
	return w.cause
}

func (w *withFields) Unwrap() error {
	return w.cause
}

func (w *withFields) Fields() Fields {
	return w.fields
}

func (w *withFields) Format(s fmt.State, verb rune) {
	switch verb {
	case 'v':
		if s.Flag('+') {
			_, _ = fmt.Fprintf(s, "%+v\n", w.Cause())
			for k, v := range w.fields {
				_, _ = fmt.Fprintf(s, "  %s: %v\n", k, v)
			}

			return
		}

		fallthrough
	case 's', 'q':
		_, _ = io.WriteString(s, w.Error())
	}
}
