package monitoring

import "context"

func (s *Service) StorageReady(ctx context.Context) error {
	if e := s.requireStorage(); e != nil {
		return e
	}
	return s.storage.Check(ctx)
}
