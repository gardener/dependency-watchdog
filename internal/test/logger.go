// Copyright 2022 SAP SE or an SAP affiliate company
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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

func (l kindLogger) V(_ kindlog.Level) kindlog.InfoLogger {
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
