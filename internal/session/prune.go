package session

// emptyThreshold is the highest message count still considered "no real
// conversation": a session that holds at most its seeded system prompt.
const emptyThreshold = 1

// PruneEmpty deletes sessions that hold no real conversation — at most the
// seeded system prompt — except the one named keep (pass "" to consider every
// session). It is best-effort: a failed delete is skipped rather than aborting
// the sweep, and the count actually pruned is returned alongside the first
// error, so one locked or missing file never blocks the rest of the cleanup.
func PruneEmpty(s Store, keep string) (int, error) {
	list, err := s.List()
	if err != nil {
		return 0, err
	}
	var firstErr error
	pruned := 0
	for _, sess := range list {
		if sess.ID == keep || sess.Messages > emptyThreshold {
			continue
		}
		if err := s.Delete(sess.ID); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		pruned++
	}
	return pruned, firstErr
}
