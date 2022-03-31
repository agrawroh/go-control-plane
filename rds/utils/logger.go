package utils

import (
	"log"
)

type Logger struct {
	Debug bool
}

// Debugf Log to stdout only if Debug is true.
func (logger Logger) Debugf(format string, args ...interface{}) {
	if logger.Debug {
		log.Printf(format+"\n", args...)
	}
}

// Infof Log to stdout only if Debug is true.
func (logger Logger) Infof(format string, args ...interface{}) {
	if logger.Debug {
		log.Printf(format+"\n", args...)
	}
}

// Warnf Log to stdout always.
func (logger Logger) Warnf(format string, args ...interface{}) {
	log.Printf(format+"\n", args...)
}

// Errorf Log to stdout always.
func (logger Logger) Errorf(format string, args ...interface{}) {
	log.Printf(format+"\n", args...)
}
