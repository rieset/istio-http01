/*
 * Функции, определенные в этом файле:
 *
 * - setupLogger() *zap.Logger
 *   Настраивает и возвращает логгер с улучшенной конфигурацией (цветная подсветка, ISO8601 время, короткий caller)
 */

package main

import (
	"flag"

	"go.uber.org/zap/zapcore"
	ctrl "sigs.k8s.io/controller-runtime"
	zaputil "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// setupLogger настраивает и возвращает логгер с улучшенной конфигурацией
func setupLogger() {
	opts := zaputil.Options{
		Development: true,
		// Используем консольный энкодер для лучшей читаемости в development
		EncoderConfigOptions: []zaputil.EncoderConfigOption{
			func(config *zapcore.EncoderConfig) {
				// Настройка цветной подсветки для уровней логирования
				config.EncodeLevel = zapcore.CapitalColorLevelEncoder
				// Настройка формата времени
				config.EncodeTime = zapcore.ISO8601TimeEncoder
				// Настройка формата caller (файл:строка)
				config.EncodeCaller = zapcore.ShortCallerEncoder
			},
		},
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	// Создаем логгер с улучшенной конфигурацией
	logger := zaputil.New(zaputil.UseFlagOptions(&opts))
	ctrl.SetLogger(logger)
}
