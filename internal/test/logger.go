package test

import (
	"fmt"
	"log"

	"go.uber.org/zap"
	kindlog "sigs.k8s.io/kind/pkg/log"
)

func newKindLogger() kindLogger {
	logger, err := zap.NewDevelopment()
	if err != nil {
		log.Fatalf("failed to create zap logger for kind provider")
	}
	return kindLogger{logger}
}

type kindLogger struct {
	logger *zap.Logger
}

func (l kindLogger) Warn(message string) {
	l.logger.Warn(message)
}

func (l kindLogger) Warnf(format string, args ...interface{}) {
	l.logger.Warn(fmt.Sprintf(format, args...))
}

func (l kindLogger) Error(message string) {
	l.logger.Error(message)
}

func (l kindLogger) Errorf(format string, args ...interface{}) {
	l.logger.Error(fmt.Sprintf(format, args...))
}

func (l kindLogger) V(level kindlog.Level) kindlog.InfoLogger {
	return l
}

func (l kindLogger) Info(message string) {
	l.logger.Info(message)
}

func (l kindLogger) Infof(format string, args ...interface{}) {
	l.logger.Info(fmt.Sprintf(format, args...))
}

func (l kindLogger) Enabled() bool {
	return true
}
