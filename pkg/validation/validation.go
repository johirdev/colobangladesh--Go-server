// Package validation registers custom rules on gin's validator and converts
// validator errors into field-level details for the frontend.
package validation

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"regexp"
	"strings"
	"unicode"

	"cholobangladesh-server/pkg/apperror"
	"cholobangladesh-server/pkg/phone"

	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

type FieldError struct {
	Field   string `json:"field" example:"password"`
	Rule    string `json:"rule" example:"strong_password"`
	Message string `json:"message" example:"must be 8-72 characters and contain a letter and a number"`
}

var (
	slugRe = regexp.MustCompile(`^[a-z0-9]+(?:[-_][a-z0-9]+)*$`)
	langRe = regexp.MustCompile(`^[a-z]{2}$`)
	nameRe = regexp.MustCompile(`^[\p{L}\p{M}][\p{L}\p{M} .'\-]*$`)
)

// Register must be called once at startup.
func Register() {
	v, ok := binding.Validator.Engine().(*validator.Validate)
	if !ok {
		return
	}
	v.RegisterTagNameFunc(func(f reflect.StructField) string {
		for _, tag := range []string{"json", "form"} {
			if name := strings.SplitN(f.Tag.Get(tag), ",", 2)[0]; name != "" && name != "-" {
				return name
			}
		}
		return f.Name
	})
	_ = v.RegisterValidation("strong_password", func(fl validator.FieldLevel) bool {
		return StrongPassword(fl.Field().String())
	})
	_ = v.RegisterValidation("bd_phone", func(fl validator.FieldLevel) bool {
		_, ok := phone.NormalizeBD(fl.Field().String())
		return ok
	})
	_ = v.RegisterValidation("intl_phone", func(fl validator.FieldLevel) bool {
		_, ok := phone.NormalizeE164(fl.Field().String())
		return ok
	})
	// country: ISO 3166-1 alpha-2, case-insensitive ("bd" and "BD" both accepted).
	_ = v.RegisterValidation("country", func(fl validator.FieldLevel) bool {
		return v.Var(strings.ToUpper(fl.Field().String()), "iso3166_1_alpha2") == nil
	})
	_ = v.RegisterValidation("slug", func(fl validator.FieldLevel) bool {
		return slugRe.MatchString(fl.Field().String())
	})
	_ = v.RegisterValidation("lang", func(fl validator.FieldLevel) bool {
		return langRe.MatchString(fl.Field().String())
	})
	_ = v.RegisterValidation("person_name", func(fl validator.FieldLevel) bool {
		return nameRe.MatchString(strings.TrimSpace(fl.Field().String()))
	})
	_ = v.RegisterValidation("numeric_code", func(fl validator.FieldLevel) bool {
		s := fl.Field().String()
		for _, r := range s {
			if r < '0' || r > '9' {
				return false
			}
		}
		return s != ""
	})
}

// StrongPassword: 8-72 bytes (bcrypt limit), at least one letter and one digit.
func StrongPassword(s string) bool {
	if len(s) < 8 || len(s) > 72 {
		return false
	}
	var letter, digit bool
	for _, r := range s {
		switch {
		case unicode.IsLetter(r):
			letter = true
		case unicode.IsDigit(r):
			digit = true
		}
	}
	return letter && digit
}

// FromBindError converts a gin binding error into a 422/400 app error.
func FromBindError(err error) error {
	var ve validator.ValidationErrors
	if errors.As(err, &ve) {
		out := make([]FieldError, 0, len(ve))
		for _, fe := range ve {
			out = append(out, FieldError{Field: fieldPath(fe), Rule: fe.Tag(), Message: message(fe)})
		}
		return apperror.ErrValidation.WithDetails(out)
	}
	var ute *json.UnmarshalTypeError
	if errors.As(err, &ute) {
		return apperror.ErrValidation.WithDetails([]FieldError{{Field: ute.Field, Rule: "type",
			Message: "must be of type " + ute.Type.String()}})
	}
	var se *json.SyntaxError
	if errors.As(err, &se) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return apperror.ErrBadRequest.WithMessage("request body must be valid JSON")
	}
	if ae, ok := apperror.As(err); ok {
		return ae
	}
	return apperror.ErrBadRequest.WithMessage("malformed request").Wrap(err)
}

// Field builds a single-field validation error from service code.
func Field(field, rule, msg string) error {
	return apperror.ErrValidation.WithDetails([]FieldError{{Field: field, Rule: rule, Message: msg}})
}

func fieldPath(fe validator.FieldError) string {
	ns := fe.Namespace()
	if i := strings.Index(ns, "."); i >= 0 {
		return ns[i+1:]
	}
	return fe.Field()
}

func message(fe validator.FieldError) string {
	p := fe.Param()
	switch fe.Tag() {
	case "required":
		return "is required"
	case "required_without":
		return "is required when " + p + " is empty"
	case "email":
		return "must be a valid email address"
	case "min":
		if fe.Kind() == reflect.String {
			return fmt.Sprintf("must be at least %s characters", p)
		}
		if fe.Kind() == reflect.Slice {
			return fmt.Sprintf("must contain at least %s items", p)
		}
		return "must be at least " + p
	case "max":
		if fe.Kind() == reflect.String {
			return fmt.Sprintf("must be at most %s characters", p)
		}
		if fe.Kind() == reflect.Slice {
			return fmt.Sprintf("must contain at most %s items", p)
		}
		return "must be at most " + p
	case "len":
		return fmt.Sprintf("must be exactly %s characters", p)
	case "oneof":
		return "must be one of: " + strings.ReplaceAll(p, " ", ", ")
	case "strong_password":
		return "must be 8-72 characters and contain at least one letter and one number"
	case "bd_phone":
		return "must be a valid Bangladeshi mobile number (e.g. 01712345678)"
	case "intl_phone":
		return "must be a valid phone number in international format (e.g. +14155552671)"
	case "country", "iso3166_1_alpha2":
		return "must be an ISO 3166-1 alpha-2 country code (e.g. BD, US)"
	case "lang":
		return "must be a 2-letter ISO 639-1 language code (e.g. en, bn)"
	case "slug":
		return "may only contain lowercase letters, numbers, '-' and '_'"
	case "person_name":
		return "must contain only letters, spaces, dots, apostrophes and hyphens"
	case "numeric_code":
		return "must contain only digits"
	case "datetime":
		return "must be a date in format " + p
	case "uuid", "uuid4":
		return "must be a valid UUID"
	case "unique":
		return "must not contain duplicates"
	case "eq":
		return "must equal " + p
	case "gtefield", "ltefield", "gte", "lte", "gt", "lt":
		return fmt.Sprintf("failed %s %s", fe.Tag(), p)
	}
	return "is invalid (" + fe.Tag() + ")"
}
