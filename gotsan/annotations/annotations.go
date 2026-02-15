package annotations

type Kind int

const (
	Requires Kind = iota
	Acquires
	Returns
	GuardedBy
)

type Annotation interface {
	Kind() Kind
	Args() []string
}

type RequiresAnnotation struct {
	Mu string
}

// Implementing the interface
func (RequiresAnnotation) Kind() Kind {
	return Requires
}
func (a RequiresAnnotation) Args() []string {
	return []string{a.Mu}
}

type AcquiresAnnotation struct {
	Mu string
}

func (AcquiresAnnotation) Kind() Kind {
	return Requires
}
func (a AcquiresAnnotation) Args() []string {
	return []string{a.Mu}
}

type ReturnsAnnotation struct {
	Mu string
}

func (ReturnsAnnotation) Kind() Kind {
	return Returns
}

func (a ReturnsAnnotation) Args() []string {
	return []string{a.Mu}
}

type GuardedByAnnotation struct {
	Mu string
}

func (GuardedByAnnotation) Kind() Kind {
	return GuardedBy
}

func (a GuardedByAnnotation) Args() []string {
	return []string{a.Mu}
}

func StringToAnnotation(annotation string) {
	// TODO: Next implementation
	// clean separation of concerns between
	// parseAnnotations and this function
}
