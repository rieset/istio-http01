/*
 * Функции, определенные в этом файле:
 *
 * - setupWebhookCertWatcher(webhookCertPath, webhookCertName, webhookCertKey string)
 *   (*certwatcher.CertWatcher, []func(*tls.Config), error)
 *   Настраивает watcher для сертификатов webhook и возвращает TLS опции
 *
 * - setupMetricsCertWatcher(metricsCertPath, metricsCertName, metricsCertKey string)
 *   (*certwatcher.CertWatcher, []func(*tls.Config), error)
 *   Настраивает watcher для сертификатов metrics и возвращает TLS опции
 */

package main

import (
	"crypto/tls"
	"fmt"
	"path/filepath"

	"sigs.k8s.io/controller-runtime/pkg/certwatcher"
)

// setupWebhookCertWatcher настраивает watcher для сертификатов webhook
func setupWebhookCertWatcher(
	webhookCertPath, webhookCertName, webhookCertKey string,
) (*certwatcher.CertWatcher, []func(*tls.Config), error) {
	if len(webhookCertPath) == 0 {
		return nil, nil, nil
	}

	setupLog.Info("Initializing webhook certificate watcher using provided certificates",
		"webhook-cert-path", webhookCertPath, "webhook-cert-name", webhookCertName, "webhook-cert-key", webhookCertKey)

	webhookCertWatcher, err := certwatcher.New(
		filepath.Join(webhookCertPath, webhookCertName),
		filepath.Join(webhookCertPath, webhookCertKey),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize webhook certificate watcher: %w", err)
	}

	tlsOpts := []func(*tls.Config){
		func(config *tls.Config) {
			config.GetCertificate = webhookCertWatcher.GetCertificate
		},
	}

	return webhookCertWatcher, tlsOpts, nil
}

// setupMetricsCertWatcher настраивает watcher для сертификатов metrics
func setupMetricsCertWatcher(
	metricsCertPath, metricsCertName, metricsCertKey string,
) (*certwatcher.CertWatcher, []func(*tls.Config), error) {
	if len(metricsCertPath) == 0 {
		return nil, nil, nil
	}

	setupLog.Info("Initializing metrics certificate watcher using provided certificates",
		"metrics-cert-path", metricsCertPath, "metrics-cert-name", metricsCertName, "metrics-cert-key", metricsCertKey)

	metricsCertWatcher, err := certwatcher.New(
		filepath.Join(metricsCertPath, metricsCertName),
		filepath.Join(metricsCertPath, metricsCertKey),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize metrics certificate watcher: %w", err)
	}

	tlsOpts := []func(*tls.Config){
		func(config *tls.Config) {
			config.GetCertificate = metricsCertWatcher.GetCertificate
		},
	}

	return metricsCertWatcher, tlsOpts, nil
}
