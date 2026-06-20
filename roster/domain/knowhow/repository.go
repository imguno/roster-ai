package knowhow

// Repository persists and retrieves knowhow entries.
type Repository interface {
	Save(deskID string, entry Knowhow) error
	Load(deskID string, limit int) ([]Knowhow, error)
	Prune(deskID string, keepLatest int) error
}
