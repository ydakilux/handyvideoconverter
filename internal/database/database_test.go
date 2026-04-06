package database

import (
	"os"

	"github.com/sirupsen/logrus"
)

func newTestLogger() *logrus.Logger {
	l := logrus.New()
	l.SetOutput(os.Stderr)
	l.SetLevel(logrus.WarnLevel)
	return l
}
