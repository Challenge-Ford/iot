package logger

import "go.uber.org/zap"

func New(json bool) (*zap.Logger, error) {
	if json {
		return zap.NewProduction()
	}
	return zap.NewDevelopment()
}
