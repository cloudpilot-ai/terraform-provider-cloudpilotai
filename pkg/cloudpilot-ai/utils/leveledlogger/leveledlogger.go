package leveledlogger

import (
	"errors"
	"fmt"

	"github.com/hashicorp/go-retryablehttp"
	"k8s.io/klog/v2"
)

type KlogLeveledLogger struct{}

var _ retryablehttp.LeveledLogger = &KlogLeveledLogger{}

func NewKlogLeveledLogger() *KlogLeveledLogger {
	return &KlogLeveledLogger{}
}

func (l *KlogLeveledLogger) Error(msg string, keysAndValues ...interface{}) {
	// Fix ci error, use errors.New here.
	klog.ErrorS(errors.New(msg), "error:", keysAndValues...)
}

func (l *KlogLeveledLogger) Info(msg string, keysAndValues ...interface{}) {
	klog.InfoS(msg, keysAndValues...)
}

func (l *KlogLeveledLogger) Debug(msg string, keysAndValues ...interface{}) {
	klog.V(5).InfoS(msg, keysAndValues...)
}

func (l *KlogLeveledLogger) Warn(msg string, keysAndValues ...interface{}) {
	klog.InfoS(fmt.Sprintf("warning: %s", msg), keysAndValues...)
}
