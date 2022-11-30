package attributes

import "github.com/uor-framework/uor-client-go/model"

type stringAttribute struct {
	key   string
	value string
}

var _ model.Attribute = stringAttribute{}

// NewString returns a new string attribute.
func NewString(key string, value string) model.Attribute {
	return stringAttribute{key: key, value: value}
}

// Kind returns the kind for the attribute.
func (a stringAttribute) Kind() model.Kind {
	return model.KindString
}

// Key return the attribute key.
func (a stringAttribute) Key() string {
	return a.key
}

// IsNull returns whether the value is null.
func (a stringAttribute) IsNull() bool {
	return false
}

// AsBool returns the value as a boolean and errors if that is not
// the underlying type.
func (a stringAttribute) AsBool() (bool, error) {
	return false, ErrWrongKind
}

// AsString returns the value as a string and errors if that is not
// the underlying type.
func (a stringAttribute) AsString() (string, error) {
	return a.value, nil
}

// AsFloat returns the value as a float value and errors if that is not
// the underlying type.
func (a stringAttribute) AsFloat() (float64, error) {
	return 0, ErrWrongKind
}

// AsInt returns the value as an int value errors and if that is not
// the underlying type.
func (a stringAttribute) AsInt() (int64, error) {
	return 0, ErrWrongKind
}

// AsAny returns the value as an interface.
func (a stringAttribute) AsAny() interface{} {
	return a.value
}
