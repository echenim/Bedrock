package node

import (
	"context"
	"fmt"

	"go.uber.org/zap"
)

// Service is the interface for managed subsystems.
type Service interface {
	Start(ctx context.Context) error
	Stop() error
	Name() string
}

// ServiceManager handles ordered start/stop of services.
type ServiceManager struct {
	services []Service
	logger   *zap.Logger
}

// NewServiceManager creates a service manager.
func NewServiceManager(logger *zap.Logger) *ServiceManager {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ServiceManager{logger: logger}
}

// Add appends a service to the manager.
func (sm *ServiceManager) Add(svc Service) {
	sm.services = append(sm.services, svc)
}

// StartAll starts all services in order. On failure, stops already-started services.
func (sm *ServiceManager) StartAll(ctx context.Context) error {
	for i, svc := range sm.services {
		sm.logger.Info("starting service", zap.String("name", svc.Name()))
		if err := svc.Start(ctx); err != nil {
			// Stop already-started services in reverse.
			for j := i - 1; j >= 0; j-- {
				if stopErr := sm.services[j].Stop(); stopErr != nil {
					sm.logger.Error("failed to stop service during rollback",
						zap.String("name", sm.services[j].Name()),
						zap.Error(stopErr),
					)
				}
			}
			return fmt.Errorf("start %s: %w", svc.Name(), err)
		}
	}
	return nil
}

// StopAll stops all services in reverse order.
func (sm *ServiceManager) StopAll() error {
	var firstErr error
	for i := len(sm.services) - 1; i >= 0; i-- {
		svc := sm.services[i]
		sm.logger.Info("stopping service", zap.String("name", svc.Name()))
		if err := svc.Stop(); err != nil {
			sm.logger.Error("failed to stop service",
				zap.String("name", svc.Name()),
				zap.Error(err),
			)
			if firstErr == nil {
				firstErr = fmt.Errorf("stop %s: %w", svc.Name(), err)
			}
		}
	}
	return firstErr
}

// Services returns the list of managed services.
func (sm *ServiceManager) Services() []Service {
	return sm.services
}
