package session

// imagesFor is the image store for one session. projectID is "" for a scratch
// session. It returns nil when the database is in memory.
func (s *Service) imagesFor(sessionID, projectID string) *imageStore {
	return newImageStore(s.manager.StorageDir(), projectID, sessionID)
}
