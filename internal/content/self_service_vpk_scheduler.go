package content

import (
	"log"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

type SelfServiceVPKScheduler struct {
	mu      sync.Mutex
	manager *SelfServiceVPKManager
	cron    *cron.Cron
	started bool
	stopped bool
}

func NewSelfServiceVPKScheduler(manager *SelfServiceVPKManager) *SelfServiceVPKScheduler {
	return &SelfServiceVPKScheduler{manager: manager, cron: cron.New()}
}

func (s *SelfServiceVPKScheduler) RunNow(now time.Time) (SelfServiceVPKCleanupResult, error) {
	return s.manager.CleanupExpired(now)
}

func (s *SelfServiceVPKScheduler) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started || s.stopped {
		return nil
	}
	if _, err := s.cron.AddFunc("@hourly", func() {
		result, err := s.RunNow(time.Now().UTC())
		if err != nil {
			log.Printf("cleanup expired self-service VPKs: deleted=%d failures=%v: %v", result.Deleted, result.Failures, err)
		}
	}); err != nil {
		return err
	}
	s.started = true
	s.cron.Start()
	return nil
}

func (s *SelfServiceVPKScheduler) Stop() {
	s.mu.Lock()
	if !s.started || s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	done := s.cron.Stop().Done()
	s.mu.Unlock()
	<-done
}
