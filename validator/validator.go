package validator

type Validator struct {
	Errors map[string][]string
}

func New() *Validator {
	return &Validator{
		Errors: map[string][]string{},
	}
}

func (v *Validator) AddError(field, message string) {
	if v.Errors == nil {
		v.Errors = make(map[string][]string)
	}
	v.Errors[field] = append(v.Errors[field], message)
}

func (v *Validator) HasErrors() bool {
	return len(v.Errors) > 0
}

func (v *Validator) First(field string) string {
	if messages, exists := v.Errors[field]; exists && len(messages) != 0 {
		return messages[0]
	}
	return ""
}

func (v *Validator) All(field string) []string {
	if messages, exists := v.Errors[field]; exists && len(messages) != 0 {
		return messages
	}
	return nil
}

func (v *Validator) Error() string {
	if v.HasErrors() {
		return "Validation errors occurred"
	}
	return ""
}

func (v *Validator) AsError() error {
	if v.HasErrors() {
		return v
	}
	return nil
}
