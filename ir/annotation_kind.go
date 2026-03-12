package ir

import "fmt"

type AnnotationKind int

const (
	None AnnotationKind = iota
	Requires
	Acquires
	Returns
	GuardedBy
)

var AnnotationKindMap = map[string]AnnotationKind{
	"requires":   Requires,
	"acquires":   Acquires,
	"returns":    Returns,
	"guarded_by": GuardedBy,
}

func (k AnnotationKind) String() string {
	switch k {
	case Requires:
		return "requires"
	case Acquires:
		return "acquires"
	case Returns:
		return "returns"
	case GuardedBy:
		return "guarded_by"
	default:
		return fmt.Sprintf("AnnotationKind(%d)", int(k))
	}
}
