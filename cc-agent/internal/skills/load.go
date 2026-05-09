package skills

// LoadTwoPhase loads localDir first, then teamDir, into r. Because the
// underlying registry's map insert is last-write-wins, team skills override
// local ones with the same name — deterministically, regardless of the
// underlying filesystem walk order.
//
// Either directory may be missing; missing directories are ignored.
func LoadTwoPhase(r *Registry, localDir, teamDir string) error {
	if err := r.LoadDir(localDir); err != nil {
		return err
	}
	return r.LoadDir(teamDir)
}
