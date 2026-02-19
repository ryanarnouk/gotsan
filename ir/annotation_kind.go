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
		return "Requires"
	case Acquires:
		return "Acquires"
	case Returns:
		return "Returns"
	case GuardedBy:
		return "GuardedBy"
	default:
		return fmt.Sprintf("AnnotationKind(%d)", int(k))
	}
}
