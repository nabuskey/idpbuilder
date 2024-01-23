package helpers

import (
	"go.uber.org/zap/zapcore"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	ZapOptions = zap.Options{
		Level:       zapcore.InfoLevel,
		Development: true,
		TimeEncoder: zapcore.RFC3339TimeEncoder,
	}
)

func SetZap() {
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&ZapOptions)))
}
