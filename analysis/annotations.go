package analysis

import (
	"fmt"
	"strings"
)

type AnnotationKind int

const (
	Requires AnnotationKind = iota
	Acquires
	Returns
	GuardedBy
)

type Annotation struct {
	Kind   AnnotationKind
	Params []string
}

var annotationKindMap = map[string]AnnotationKind{
	"requires":   Requires,
	"acquires":   Acquires,
	"returns":    Returns,
	"guarded_by": GuardedBy,
}

// Check if the comment is an annotation, returning true and returning the extracted value, such that:
// The comment string is one of:
//   - "//@"
//   - "/*@"
//   - "@" (in the case of being in the middle or end of a multi-line code block comment)
//
// Note: a single line cannot contain more than one annotation
func extractAnnotation(comment string) (string, bool) {
	// Immediately can verify this comment is not an annotation
	if !strings.Contains(comment, "@") {
		return "", false
	}
	trimmed := strings.TrimSpace(comment)
	removedCommentMarkers := strings.TrimPrefix(trimmed, "//")
	removedCommentMarkers = strings.TrimPrefix(removedCommentMarkers, "/*")
	removedCommentMarkers = strings.TrimSuffix(removedCommentMarkers, "*/")
	cleaned := strings.TrimSpace(removedCommentMarkers)

	if !strings.HasPrefix(cleaned, "@") {
		return "", false
	}

	return strings.TrimSpace(cleaned[1:]), true
}

// Invariant: annotation has had the comment prefix/suffix and @ removed from the string already
func parseAnnotation(annotation string) (Annotation, error) {
	open := strings.Index(annotation, "(")
	close := strings.Index(annotation, ")")

	if open == -1 || close == -1 || open > close {
		return Annotation{}, fmt.Errorf("invalid annotation format: %q", annotation)
	}

	annotationName := strings.TrimSpace(annotation[:open])
	params := strings.Split(annotation[open+1:close], ",")
	for i := range params {
		params[i] = strings.TrimSpace(params[i])
	}

	return Annotation{
		Kind:   annotationKindMap[annotationName],
		Params: params,
	}, nil
}

func ParseAnnotation(commentText string) (Annotation, error) {
	annotation, isAnnotation := extractAnnotation(commentText)
	if !isAnnotation {
		return Annotation{}, nil
	}

	return parseAnnotation(annotation)
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
