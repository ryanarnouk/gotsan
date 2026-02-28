package logger

import "fmt"

type Level int

const (
	Debug Level = iota
	Info
	Warn
	Error
)

var currentLevel = Info

func SetLevel(l Level) {
	currentLevel = l
}

func IsVerbose() bool {
	return currentLevel <= Debug
}

func Debugf(format string, args ...any) {
	if currentLevel <= Debug {
		fmt.Printf("[DEBUG] "+format+"\n", args...)
	}
}

func Infof(format string, args ...any) {
	if currentLevel <= Info {
		fmt.Printf(format+"\n", args...)
	}
}
