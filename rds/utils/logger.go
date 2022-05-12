package utils

import (
	"log"
)

type Logger struct {
	Debug bool
	Info  bool
}

// Debugf logs to stdout only if Debug is set to true.
func (logger Logger) Debugf(format string, args ...interface{}) {
	if logger.Debug {
		log.Printf(format+"\n", args...)
	}
}

// Infof logs to stdout only if Debug or Info is set to true.
func (logger Logger) Infof(format string, args ...interface{}) {
	if logger.Debug || logger.Info {
		log.Printf(format+"\n", args...)
	}
}

// Warnf logs to stdout always.
func (logger Logger) Warnf(format string, args ...interface{}) {
	log.Printf(format+"\n", args...)
}

// Errorf logs to stdout always.
func (logger Logger) Errorf(format string, args ...interface{}) {
	log.Printf(format+"\n", args...)
}
